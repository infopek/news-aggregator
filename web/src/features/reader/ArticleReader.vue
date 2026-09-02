<script setup lang="ts">
/* global AbortController, URL */
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { api as client } from '../../api/client'
import { createServerApi, type ServerApi } from '../../api/server-api'
import type { ArticleDetail, LibraryStateWrite } from '../../api/generated/models'
import PermissionBadge from '../../components/shared/PermissionBadge.vue'
import RankingExplanation from '../../components/shared/RankingExplanation.vue'
import AppLink from '../../router/AppLink.vue'
import LibraryActions from '../feed/LibraryActions.vue'
import { publishLibraryInvalidation, subscribeLibraryInvalidation } from '../feed/library-invalidation'
import { toUserSafeError } from '../../state/errors'
import { ServerMutations } from '../../state/mutations'
import { ServerStateClient } from '../../state/query-client'
const props = withDefaults(defineProps<{ articleId: string; serverApi?: ServerApi }>(), { serverApi: undefined })
const server = props.serverApi ?? createServerApi(client), detail = ref<ArticleDetail>(), state = ref<'loading'|'ready'|'error'>('loading'), error = ref('')
const mutations = new ServerMutations(server, new ServerStateClient()), actionBusy=ref(false), actionMessage=ref('')
let controller: AbortController | undefined, generation=0, mounted=true
let unsubscribeInvalidation:(()=>void)|undefined
const publisher = computed(() => { try { const url = new URL(detail.value?.article.canonicalUrl ?? ''); return ['http:','https:'].includes(url.protocol) ? url.href : null } catch { return null } })
onMounted(()=>{unsubscribeInvalidation=subscribeLibraryInvalidation(id=>{if(id===props.articleId)void load()});void load()}); onBeforeUnmount(()=>{mounted=false;++generation;unsubscribeInvalidation?.();controller?.abort()}); watch(()=>props.articleId,()=>{actionBusy.value=false;actionMessage.value='';void load()})
async function load(){const current=++generation;controller?.abort();controller=new AbortController();state.value='loading';error.value='';try{const value=await server.article(props.articleId,controller.signal);if(current!==generation)return;detail.value=value;state.value='ready'}catch(cause){if(current!==generation||controller.signal.aborted)return;error.value=toUserSafeError(cause).message;state.value='error'}}
async function mutate(patch:LibraryStateWrite){if(!detail.value)return;const id=detail.value.article.id,current=generation;actionBusy.value=true;actionMessage.value='';const result=await mutations.updateLibrary(id,patch);if(mounted&&current===generation&&props.articleId===id&&detail.value?.article.id===id){if(result.status==='success'){detail.value.article.library=result.data;actionMessage.value='Article state updated.'}else actionMessage.value=result.error.message;actionBusy.value=false}publishLibraryInvalidation(id)}
</script>
<template>
  <section
    class="reader-page"
    aria-labelledby="reader-title"
  >
    <AppLink
      class="reader-back"
      to="/"
    >
      ← Back to ranked feed
    </AppLink>
    <div
      v-if="state==='loading'"
      class="reader-state"
      role="status"
    >
      Loading article…
    </div>
    <div
      v-else-if="state==='error'"
      class="reader-error"
      role="alert"
    >
      <h1 id="reader-title">
        Article unavailable
      </h1><p>{{ error }}</p><button
        type="button"
        @click="load"
      >
        Try again
      </button>
    </div>
    <article
      v-else-if="detail"
      class="reader-article"
    >
      <header class="reader-header">
        <div class="reader-meta">
          <PermissionBadge :permission="detail.article.contentPermission" /><span v-if="detail.article.author">By {{ detail.article.author }}</span><time
            v-if="detail.article.publishedAt"
            :datetime="detail.article.publishedAt"
          >{{ new Date(detail.article.publishedAt).toLocaleString() }}</time>
        </div>
        <h1
          id="reader-title"
          tabindex="-1"
        >
          {{ detail.article.title }}
        </h1>
        <p
          v-if="detail.article.excerpt"
          class="reader-deck"
        >
          {{ detail.article.excerpt }}
        </p>
        <div class="reader-tools">
          <LibraryActions
            :article="detail.article"
            :busy="actionBusy"
            :message="actionMessage"
            @mutate="mutate"
          /><RankingExplanation :contributions="detail.article.ranking.contributions" />
        </div>
      </header>
      <div
        v-if="detail.article.contentPermission==='full_content_allowed'"
        class="article-content"
      >
        {{ detail.fullContent || 'No article body was provided by this source.' }}
      </div>
      <div
        v-else
        class="publisher-fallback"
      >
        <div><h2>Continue at the publisher</h2><p>This source shares article details only, so the full text is not stored on this computer.</p></div>
        <a
          v-if="publisher"
          class="button-link"
          :href="publisher"
          target="_blank"
          rel="noopener noreferrer"
        >Read the full article</a>
        <p v-else>
          Publisher link unavailable.
        </p>
      </div>
    </article>
  </section>
</template>
<style scoped>
.reader-page { max-width: var(--content-reading); display: grid; gap: var(--space-5); }
.reader-back { width: fit-content; font-weight: 680; text-decoration: none; }
.reader-state { min-height: 15rem; display: grid; place-items: center; color: var(--color-muted); }
.reader-error, .reader-article, .reader-header { display: grid; gap: var(--space-4); }
.reader-meta { display: flex; align-items: center; flex-wrap: wrap; gap: var(--space-3); color: var(--color-muted); font-size: .9rem; }
.reader-deck { color: var(--color-muted); font-size: 1.08rem; line-height: 1.65; }
.reader-tools { display: flex; align-items: center; justify-content: space-between; flex-wrap: wrap; gap: var(--space-3); padding-block: var(--space-3); border-block: 1px solid var(--color-border); }
.article-content { white-space: pre-wrap; overflow-wrap: anywhere; font-size: 1.05rem; line-height: 1.78; }
.publisher-fallback { display: flex; align-items: center; justify-content: space-between; gap: var(--space-5); padding: var(--space-5); border: 1px solid #b8c9e4; border-radius: var(--radius-lg); background: var(--color-brand-soft); }
.publisher-fallback div { display: grid; gap: var(--space-2); }
.publisher-fallback p { color: var(--color-muted); }
@media (max-width: 36rem) { .reader-tools, .publisher-fallback { align-items: stretch; flex-direction: column; } .publisher-fallback .button-link { width: 100%; } }
</style>
