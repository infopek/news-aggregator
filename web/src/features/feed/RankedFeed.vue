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

const props = withDefaults(defineProps<{ serverApi?: ServerApi }>(), { serverApi: undefined })
const server = props.serverApi ?? createServerApi(client)
const items = ref<ArticleSummary[]>([]), sources = ref<Source[]>([]), cursor = ref<string|null>(null)
const state = ref<'loading'|'ready'|'empty'|'error'>('loading'), loadingMore = ref(false), error = ref('')
const source = ref(''), read = ref('all'), saved = ref('all'), text = ref(''), after = ref(''), before = ref('')
const controller = new AbortController()
const sourceNames = computed(() => Object.fromEntries(sources.value.map((item) => [item.id, item.name])))
onMounted(load); onBeforeUnmount(() => controller.abort())
function query(next?: string): FeedQuery { return { cursor: next || undefined, limit: 20, sourceId: source.value ? [source.value] : undefined, read: read.value === 'all' ? undefined : read.value === 'read', saved: saved.value === 'saved' ? true : undefined, text: text.value.trim() || undefined, publishedAfter: after.value ? new Date(`${after.value}T00:00:00`).toISOString() : undefined, publishedBefore: before.value ? new Date(`${before.value}T23:59:59`).toISOString() : undefined } }
async function load() {
  state.value = 'loading'; error.value = ''
  try { const [page, list] = await Promise.all([server.feed(query(), controller.signal), server.sources(controller.signal)]); items.value = page.items; cursor.value = page.nextCursor; sources.value = list.items; state.value = items.value.length ? 'ready' : 'empty' } catch (cause) { error.value = toUserSafeError(cause).message; state.value = 'error' }
}
async function more() { if (!cursor.value) return; loadingMore.value = true; try { const page = await server.feed(query(cursor.value)); items.value.push(...page.items); cursor.value = page.nextCursor } catch (cause) { error.value = toUserSafeError(cause).message } finally { loadingMore.value = false } }
function reconcile(article: ArticleSummary, library: LibraryState) { article.library = library }
function hide(id: string) { items.value = items.value.filter((item) => item.id !== id); if (!items.value.length) state.value = 'empty' }
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
    <form
      class="feed-filters"
      @submit.prevent="load"
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
        @click="source='';read='all';saved='all';text='';after='';before='';load()"
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
        @click="load"
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
