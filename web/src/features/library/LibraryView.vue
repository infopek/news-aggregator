<script setup lang="ts">
/* global AbortController, AbortSignal, localStorage, document */
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { api as client } from '../../api/client'
import { createServerApi, type ServerApi } from '../../api/server-api'
import type { ArticleSummary, FeedQuery, LibraryStateWrite, Source } from '../../api/generated/models'
import ArticleSummaryCard from '../../components/shared/ArticleSummaryCard.vue'
import RankingExplanation from '../../components/shared/RankingExplanation.vue'
import AppLink from '../../router/AppLink.vue'
import { toUserSafeError } from '../../state/errors'
import { ServerMutations } from '../../state/mutations'
import { ServerStateClient } from '../../state/query-client'
import { publishLibraryInvalidation, subscribeLibraryInvalidation } from '../feed/library-invalidation'

type LibraryView = 'saved'|'unread'|'read'|'hidden'
const props=withDefaults(defineProps<{serverApi?:ServerApi}>(),{serverApi:undefined})
const server=props.serverApi??createServerApi(client),mutations=new ServerMutations(server,new ServerStateClient())
const preferenceKey='news-aggregator:library-view',views:LibraryView[]=['saved','unread','read','hidden']
const stored=localStorage.getItem(preferenceKey) as LibraryView|null
const view=ref<LibraryView>(stored&&views.includes(stored)?stored:'saved'),items=ref<ArticleSummary[]>([]),sources=ref<Source[]>([])
const state=ref<'loading'|'ready'|'empty'|'error'>('loading'),message=ref(''),busy=ref<Record<string,boolean>>({})
const sourceNames=computed(()=>Object.fromEntries(sources.value.map(item=>[item.id,item.name])))
let controller:AbortController|undefined,generation=0,mounted=true,unsubscribe:(()=>void)|undefined
onMounted(()=>{unsubscribe=subscribeLibraryInvalidation(()=>{void load()});document.addEventListener('visibilitychange',visible);void load()})
onBeforeUnmount(()=>{mounted=false;++generation;unsubscribe?.();document.removeEventListener('visibilitychange',visible);controller?.abort()})
function visible(){if(document.visibilityState==='visible')void load()}
function query():FeedQuery{return view.value==='saved'?{saved:true,limit:100}:view.value==='unread'?{read:false,limit:100}:view.value==='read'?{read:true,limit:100}:{includeHidden:true,limit:100}}
async function load(){const current=++generation;controller?.abort();controller=new AbortController();state.value='loading';message.value='';try{const [articles,list]=await Promise.all([loadArticles(query(),controller.signal),server.sources(controller.signal)]);if(!mounted||current!==generation)return;items.value=view.value==='hidden'?articles.filter(item=>Boolean(item.library.hiddenAt)):articles; sources.value=list.items;state.value=items.value.length?'ready':'empty'}catch(cause){if(!mounted||current!==generation||controller.signal.aborted)return;message.value=toUserSafeError(cause).message;state.value='error'}}
async function loadArticles(base:FeedQuery,signal:AbortSignal){const result:ArticleSummary[]=[];let cursor:string|undefined;do{const page=await server.feed({...base,cursor},signal);result.push(...page.items);cursor=page.nextCursor??undefined}while(cursor);return result}
function select(next:LibraryView){view.value=next;localStorage.setItem(preferenceKey,next);void load()}
async function mutate(article:ArticleSummary,patch:LibraryStateWrite){busy.value[article.id]=true;const result=await mutations.updateLibrary(article.id,patch);publishLibraryInvalidation(article.id);if(!mounted)return;busy.value[article.id]=false;message.value=result.status==='success'?'Article state updated.':result.error.message}
</script>

<template>
  <section aria-labelledby="library-title">
    <p class="eyebrow">
      Your local reading history
    </p><h1
      id="library-title"
      tabindex="-1"
    >
      Personal library
    </h1>
    <p>Saved, read, and hidden states come from the local database and remain reversible.</p>
    <nav
      aria-label="Library views"
      class="library-tabs"
    >
      <button
        v-for="name in views"
        :key="name"
        type="button"
        :aria-pressed="view===name"
        @click="select(name)"
      >
        {{ name === 'unread' ? 'Unread' : name[0].toUpperCase()+name.slice(1) }}
      </button>
    </nav>
    <p
      v-if="state==='loading'"
      role="status"
    >
      Loading {{ view }} articles…
    </p>
    <div
      v-else-if="state==='error'"
      role="alert"
    >
      <p>{{ message }}</p><button
        type="button"
        @click="load"
      >
        Try again
      </button>
    </div>
    <div
      v-else-if="state==='empty'"
      class="empty-state"
    >
      <p>No {{ view }} articles.</p><AppLink to="/">
        Return to the ranked feed
      </AppLink>
    </div>
    <ol
      v-else
      class="library-list"
    >
      <li
        v-for="article in items"
        :key="article.id"
      >
        <ArticleSummaryCard
          :article="article"
          :source-name="sourceNames[article.sourceId]??'Unknown source'"
          heading-level="h2"
        />
        <RankingExplanation
          :contributions="article.ranking.contributions"
          heading-level="h3"
        />
        <AppLink :to="`/articles/${encodeURIComponent(article.id)}`">
          Open reader
        </AppLink>
        <div
          class="actions"
          aria-label="Article actions"
        >
          <button
            type="button"
            :disabled="busy[article.id]"
            @click="mutate(article,{read:!article.library.readAt})"
          >
            {{ article.library.readAt?'Mark unread':'Mark read' }}
          </button>
          <button
            type="button"
            :disabled="busy[article.id]"
            @click="mutate(article,{saved:!article.library.savedAt})"
          >
            {{ article.library.savedAt?'Unsave':'Save' }}
          </button>
          <button
            v-if="article.library.hiddenAt"
            type="button"
            :disabled="busy[article.id]"
            @click="mutate(article,{hidden:false})"
          >
            Restore
          </button>
          <button
            v-else
            type="button"
            :disabled="busy[article.id]"
            @click="mutate(article,{hidden:true})"
          >
            Hide
          </button>
        </div>
      </li>
    </ol>
    <p aria-live="polite">
      {{ message }}
    </p>
  </section>
</template>

<style scoped>.library-tabs,.actions{display:flex;flex-wrap:wrap;gap:.75rem}.library-tabs [aria-pressed=true]{font-weight:700;outline:.15rem solid currentColor}.library-list{list-style:none;padding:0}.library-list>li{padding-block:1rem;border-block-end:1px solid var(--border,#777)}.empty-state{padding:1rem;background:var(--surface-muted,#f4f5f6)}</style>
