<script setup lang="ts">
/* global AbortController, AbortSignal, localStorage, document */
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { api as client } from '../../api/client'
import { createServerApi, type ServerApi } from '../../api/server-api'
import type { ArticleSummary, FeedQuery, LibraryStateWrite, Source } from '../../api/generated/models'
import ArticleSummaryCard from '../../components/shared/ArticleSummaryCard.vue'
import RankingExplanation from '../../components/shared/RankingExplanation.vue'
import EmptyState from '../../components/shared/EmptyState.vue'
import StatusBanner from '../../components/shared/StatusBanner.vue'
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
  <section
    class="library-page"
    aria-labelledby="library-title"
  >
    <header class="library-header">
      <p class="eyebrow">
        Your stories
      </p><h1
        id="library-title"
        tabindex="-1"
      >
        Library
      </h1><p>Find stories you saved, read, or hid. Every action can be reversed.</p>
    </header>
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
    <div
      v-if="state==='loading'"
      class="library-state"
      role="status"
    >
      Loading {{ view }} stories…
    </div>
    <StatusBanner
      v-else-if="state==='error'"
      title="Library unavailable"
      tone="danger"
    >
      {{ message }}<template #actions>
        <button
          type="button"
          class="secondary"
          @click="load"
        >
          Try again
        </button>
      </template>
    </StatusBanner>
    <EmptyState
      v-else-if="state==='empty'"
      :title="`No ${view} stories`"
      :description="view === 'saved' ? 'Save a story from your feed and it will appear here.' : view === 'hidden' ? 'Stories you hide will appear here so you can restore them.' : `You do not have any ${view} stories yet.`"
    >
      <AppLink
        class="button-link"
        to="/"
      >
        Browse ranked stories
      </AppLink>
    </EmptyState>
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
          :reader-to="`/articles/${encodeURIComponent(article.id)}`"
        >
          <template #explanation>
            <RankingExplanation :contributions="article.ranking.contributions" />
          </template><template #actions>
            <div
              class="library-actions"
              aria-label="Article actions"
            >
              <button
                type="button"
                class="tertiary"
                :disabled="busy[article.id]"
                @click="mutate(article,{read:!article.library.readAt})"
              >
                {{ article.library.readAt?'Mark unread':'Mark read' }}
              </button><button
                type="button"
                class="tertiary"
                :disabled="busy[article.id]"
                @click="mutate(article,{saved:!article.library.savedAt})"
              >
                {{ article.library.savedAt?'Unsave':'Save' }}
              </button><button
                v-if="article.library.hiddenAt"
                type="button"
                class="secondary"
                :disabled="busy[article.id]"
                @click="mutate(article,{hidden:false})"
              >
                Restore
              </button><button
                v-else
                type="button"
                class="tertiary"
                :disabled="busy[article.id]"
                @click="mutate(article,{hidden:true})"
              >
                Hide
              </button>
            </div>
          </template>
        </ArticleSummaryCard>
      </li>
    </ol>
    <p aria-live="polite">
      {{ message }}
    </p>
  </section>
</template>

<style scoped>
.library-page,.library-header{display:grid;gap:var(--space-5)}.library-header{gap:var(--space-2)}.library-header>p:last-child{color:var(--color-muted);font-size:1.02rem}.library-tabs{display:flex;flex-wrap:wrap;gap:var(--space-1);padding:var(--space-1);border:1px solid var(--color-border);border-radius:var(--radius-md);background:var(--color-surface-soft);width:fit-content}.library-tabs button{border-color:transparent;background:transparent;color:var(--color-muted)}.library-tabs button:hover{background:var(--color-brand-soft);color:var(--color-brand)}.library-tabs [aria-pressed=true]{border-color:#b8cae2;background:var(--color-brand-soft);color:var(--color-brand-strong)}.library-state{min-height:12rem;display:grid;place-items:center;color:var(--color-muted)}.library-list{display:grid;gap:var(--space-4);margin:0;padding:0;list-style:none}.library-actions{display:flex;align-items:center;flex-wrap:wrap;gap:var(--space-1)}.library-actions button{min-height:2.5rem;padding-inline:.7rem}@media(max-width:36rem){.library-tabs{width:100%;display:grid;grid-template-columns:repeat(2,minmax(0,1fr))}.library-actions button{flex:1 1 auto}}
</style>
