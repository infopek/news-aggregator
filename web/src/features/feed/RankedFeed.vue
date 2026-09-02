<script setup lang="ts">
/* global AbortController */
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { api as client } from '../../api/client'
import { createServerApi, type ServerApi } from '../../api/server-api'
import type { ArticleSummary, FeedQuery, LibraryStateWrite, Source } from '../../api/generated/models'
import ArticleSummaryCard from '../../components/shared/ArticleSummaryCard.vue'
import RankingExplanation from '../../components/shared/RankingExplanation.vue'
import FilterControl from '../../components/shared/FilterControl.vue'
import EmptyState from '../../components/shared/EmptyState.vue'
import StatusBanner from '../../components/shared/StatusBanner.vue'
import AppLink from '../../router/AppLink.vue'
import { toUserSafeError } from '../../state/errors'
import { ServerMutations } from '../../state/mutations'
import { ServerStateClient } from '../../state/query-client'
import LibraryActions from './LibraryActions.vue'
import { publishLibraryInvalidation, subscribeLibraryInvalidation } from './library-invalidation'
import RefreshControl from '../refresh/RefreshControl.vue'

const props = withDefaults(defineProps<{ serverApi?: ServerApi }>(), { serverApi: undefined })
const server = props.serverApi ?? createServerApi(client)
const mutations = new ServerMutations(server, new ServerStateClient())
const items = ref<ArticleSummary[]>([]), sources = ref<Source[]>([]), cursor = ref<string|null>(null)
const state = ref<'loading'|'ready'|'empty'|'error'>('loading'), loadingMore = ref(false), queryError = ref(''), pageError = ref('')
const revalidating = ref(false), stale = ref(false), paginationBlocked=ref(false)
const actionBusy = ref<Record<string, boolean>>({}), actionMessages = ref<Record<string, string>>({})
const source = ref(''), read = ref('all'), saved = ref('all'), text = ref(''), after = ref(''), before = ref('')
type FilterSnapshot = { source: string; read: string; saved: string; text: string; after: string; before: string }
const applied = ref<FilterSnapshot>({ source: '', read: 'all', saved: 'all', text: '', after: '', before: '' })
let generation = 0, loadController: AbortController | undefined, pageController: AbortController | undefined, mounted=true
let unsubscribeInvalidation: (()=>void) | undefined
const refreshState = ref<'idle'|'running'|'uncertain'>('idle')
const sourceNames = computed(() => Object.fromEntries(sources.value.map((item) => [item.id, item.name])))
const filtersActive = computed(() => Boolean(source.value || read.value !== 'all' || saved.value !== 'all' || text.value.trim() || after.value || before.value))
onMounted(() => {unsubscribeInvalidation=subscribeLibraryInvalidation(()=>{void load(false)});void restoreAndLoad()}); onBeforeUnmount(() => { mounted=false;++generation;unsubscribeInvalidation?.();loadController?.abort();pageController?.abort() })
async function restoreAndLoad() {
  try {
    const persisted = await server.feedFilter()
    source.value=persisted.sourceId;read.value=persisted.read;saved.value=persisted.savedOnly?'saved':'all';text.value=persisted.searchQuery
    applied.value=draft()
  } catch { applied.value=draft() }
  if(mounted)await load(false)
}
function draft(): FilterSnapshot { return { source: source.value, read: read.value, saved: saved.value, text: text.value, after: after.value, before: before.value } }
function query(filter: FilterSnapshot, next?: string): FeedQuery { return { cursor: next || undefined, limit: 20, sourceId: filter.source ? [filter.source] : undefined, read: filter.read === 'all' ? undefined : filter.read === 'read', saved: filter.saved === 'saved' ? true : undefined, text: filter.text.trim() || undefined, publishedAfter: filter.after ? new Date(`${filter.after}T00:00:00`).toISOString() : undefined, publishedBefore: filter.before ? new Date(`${filter.before}T23:59:59`).toISOString() : undefined } }
async function load(applyDraft = true) {
  const requested=applyDraft?draft():{...applied.value}, changed=JSON.stringify(requested)!==JSON.stringify(applied.value)
  const requestGeneration = ++generation; loadController?.abort(); pageController?.abort(); loadController = new AbortController(); loadingMore.value=false; pageError.value=''
  paginationBlocked.value=true;if(changed)cursor.value=null
  const hasData = !changed && (state.value === 'ready' || state.value === 'empty')
  if (hasData) revalidating.value = true
  else state.value = 'loading'
  queryError.value = ''; stale.value = false
  try { if(applyDraft)await server.updateFeedFilter({sourceId:requested.source,read:requested.read as 'all'|'read'|'unread',savedOnly:requested.saved==='saved',includeHidden:false,searchQuery:requested.text.trim()},loadController.signal);const [page,list]=await Promise.all([server.feed(query(requested),loadController.signal),server.sources(loadController.signal)]); if(requestGeneration!==generation)return; applied.value=requested;items.value = page.items; cursor.value = page.nextCursor; sources.value = list.items; state.value = items.value.length ? 'ready' : 'empty'; stale.value = false } catch (cause) { if(requestGeneration!==generation)return; queryError.value = toUserSafeError(cause).message; if(hasData)stale.value=true;else state.value='error' } finally { if(requestGeneration===generation){revalidating.value=false;paginationBlocked.value=false} }
}
async function more() { if (!cursor.value||paginationBlocked.value) return; const requestGeneration=generation, next=cursor.value, filter={...applied.value}; pageController?.abort(); pageController=new AbortController(); loadingMore.value = true; pageError.value=''; try { const page = await server.feed(query(filter,next),pageController.signal); if(requestGeneration!==generation||paginationBlocked.value)return; items.value.push(...page.items); cursor.value = page.nextCursor; pageError.value='' } catch (cause) { if(requestGeneration===generation)pageError.value = toUserSafeError(cause).message } finally { if(requestGeneration===generation)loadingMore.value = false } }
function hide(id: string) { items.value = items.value.filter((item) => item.id !== id); if (!items.value.length) state.value = 'empty' }
async function mutate(article: ArticleSummary, patch: LibraryStateWrite) {
  actionBusy.value[article.id] = true; actionMessages.value[article.id] = ''
  const result = await mutations.updateLibrary(article.id, patch)
  publishLibraryInvalidation(article.id)
  if (!mounted) return
  if (result.status === 'success') {
    article.library = result.data; actionMessages.value[article.id] = 'Article state updated.'
    if (result.data.hiddenAt || (applied.value.saved==='saved'&&!result.data.savedAt)||(applied.value.read==='unread'&&result.data.readAt)||(applied.value.read==='read'&&!result.data.readAt)) hide(article.id)
  } else actionMessages.value[article.id] = result.error.message
  actionBusy.value[article.id] = false
}
async function refreshed(){refreshState.value='idle';await load(false)}
function clear(){source.value='';read.value='all';saved.value='all';text.value='';after.value='';before.value='';void load()}
</script>
<template>
  <section
    class="feed-page"
    aria-labelledby="feed-title"
  >
    <header class="feed-header">
      <div class="feed-heading">
        <p class="eyebrow">
          Your local news
        </p>
        <h1
          id="feed-title"
          tabindex="-1"
        >
          Ranked feed
        </h1>
        <p>Stories ordered using your preferences on this computer.</p>
      </div>
      <RefreshControl
        :server="server"
        :source-names="sourceNames"
        compact
        @started="refreshState='running'"
        @completed="refreshed"
        @stopped="refreshState='uncertain'"
      />
    </header>

    <StatusBanner
      v-if="refreshState==='running'"
      title="Refreshing your sources"
      live
    >
      Current stories remain available while new articles are checked.
    </StatusBanner>
    <StatusBanner
      v-else-if="refreshState==='uncertain'"
      title="Refresh status unavailable"
      tone="warning"
      live
    >
      Current stories may be out of date. Retry the saved refresh status or start a new refresh.
    </StatusBanner>

    <form
      class="feed-filters"
      aria-label="Filter ranked stories"
      @submit.prevent="load()"
    >
      <div class="feed-filters__main">
        <FilterControl
          id="source-filter"
          v-model="source"
          label="Source"
          :options="[{value:'',label:'All sources'},...sources.map(item=>({value:item.id,label:item.name}))]"
        />
        <FilterControl
          id="read-filter"
          v-model="read"
          label="Reading status"
          :options="[{value:'all',label:'All stories'},{value:'unread',label:'Unread'},{value:'read',label:'Read'}]"
        />
        <FilterControl
          id="saved-filter"
          v-model="saved"
          label="Saved"
          :options="[{value:'all',label:'All stories'},{value:'saved',label:'Saved only'}]"
        />
        <label
          class="feed-search"
          for="feed-search"
        >Search stories<input
          id="feed-search"
          v-model="text"
          type="search"
          maxlength="200"
          placeholder="Topics, titles, or keywords…"
        ></label>
      </div>
      <details
        class="more-filters"
        :open="Boolean(after || before)"
      >
        <summary>More filters</summary>
        <div class="more-filters__fields">
          <label for="published-after">Published after<input
            id="published-after"
            v-model="after"
            type="date"
          ></label>
          <label for="published-before">Published before<input
            id="published-before"
            v-model="before"
            type="date"
          ></label>
        </div>
      </details>
      <div class="feed-filters__actions">
        <button type="submit">
          Apply filters
        </button>
        <button
          v-if="filtersActive"
          type="button"
          class="tertiary"
          @click="clear"
        >
          Clear filters
        </button>
      </div>
    </form>

    <p
      v-if="revalidating"
      class="feed-update"
      role="status"
    >
      Updating ranked stories… Current results remain visible.
    </p>
    <StatusBanner
      v-if="stale"
      title="Showing your last results"
      tone="warning"
    >
      {{ queryError }}
      <template #actions>
        <button
          type="button"
          class="secondary"
          @click="load(false)"
        >
          Retry update
        </button>
      </template>
    </StatusBanner>
    <div
      v-if="state==='loading'"
      class="feed-loading"
      role="status"
    >
      Loading ranked stories…
    </div>
    <StatusBanner
      v-else-if="state==='error'"
      title="The ranked feed is unavailable"
      tone="danger"
    >
      {{ queryError }}
      <template #actions>
        <button
          type="button"
          class="secondary"
          @click="load()"
        >
          Try again
        </button>
      </template>
    </StatusBanner>
    <EmptyState
      v-else-if="state==='empty'"
      title="No stories found"
      :description="filtersActive ? 'No stories match these filters. Try clearing one or more filters.' : 'Add or refresh a source to bring stories into your feed.'"
    >
      <div class="actions">
        <button
          v-if="filtersActive"
          type="button"
          class="secondary"
          @click="clear"
        >
          Clear filters
        </button><AppLink
          class="button-link"
          to="/sources"
        >
          Manage sources
        </AppLink>
      </div>
    </EmptyState>
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
          :reader-to="`/articles/${encodeURIComponent(article.id)}`"
        >
          <template #explanation>
            <RankingExplanation :contributions="article.ranking.contributions" />
          </template>
          <template #actions>
            <LibraryActions
              :article="article"
              :busy="actionBusy[article.id]"
              :message="actionMessages[article.id]"
              @mutate="mutate(article,$event)"
            />
          </template>
        </ArticleSummaryCard>
      </li>
    </ol>
    <StatusBanner
      v-if="pageError"
      title="Could not load more stories"
      tone="danger"
    >
      {{ pageError }}
    </StatusBanner>
    <div
      v-if="cursor"
      class="load-more"
    >
      <button
        type="button"
        class="secondary"
        :disabled="loadingMore || paginationBlocked"
        @click="more"
      >
        {{ loadingMore ? 'Loading more…' : 'Load more stories' }}
      </button>
    </div>
  </section>
</template>
<style scoped>
.feed-page { display: grid; gap: var(--space-5); }
.feed-header { display: flex; align-items: flex-start; justify-content: space-between; gap: var(--space-6); }
.feed-heading { display: grid; gap: var(--space-2); }
.feed-heading > p:last-child { color: var(--color-muted); font-size: 1.02rem; }
.feed-filters { display: grid; gap: var(--space-4); padding: var(--space-4); border: 1px solid var(--color-border); border-radius: var(--radius-lg); background: var(--color-surface-soft); }
.feed-filters__main { display: grid; grid-template-columns: repeat(3, minmax(9rem, .7fr)) minmax(15rem, 1.6fr); gap: var(--space-3); align-items: end; }
.feed-search, .more-filters label { display: grid; gap: var(--space-1); color: var(--color-ink); font-weight: 720; }
.feed-search input, .more-filters input { width: 100%; min-height: var(--control-height); padding: .65rem .75rem; border: 1px solid var(--color-border-strong); border-radius: var(--radius-md); background: var(--color-surface); color: var(--color-ink); }
.more-filters { border-top: 1px solid var(--color-border); padding-top: var(--space-3); }
.more-filters summary { width: fit-content; min-height: 2.75rem; display: flex; align-items: center; color: var(--color-brand); cursor: pointer; font-weight: 720; }
.more-filters__fields { display: grid; grid-template-columns: repeat(2, minmax(12rem, 18rem)); gap: var(--space-3); padding-top: var(--space-2); }
.feed-filters__actions { display: flex; flex-wrap: wrap; gap: var(--space-2); }
.feed-update { color: var(--color-muted); }
.feed-loading { min-height: 12rem; display: grid; place-items: center; color: var(--color-muted); }
.ranked-list { display: grid; gap: var(--space-4); margin: 0; padding: 0; list-style: none; }
.load-more { display: flex; justify-content: center; }
@media (max-width: 62rem) { .feed-filters__main { grid-template-columns: repeat(2, minmax(0, 1fr)); } }
@media (max-width: 40rem) { .feed-header { flex-direction: column; } .feed-filters__main, .more-filters__fields { grid-template-columns: 1fr; } .feed-filters__actions > * { flex: 1 1 auto; } }
</style>
