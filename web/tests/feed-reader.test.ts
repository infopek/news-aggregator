// @vitest-environment happy-dom
import axe from 'axe-core'
import { flushPromises, mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'
import type { ArticleDetail, ArticleSummary, FeedPage, LibraryState } from '../src/api/generated/models'
import type { ServerApi } from '../src/api/server-api'
import RankedFeed from '../src/features/feed/RankedFeed.vue'
import ArticleReader from '../src/features/reader/ArticleReader.vue'

const library: LibraryState = { articleId: 'article-a', readAt: null, savedAt: null, hiddenAt: null }
function article(id: string, score: number, permission: ArticleSummary['contentPermission'] = 'metadata_only'): ArticleSummary {
  return { id, sourceId: 'opaque-source', canonicalUrl: `https://publisher.example/${id}`, title: id === 'article-a' ? 'Top ranked story' : 'Second story', author: 'Reporter', publishedAt: '2026-08-14T10:00:00Z', fetchedAt: '2026-08-14T10:01:00Z', excerpt: 'A useful excerpt.', contentPermission: permission, language: 'en', topics: ['science'], library: { ...library, articleId: id }, ranking: { score, algorithmVersion: 'v1', calculatedAt: '2026-08-14T10:02:00Z', contributions: [{ signal: 'interest', rawScore: .8, weight: .5, weightedScore: .4, reasonCode: 'interest_match', reasonValues: { interest: 'science' } }] } }
}
function fakeApi(detail?: ArticleDetail) {
  const pages: FeedPage[] = [{ items: [article('article-a', .9), article('article-b', .7)], nextCursor: 'next' }, { items: [article('article-c', .6)], nextCursor: null }]
  return { feed: vi.fn(async (query) => query.cursor ? pages[1] : pages[0]), sources: vi.fn(async () => ({ items: [{ id: 'opaque-source', name: 'Science Wire', url: 'https://publisher.example/feed', kind: 'feed', enabled: true, contentPermission: 'metadata_only', adapterConfig: { format: 'rss' }, scraperPolicy: { status: 'not_applicable', termsUrl: null, robotsUrl: null, reviewedAt: null, reviewNotes: null }, credentialConfigured: false, lastSuccessAt: null, lastError: null, retryAfter: null }] })), updateLibrary: vi.fn(async (id, patch) => ({ articleId: id, readAt: patch.read ? '2026-08-14T11:00:00Z' : null, savedAt: patch.saved ? '2026-08-14T11:00:00Z' : null, hiddenAt: patch.hidden ? '2026-08-14T11:00:00Z' : null })), article: vi.fn(async () => detail ?? { article: article('article-a', .9), fullContent: null }) } as unknown as ServerApi
}

describe('ranked feed', () => {
  it('preserves server order, explanations, source metadata, filters, and pagination', async () => {
    const api = fakeApi(), wrapper = mount(RankedFeed, { props: { serverApi: api }, attachTo: document.body }); await flushPromises()
    expect(wrapper.findAll('.ranked-list>li').map((item) => item.text())).toEqual([expect.stringContaining('Top ranked story'), expect.stringContaining('Second story')])
    expect(wrapper.text()).toContain('Science Wire'); expect(wrapper.text()).toContain('Matches an interest')
    await wrapper.get('#source-filter').setValue('opaque-source'); await wrapper.get('#read-filter').setValue('unread'); await wrapper.get('input[type=search], input[maxlength="200"]').setValue('science')
    await wrapper.get('.feed-filters').trigger('submit'); await flushPromises()
    expect(api.feed).toHaveBeenLastCalledWith(expect.objectContaining({ sourceId: ['opaque-source'], read: false, text: 'science' }), expect.any(AbortSignal))
    await wrapper.findAll('button').find((item) => item.text() === 'Load more')!.trigger('click'); await flushPromises(); expect(wrapper.findAll('.ranked-list>li')).toHaveLength(3)
    expect((await axe.run(wrapper.element)).violations).toEqual([]); wrapper.unmount()
  })

  it('uses authoritative library responses and removes a hidden article', async () => {
    const api = fakeApi(), wrapper = mount(RankedFeed, { props: { serverApi: api } }); await flushPromises()
    const first = wrapper.findAll('.ranked-list>li')[0]
    await first.findAll('button').find((item) => item.text() === 'Save')!.trigger('click'); await flushPromises(); expect(first.text()).toContain('Unsave')
    await first.findAll('button').find((item) => item.text() === 'Hide')!.trigger('click'); await flushPromises(); expect(wrapper.text()).not.toContain('Top ranked story')
    wrapper.unmount()
  })
})

describe('permission-aware reader', () => {
  it('never renders metadata-only body and provides the canonical publisher link', async () => {
    const wrapper = mount(ArticleReader, { props: { articleId: 'article-a', serverApi: fakeApi() }, attachTo: document.body }); await flushPromises()
    expect(wrapper.text()).toContain('body is not stored or displayed'); expect(wrapper.get('a[href="https://publisher.example/article-a"]').exists()).toBe(true)
    expect((await axe.run(wrapper.element)).violations).toEqual([]); wrapper.unmount()
  })

  it('renders permitted content as inert text so malicious markup cannot execute', async () => {
    const malicious = '<img src=x onerror=alert(1)><script>globalThis.pwned=true</script>Permitted text'
    const detail: ArticleDetail = { article: article('article-a', .9, 'full_content_allowed'), fullContent: malicious }
    const wrapper = mount(ArticleReader, { props: { articleId: 'article-a', serverApi: fakeApi(detail) } }); await flushPromises()
    expect(wrapper.get('.article-content').text()).toBe(malicious); expect(wrapper.find('.article-content img').exists()).toBe(false); expect(wrapper.find('.article-content script').exists()).toBe(false)
    wrapper.unmount()
  })
})
