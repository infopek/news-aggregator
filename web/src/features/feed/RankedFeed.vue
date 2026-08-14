<script setup lang="ts">
/* global AbortController */
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { api as client } from '../../api/client'
import { createServerApi, type ServerApi } from '../../api/server-api'
import type { ArticleSummary, FeedQuery, LibraryState, Source } from '../../api/generated/models'
import ArticleSummaryCard from '../../components/shared/ArticleSummaryCard.vue'
import RankingExplanation from '../../components/shared/RankingExplanation.vue'
import FilterControl from '../../components/shared/FilterControl.vue'
import AppLink from '../../router/AppLink.vue'
import { toUserSafeError } from '../../state/errors'
import LibraryActions from './LibraryActions.vue'
import RefreshControl from '../refresh/RefreshControl.vue'

const props = withDefaults(defineProps<{ serverApi?: ServerApi }>(), { serverApi: undefined })
const server = props.serverApi ?? createServerApi(client)
const items = ref<ArticleSummary[]>([]), sources = ref<Source[]>([]), cursor = ref<string|null>(null)
const state = ref<'loading'|'ready'|'empty'|'error'>('loading'), loadingMore = ref(false), error = ref('')
const source = ref(''), read = ref('all'), saved = ref('all'), text = ref(''), after = ref(''), before = ref('')
type FilterSnapshot = { source: string; read: string; saved: string; text: string; after: string; before: string }
const applied = ref<FilterSnapshot>({ source: '', read: 'all', saved: 'all', text: '', after: '', before: '' })
let generation = 0, loadController: AbortController | undefined, pageController: AbortController | undefined
const refreshing = ref(false)
const sourceNames = computed(() => Object.fromEntries(sources.value.map((item) => [item.id, item.name])))
onMounted(() => load(false)); onBeforeUnmount(() => { loadController?.abort(); pageController?.abort() })
function draft(): FilterSnapshot { return { source: source.value, read: read.value, saved: saved.value, text: text.value, after: after.value, before: before.value } }
function query(filter: FilterSnapshot, next?: string): FeedQuery { return { cursor: next || undefined, limit: 20, sourceId: filter.source ? [filter.source] : undefined, read: filter.read === 'all' ? undefined : filter.read === 'read', saved: filter.saved === 'saved' ? true : undefined, text: filter.text.trim() || undefined, publishedAfter: filter.after ? new Date(`${filter.after}T00:00:00`).toISOString() : undefined, publishedBefore: filter.before ? new Date(`${filter.before}T23:59:59`).toISOString() : undefined } }
async function load(applyDraft = true) {
  if (applyDraft) applied.value = draft()
  const requestGeneration = ++generation; loadController?.abort(); pageController?.abort(); loadController = new AbortController()
  state.value = 'loading'; error.value = ''
  try { const [page, list] = await Promise.all([server.feed(query(applied.value), loadController.signal), server.sources(loadController.signal)]); if(requestGeneration!==generation)return; items.value = page.items; cursor.value = page.nextCursor; sources.value = list.items; state.value = items.value.length ? 'ready' : 'empty' } catch (cause) { if(requestGeneration!==generation)return; error.value = toUserSafeError(cause).message; state.value = 'error' }
}
async function more() { if (!cursor.value) return; const requestGeneration=generation, next=cursor.value, filter={...applied.value}; pageController?.abort(); pageController=new AbortController(); loadingMore.value = true; try { const page = await server.feed(query(filter,next),pageController.signal); if(requestGeneration!==generation)return; items.value.push(...page.items); cursor.value = page.nextCursor } catch (cause) { if(requestGeneration===generation)error.value = toUserSafeError(cause).message } finally { if(requestGeneration===generation)loadingMore.value = false } }
function reconcile(article: ArticleSummary, library: LibraryState) { article.library = library; if ((applied.value.saved==='saved'&&!library.savedAt)||(applied.value.read==='unread'&&library.readAt)||(applied.value.read==='read'&&!library.readAt)) hide(article.id) }
function hide(id: string) { items.value = items.value.filter((item) => item.id !== id); if (!items.value.length) state.value = 'empty' }
async function refreshed(){refreshing.value=false;await load(false)}
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
      v-if="refreshing"
      role="status"
    >
      Refresh is running. Current articles may be stale until it completes.
    </p>
    <RefreshControl
      :server="server"
      :source-names="sourceNames"
      @started="refreshing=true"
      @completed="refreshed"
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
      v-if="state==='loading'"
      role="status"
    >
      Loading ranked articles…
    </p><div
      v-else-if="state==='error'"
      role="alert"
    >
      <p>{{ error }}</p><button
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
          :server="server"
          @updated="reconcile(article,$event)"
          @hidden="hide(article.id)"
        />
      </li>
    </ol>
    <p
      v-if="error"
      role="alert"
    >
      {{ error }}
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
