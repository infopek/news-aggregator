<script setup lang="ts">
/* global AbortController */
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { api as client } from '../../api/client'
import { createServerApi, type ServerApi } from '../../api/server-api'
import type { ArticleSummary, FeedQuery, LibraryStateWrite, Source } from '../../api/generated/models'
import ArticleSummaryCard from '../../components/shared/ArticleSummaryCard.vue'
import RankingExplanation from '../../components/shared/RankingExplanation.vue'
import FilterControl from '../../components/shared/FilterControl.vue'
import AppLink from '../../router/AppLink.vue'
import { toUserSafeError } from '../../state/errors'
import { ServerMutations } from '../../state/mutations'
import { ServerStateClient } from '../../state/query-client'
import LibraryActions from './LibraryActions.vue'
import RefreshControl from '../refresh/RefreshControl.vue'

const props = withDefaults(defineProps<{ serverApi?: ServerApi }>(), { serverApi: undefined })
const server = props.serverApi ?? createServerApi(client)
const mutations = new ServerMutations(server, new ServerStateClient())
const items = ref<ArticleSummary[]>([]), sources = ref<Source[]>([]), cursor = ref<string|null>(null)
const state = ref<'loading'|'ready'|'empty'|'error'>('loading'), loadingMore = ref(false), queryError = ref(''), pageError = ref('')
const revalidating = ref(false), stale = ref(false)
const actionBusy = ref<Record<string, boolean>>({}), actionMessages = ref<Record<string, string>>({})
const source = ref(''), read = ref('all'), saved = ref('all'), text = ref(''), after = ref(''), before = ref('')
type FilterSnapshot = { source: string; read: string; saved: string; text: string; after: string; before: string }
const applied = ref<FilterSnapshot>({ source: '', read: 'all', saved: 'all', text: '', after: '', before: '' })
let generation = 0, loadController: AbortController | undefined, pageController: AbortController | undefined, mounted=true
const refreshState = ref<'idle'|'running'|'uncertain'>('idle')
const sourceNames = computed(() => Object.fromEntries(sources.value.map((item) => [item.id, item.name])))
onMounted(() => load(false)); onBeforeUnmount(() => { mounted=false;++generation;loadController?.abort();pageController?.abort() })
function draft(): FilterSnapshot { return { source: source.value, read: read.value, saved: saved.value, text: text.value, after: after.value, before: before.value } }
function query(filter: FilterSnapshot, next?: string): FeedQuery { return { cursor: next || undefined, limit: 20, sourceId: filter.source ? [filter.source] : undefined, read: filter.read === 'all' ? undefined : filter.read === 'read', saved: filter.saved === 'saved' ? true : undefined, text: filter.text.trim() || undefined, publishedAfter: filter.after ? new Date(`${filter.after}T00:00:00`).toISOString() : undefined, publishedBefore: filter.before ? new Date(`${filter.before}T23:59:59`).toISOString() : undefined } }
async function load(applyDraft = true) {
  if (applyDraft) applied.value = draft()
  const requestGeneration = ++generation; loadController?.abort(); pageController?.abort(); loadController = new AbortController(); loadingMore.value=false; pageError.value=''
  const hasData = state.value === 'ready' || state.value === 'empty'
  if (hasData) revalidating.value = true
  else state.value = 'loading'
  queryError.value = ''; stale.value = false
  try { const [page, list] = await Promise.all([server.feed(query(applied.value), loadController.signal), server.sources(loadController.signal)]); if(requestGeneration!==generation)return; items.value = page.items; cursor.value = page.nextCursor; sources.value = list.items; state.value = items.value.length ? 'ready' : 'empty'; stale.value = false } catch (cause) { if(requestGeneration!==generation)return; queryError.value = toUserSafeError(cause).message; if(hasData)stale.value=true;else state.value='error' } finally { if(requestGeneration===generation)revalidating.value=false }
}
async function more() { if (!cursor.value) return; const requestGeneration=generation, next=cursor.value, filter={...applied.value}; pageController?.abort(); pageController=new AbortController(); loadingMore.value = true; pageError.value=''; try { const page = await server.feed(query(filter,next),pageController.signal); if(requestGeneration!==generation)return; items.value.push(...page.items); cursor.value = page.nextCursor; pageError.value='' } catch (cause) { if(requestGeneration===generation)pageError.value = toUserSafeError(cause).message } finally { if(requestGeneration===generation)loadingMore.value = false } }
function hide(id: string) { items.value = items.value.filter((item) => item.id !== id); if (!items.value.length) state.value = 'empty' }
async function mutate(article: ArticleSummary, patch: LibraryStateWrite) {
  actionBusy.value[article.id] = true; actionMessages.value[article.id] = ''
  const result = await mutations.updateLibrary(article.id, patch)
  if (!mounted) return
  if (result.status === 'success') {
    article.library = result.data; actionMessages.value[article.id] = 'Article state updated.'
    if (result.data.hiddenAt || (applied.value.saved==='saved'&&!result.data.savedAt)||(applied.value.read==='unread'&&result.data.readAt)||(applied.value.read==='read'&&!result.data.readAt)) hide(article.id)
  } else actionMessages.value[article.id] = result.error.message
  actionBusy.value[article.id] = false
  await load(false)
}
async function refreshed(){refreshState.value='idle';await load(false)}
function clear(){source.value='';read.value='all';saved.value='all';text.value='';after.value='';before.value='';void load()}
</script>
<template>
  <section aria-labelledby="feed-title">
    <p class="eyebrow">
      Relevant first
    </p><h1
      id="feed-title"
      tabindex="-1"
    >
      Ranked feed
    </h1><p>Order and explanations come from your local ranking service.</p>
    <p
      v-if="refreshState==='running'"
      role="status"
    >
      Refresh is running. Current articles may be stale until it completes.
    </p>
    <p
      v-else-if="refreshState==='uncertain'"
      role="status"
    >
      Refresh status is unavailable. Current articles may be stale; retry the saved status below or start a new refresh.
    </p>
    <RefreshControl
      :server="server"
      :source-names="sourceNames"
      @started="refreshState='running'"
      @completed="refreshed"
      @stopped="refreshState='uncertain'"
    />
    <form
      class="feed-filters"
      @submit.prevent="load()"
    >
      <FilterControl
        id="source-filter"
        v-model="source"
        label="Source"
        :options="[{value:'',label:'All sources'},...sources.map(item=>({value:item.id,label:item.name}))]"
      /><FilterControl
        id="read-filter"
        v-model="read"
        label="Read state"
        :options="[{value:'all',label:'All'},{value:'unread',label:'Unread'},{value:'read',label:'Read'}]"
      /><FilterControl
        id="saved-filter"
        v-model="saved"
        label="Saved state"
        :options="[{value:'all',label:'All'},{value:'saved',label:'Saved only'}]"
      /><label>Search <input
        v-model="text"
        maxlength="200"
      ></label><label>Published after <input
        v-model="after"
        type="date"
      ></label><label>Published before <input
        v-model="before"
        type="date"
      ></label><button type="submit">
        Apply filters
      </button><button
        type="button"
        @click="clear"
      >
        Clear filters
      </button>
    </form>
    <p
      v-if="revalidating"
      role="status"
    >
      Updating ranked articles… Current results remain visible.
    </p>
    <div
      v-if="stale"
      role="alert"
    >
      <p>These articles may be stale. {{ queryError }}</p><button
        type="button"
        @click="load(false)"
      >
        Retry update
      </button>
    </div>
    <p
      v-if="state==='loading'"
      role="status"
    >
      Loading ranked articles…
    </p><div
      v-else-if="state==='error'"
      role="alert"
    >
      <p>{{ queryError }}</p><button
        type="button"
        @click="load()"
      >
        Try again
      </button>
    </div><div v-else-if="state==='empty'">
      <p>No articles match these filters.</p><AppLink to="/sources">
        Manage sources or refresh news
      </AppLink>
    </div>
    <ol
      v-else
      class="ranked-list"
    >
      <li
        v-for="article in items"
        :key="article.id"
      >
        <ArticleSummaryCard
          :article="article"
          :source-name="sourceNames[article.sourceId] ?? 'Unknown source'"
          heading-level="h2"
        /><p v-if="article.publishedAt">
          Published {{ new Date(article.publishedAt).toLocaleString() }}
        </p><RankingExplanation
          :contributions="article.ranking.contributions"
          heading-level="h3"
        /><AppLink :to="`/articles/${encodeURIComponent(article.id)}`">
          Open reader
        </AppLink><LibraryActions
          :article="article"
          :busy="actionBusy[article.id]"
          :message="actionMessages[article.id]"
          @mutate="mutate(article,$event)"
        />
      </li>
    </ol>
    <p
      v-if="pageError"
      role="alert"
    >
      {{ pageError }}
    </p><button
      v-if="cursor"
      type="button"
      :disabled="loadingMore"
      @click="more"
    >
      {{ loadingMore ? 'Loading more…' : 'Load more' }}
    </button>
  </section>
</template>
<style scoped>.feed-filters{display:flex;flex-wrap:wrap;gap:1rem;align-items:end}.feed-filters label{display:grid}.ranked-list{padding:0;list-style:none}.ranked-list>li{border-block-end:1px solid var(--border,#777);padding-block:1rem}</style>
