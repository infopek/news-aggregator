// @vitest-environment happy-dom
import axe from 'axe-core'
import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { ArticleDetail, ArticleSummary, FeedPage, LibraryState } from '../src/api/generated/models'
import type { ServerApi } from '../src/api/server-api'
import { ApiRequestError } from '../src/api/client'
import RankedFeed from '../src/features/feed/RankedFeed.vue'
import ArticleReader from '../src/features/reader/ArticleReader.vue'

const library: LibraryState = { articleId: 'article-a', readAt: null, savedAt: null, hiddenAt: null }
function article(id: string, score: number, permission: ArticleSummary['contentPermission'] = 'metadata_only'): ArticleSummary {
  return { id, sourceId: 'opaque-source', canonicalUrl: `https://publisher.example/${id}`, title: id === 'article-a' ? 'Top ranked story' : id === 'article-b' ? 'Second story' : id, author: 'Reporter', publishedAt: '2026-08-14T10:00:00Z', fetchedAt: '2026-08-14T10:01:00Z', excerpt: 'A useful excerpt.', contentPermission: permission, language: 'en', topics: ['science'], library: { ...library, articleId: id }, ranking: { score, algorithmVersion: 'v1', calculatedAt: '2026-08-14T10:02:00Z', contributions: [{ signal: 'interest', rawScore: .8, weight: .5, weightedScore: .4, reasonCode: 'interest_match', reasonValues: { interest: 'science' } }] } }
}
function fakeApi(detail?: ArticleDetail) {
  const pages: FeedPage[] = [{ items: [article('article-a', .9), article('article-b', .7)], nextCursor: 'next' }, { items: [article('article-c', .6)], nextCursor: null }]
  const states = new Map<string, LibraryState>()
  const feed = vi.fn(async (query) => {
    if (query.cursor) return pages[1]
    const visible = pages[0].items.map(item => ({ ...item, library: states.get(item.id) ?? item.library })).filter(item => !item.library.hiddenAt && (query.saved !== true || item.library.savedAt) && (query.read === undefined || Boolean(item.library.readAt) === query.read))
    return { items: visible, nextCursor: 'next' }
  })
  const updateLibrary = vi.fn(async (id, patch) => { const previous=states.get(id)??{...library,articleId:id};const next={articleId:id,readAt:patch.read===undefined?previous.readAt:patch.read?'2026-08-14T11:00:00Z':null,savedAt:patch.saved===undefined?previous.savedAt:patch.saved?'2026-08-14T11:00:00Z':null,hiddenAt:patch.hidden===undefined?previous.hiddenAt:patch.hidden?'2026-08-14T11:00:00Z':null};states.set(id,next);return next })
  return { feed, sources: vi.fn(async () => ({ items: [{ id: 'opaque-source', name: 'Science Wire', url: 'https://publisher.example/feed', kind: 'feed', enabled: true, contentPermission: 'metadata_only', adapterConfig: { format: 'rss' }, scraperPolicy: { status: 'not_applicable', termsUrl: null, robotsUrl: null, reviewedAt: null, reviewNotes: null }, credentialConfigured: false, lastSuccessAt: null, lastError: null, retryAfter: null }] })), updateLibrary, article: vi.fn(async () => detail ?? { article: article('article-a', .9), fullContent: null }), startRefresh: vi.fn(async () => ({ id: 'feed-refresh', status: 'running', startedAt: '2026-08-14T10:00:00Z', finishedAt: null, outcomes: [] })), refresh: vi.fn(async () => ({ id: 'feed-refresh', status: 'partial_success', startedAt: '2026-08-14T10:00:00Z', finishedAt: '2026-08-14T10:01:00Z', outcomes: [] })) } as unknown as ServerApi
}

beforeEach(() => localStorage.clear())

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
    await first.findAll('button').find((item) => item.text() === 'Save')!.trigger('click'); await flushPromises(); expect(api.feed).toHaveBeenCalledTimes(2)
    await wrapper.findAll('.ranked-list>li')[0].findAll('button').find((item) => item.text() === 'Hide')!.trigger('click'); await flushPromises(); expect(wrapper.text()).not.toContain('Top ranked story')
    wrapper.unmount()
  })

  it('reloads authoritative order after a terminal refresh', async () => {
    const api = fakeApi(), wrapper = mount(RankedFeed, { props: { serverApi: api } }); await flushPromises()
    await wrapper.findAll('button').find((item) => item.text().includes('Refresh all'))!.trigger('click'); await flushPromises()
    expect(api.startRefresh).toHaveBeenCalledOnce(); expect(api.feed).toHaveBeenCalledTimes(2); expect(wrapper.text()).toContain('some source failures')
    wrapper.unmount()
  })

  it('binds pagination to applied filters and ignores obsolete page responses', async () => {
    const api = fakeApi(); let releasePage!: (value: FeedPage) => void
    vi.mocked(api.feed).mockImplementation(async (query) => query.cursor ? new Promise(resolve => { releasePage = resolve }) : { items: [article(query.sourceId?.[0] === 'opaque-source' ? 'filtered' : 'initial', .9)], nextCursor: 'cursor' })
    const wrapper = mount(RankedFeed, { props: { serverApi: api } }); await flushPromises()
    await wrapper.get('#source-filter').setValue('opaque-source')
    await wrapper.findAll('button').find((item) => item.text() === 'Load more')!.trigger('click'); await flushPromises()
    expect(api.feed).toHaveBeenLastCalledWith(expect.objectContaining({ cursor: 'cursor', sourceId: undefined }), expect.any(AbortSignal))
    await wrapper.get('.feed-filters').trigger('submit'); await flushPromises()
    releasePage({ items: [article('obsolete', .5)], nextCursor: null }); await flushPromises()
    expect(wrapper.text()).toContain('filtered'); expect(wrapper.text()).not.toContain('obsolete')
    wrapper.unmount()
  })

  it('withholds the old cursor while a replacement filter query is pending', async () => {
    const api=fakeApi();let resolveFiltered!:(page:FeedPage)=>void
    vi.mocked(api.feed).mockResolvedValueOnce({items:[article('initial',.9)],nextCursor:'cursor-a'}).mockImplementationOnce(async()=>new Promise(resolve=>{resolveFiltered=resolve})).mockResolvedValueOnce({items:[article('page-b',.6)],nextCursor:null})
    const wrapper=mount(RankedFeed,{props:{serverApi:api}});await flushPromises();await wrapper.get('#source-filter').setValue('opaque-source');await wrapper.get('.feed-filters').trigger('submit');await flushPromises();expect(wrapper.findAll('button').some(item=>item.text()==='Load more')).toBe(false);expect(api.feed).toHaveBeenCalledTimes(2)
    resolveFiltered({items:[article('filtered',.8)],nextCursor:'cursor-b'});await flushPromises();await wrapper.findAll('button').find(item=>item.text()==='Load more')!.trigger('click');await flushPromises();expect(api.feed).toHaveBeenLastCalledWith(expect.objectContaining({cursor:'cursor-b',sourceId:['opaque-source']}),expect.any(AbortSignal));expect(wrapper.text()).toContain('page-b');wrapper.unmount()
  })

  it('does not retain an old cursor when a replacement filter query fails', async () => {
    const api=fakeApi(),unavailable=new ApiRequestError(503,{code:'unavailable',message:'Down',correlationId:'safe',fields:[]})
    vi.mocked(api.feed).mockResolvedValueOnce({items:[article('initial',.9)],nextCursor:'cursor-a'}).mockRejectedValueOnce(unavailable)
    const wrapper=mount(RankedFeed,{props:{serverApi:api}});await flushPromises();await wrapper.get('#source-filter').setValue('opaque-source');await wrapper.get('.feed-filters').trigger('submit');await flushPromises();expect(wrapper.findAll('button').some(item=>item.text()==='Load more')).toBe(false);expect(wrapper.text()).not.toContain('may be stale');expect(api.feed).toHaveBeenCalledTimes(2);wrapper.unmount()
  })

  it.each([['saved', 'Unsave'], ['unread', 'Mark read']] as const)('removes mutations that no longer match %s filter', async (filter, action) => {
    const api = fakeApi()
    if(filter==='saved')await api.updateLibrary('article-a',{saved:true})
    const wrapper = mount(RankedFeed, { props: { serverApi: api } }); await flushPromises()
    await wrapper.get(filter === 'saved' ? '#saved-filter' : '#read-filter').setValue(filter); await wrapper.get('.feed-filters').trigger('submit'); await flushPromises()
    await wrapper.findAll('button').find((item) => item.text() === action)!.trigger('click'); await flushPromises()
    expect(wrapper.text()).not.toContain('Top ranked story'); wrapper.unmount()
  })

  it('clears cancelled page loading and permits pagination for the new query', async () => {
    const api=fakeApi();let release!: (page:FeedPage)=>void
    vi.mocked(api.feed).mockImplementation(async query=>query.cursor?new Promise(resolve=>{release=resolve}):{items:[article('current',.8)],nextCursor:'new-cursor'})
    const wrapper=mount(RankedFeed,{props:{serverApi:api}});await flushPromises();await wrapper.findAll('button').find(item=>item.text()==='Load more')!.trigger('click');await wrapper.get('#source-filter').setValue('opaque-source');await wrapper.get('.feed-filters').trigger('submit');await flushPromises()
    expect(wrapper.findAll('button').find(item=>item.text()==='Load more')!.attributes('disabled')).toBeUndefined();release({items:[],nextCursor:null});wrapper.unmount()
  })

  it('shows one query error and clears a pagination error after retry', async () => {
    const unavailable=new ApiRequestError(503,{code:'unavailable',message:'Down',correlationId:'safe',fields:[]}), api=fakeApi();vi.mocked(api.feed).mockRejectedValueOnce(unavailable)
    const wrapper=mount(RankedFeed,{props:{serverApi:api}});await flushPromises();expect(wrapper.findAll('[role=alert]')).toHaveLength(1)
    vi.mocked(api.feed).mockResolvedValueOnce({items:[article('current',.8)],nextCursor:'cursor'});await wrapper.findAll('button').find(item=>item.text()==='Try again')!.trigger('click');await flushPromises()
    vi.mocked(api.feed).mockRejectedValueOnce(unavailable);await wrapper.findAll('button').find(item=>item.text()==='Load more')!.trigger('click');await flushPromises();expect(wrapper.findAll('[role=alert]')).toHaveLength(1)
    vi.mocked(api.feed).mockResolvedValueOnce({items:[article('next-page',.7)],nextCursor:null});await wrapper.findAll('button').find(item=>item.text()==='Load more')!.trigger('click');await flushPromises();expect(wrapper.findAll('[role=alert]')).toHaveLength(0);expect(wrapper.text()).toContain('next-page');wrapper.unmount()
  })

  it('refreshes server order and explanations after an inline mutation', async () => {
    const api=fakeApi();vi.mocked(api.feed).mockResolvedValueOnce({items:[article('article-a',.9),article('article-b',.8)],nextCursor:null}).mockResolvedValueOnce({items:[article('article-b',.8),{...article('article-a',.6),ranking:{...article('article-a',.6).ranking,contributions:[]}}],nextCursor:null})
    const wrapper=mount(RankedFeed,{props:{serverApi:api}});await flushPromises();await wrapper.findAll('button').find(item=>item.text()==='Mark read')!.trigger('click');await flushPromises()
    expect(wrapper.findAll('.ranked-list>li')[0].text()).toContain('Second story');expect(wrapper.findAll('.ranked-list>li')[1].text()).toContain('No ranking explanation');wrapper.unmount()
  })

  it('revalidates after a mutation response is lost after commit', async () => {
    const api=fakeApi();let committed=false
    vi.mocked(api.feed).mockImplementation(async()=>({items:[{...article('article-a',.9),library:{...library,readAt:committed?'2026-08-14T11:00:00Z':null}}],nextCursor:null}))
    vi.mocked(api.updateLibrary).mockImplementation(async()=>{committed=true;throw new TypeError('response lost after commit')})
    const wrapper=mount(RankedFeed,{props:{serverApi:api}});await flushPromises();await wrapper.findAll('button').find(item=>item.text()==='Mark read')!.trigger('click');await flushPromises()
    expect(api.feed).toHaveBeenCalledTimes(2);expect(wrapper.text()).toContain('Mark unread');wrapper.unmount()
  })

  it('reconciles both overlapping mutations after action rows are re-rendered', async () => {
    const api=fakeApi(), committed=new Map<string,LibraryState>();let resolveA!:(value:LibraryState)=>void,resolveB!:(value:LibraryState)=>void
    vi.mocked(api.feed).mockImplementation(async()=>({items:['article-a','article-b'].map((id,index)=>({...article(id,.9-index*.1),library:committed.get(id)??{...library,articleId:id}})),nextCursor:null}))
    vi.mocked(api.updateLibrary).mockImplementation(async(id,patch)=>new Promise(resolve=>{const finish=()=>{const value={...library,articleId:id,readAt:patch.read?'2026-08-14T11:00:00Z':null,savedAt:patch.saved?'2026-08-14T11:00:00Z':null};committed.set(id,value);resolve(value)};if(id==='article-a')resolveA=finish;else resolveB=finish}))
    const wrapper=mount(RankedFeed,{props:{serverApi:api}});await flushPromises();const rows=wrapper.findAll('.ranked-list>li');await rows[0].findAll('button').find(item=>item.text()==='Mark read')!.trigger('click');await rows[1].findAll('button').find(item=>item.text()==='Save')!.trigger('click')
    resolveB({} as LibraryState);await flushPromises();resolveA({} as LibraryState);await flushPromises()
    expect(wrapper.findAll('.ranked-list>li')[0].text()).toContain('Mark unread');expect(wrapper.findAll('.ranked-list>li')[1].text()).toContain('Unsave');expect(api.feed).toHaveBeenCalledTimes(3);wrapper.unmount()
  })

  it('retains prior articles as stale when revalidation fails and retries them', async () => {
    const unavailable=new ApiRequestError(503,{code:'unavailable',message:'Down',correlationId:'safe',fields:[]}),api=fakeApi()
    vi.mocked(api.feed).mockResolvedValueOnce({items:[article('article-a',.9)],nextCursor:null}).mockRejectedValueOnce(unavailable).mockResolvedValueOnce({items:[article('article-b',.8)],nextCursor:null})
    const wrapper=mount(RankedFeed,{props:{serverApi:api}});await flushPromises();await wrapper.findAll('button').find(item=>item.text().includes('Refresh all'))!.trigger('click');await flushPromises()
    expect(wrapper.text()).toContain('Top ranked story');expect(wrapper.text()).toContain('may be stale')
    await wrapper.findAll('button').find(item=>item.text()==='Retry update')!.trigger('click');await flushPromises();expect(wrapper.text()).toContain('Second story');expect(wrapper.text()).not.toContain('may be stale');wrapper.unmount()
  })

  it.each([
    ['unavailable', new TypeError('temporary network failure')],
    ['missing', new ApiRequestError(404,{code:'not_found',message:'Missing',correlationId:'safe',fields:[]})]
  ])('stops claiming refresh is running when status is %s', async (_case, failure) => {
    const api=fakeApi();vi.mocked(api.refresh).mockRejectedValueOnce(failure)
    const wrapper=mount(RankedFeed,{props:{serverApi:api}});await flushPromises();await wrapper.findAll('button').find(item=>item.text().includes('Refresh all'))!.trigger('click');await flushPromises()
    expect(wrapper.text()).not.toContain('Refresh is running.');expect(wrapper.text()).toContain('Refresh status is unavailable.');expect(wrapper.text()).toContain('may be stale');wrapper.unmount()
  })

  it('does not query the feed after a pending mutation settles post-unmount', async () => {
    const api=fakeApi();let resolveMutation!:(value:LibraryState)=>void
    vi.mocked(api.updateLibrary).mockImplementation(async()=>new Promise(resolve=>{resolveMutation=resolve}))
    const wrapper=mount(RankedFeed,{props:{serverApi:api}});await flushPromises();await wrapper.findAll('button').find(item=>item.text()==='Mark read')!.trigger('click');wrapper.unmount();resolveMutation({...library,readAt:'2026-08-14T11:00:00Z'});await flushPromises()
    expect(api.feed).toHaveBeenCalledOnce();expect(api.sources).toHaveBeenCalledOnce()
  })

  it('revalidates a remounted feed when an older route mutation settles', async () => {
    const api=fakeApi();let committed=false,resolveMutation!:()=>void
    vi.mocked(api.feed).mockImplementation(async()=>({items:[{...article('article-a',.9),library:{...library,readAt:committed?'2026-08-14T11:00:00Z':null}}],nextCursor:null}))
    vi.mocked(api.updateLibrary).mockImplementation(async()=>new Promise(resolve=>{resolveMutation=()=>{committed=true;resolve({...library,readAt:'2026-08-14T11:00:00Z'})}}))
    const first=mount(RankedFeed,{props:{serverApi:api}});await flushPromises();await first.findAll('button').find(item=>item.text()==='Mark read')!.trigger('click');first.unmount()
    const current=mount(RankedFeed,{props:{serverApi:api}});await flushPromises();expect(current.text()).toContain('Mark read');resolveMutation();await flushPromises();expect(current.text()).toContain('Mark unread');expect(api.feed).toHaveBeenCalledTimes(3);current.unmount()
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

  it('labels empty permitted content without claiming metadata-only permission', async () => {
    const detail: ArticleDetail={article:article('article-a',.9,'full_content_allowed'),fullContent:''};const wrapper=mount(ArticleReader,{props:{articleId:'article-a',serverApi:fakeApi(detail)}});await flushPromises();expect(wrapper.text()).toContain('No article body was provided');expect(wrapper.text()).not.toContain('permits metadata only');wrapper.unmount()
  })

  it('ignores an obsolete article request after the route changes', async () => {
    const api=fakeApi();let resolveA!:(value:ArticleDetail)=>void,resolveB!:(value:ArticleDetail)=>void;const signals:AbortSignal[]=[]
    vi.mocked(api.article).mockImplementation(async(id,signal)=>{signals.push(signal!);return new Promise(resolve=>{if(id==='article-a')resolveA=resolve;else resolveB=resolve})})
    const wrapper=mount(ArticleReader,{props:{articleId:'article-a',serverApi:api}});await flushPromises();await wrapper.setProps({articleId:'article-b'});await flushPromises();resolveB({article:article('article-b',.8),fullContent:null});await flushPromises();resolveA({article:article('article-a',.9),fullContent:null});await flushPromises();expect(wrapper.text()).toContain('Second story');expect(wrapper.text()).not.toContain('Top ranked story');expect(signals[0].aborted).toBe(true);wrapper.unmount()
  })

  it('reloads the reader ranking explanation after a library mutation', async () => {
    const api=fakeApi(), changed={...article('article-a',.6),library:{...library,readAt:'2026-08-14T11:00:00Z'},ranking:{...article('article-a',.6).ranking,contributions:[]}}
    vi.mocked(api.article).mockResolvedValueOnce({article:article('article-a',.9),fullContent:null}).mockResolvedValueOnce({article:changed,fullContent:null})
    const wrapper=mount(ArticleReader,{props:{articleId:'article-a',serverApi:api}});await flushPromises();expect(wrapper.text()).toContain('Matches an interest');await wrapper.findAll('button').find(item=>item.text()==='Mark read')!.trigger('click');await flushPromises();expect(wrapper.text()).toContain('Mark unread');expect(wrapper.text()).toContain('No ranking explanation');expect(api.article).toHaveBeenCalledTimes(2);wrapper.unmount()
  })

  it('does not apply an article mutation after navigating to another article', async () => {
    const api=fakeApi();let resolveMutation!:(value:LibraryState)=>void
    vi.mocked(api.article).mockImplementation(async id=>({article:article(id,id==='article-a'?.9:.8),fullContent:null}))
    vi.mocked(api.updateLibrary).mockImplementation(async()=>new Promise(resolve=>{resolveMutation=resolve}))
    const wrapper=mount(ArticleReader,{props:{articleId:'article-a',serverApi:api}});await flushPromises();await wrapper.findAll('button').find(item=>item.text()==='Mark read')!.trigger('click');await wrapper.setProps({articleId:'article-b'});await flushPromises()
    expect(wrapper.text()).toContain('Second story');expect(wrapper.findAll('button').find(item=>item.text()==='Mark read')!.attributes('disabled')).toBeUndefined()
    resolveMutation({...library,articleId:'article-a',readAt:'2026-08-14T11:00:00Z'});await flushPromises();expect(wrapper.text()).toContain('Second story');expect(wrapper.text()).toContain('Mark read');expect(wrapper.text()).not.toContain('Article state updated.');expect(api.article).toHaveBeenCalledTimes(2);wrapper.unmount()
  })

  it('does not reload article detail when a mutation settles after unmount', async () => {
    const api=fakeApi();let resolveMutation!:(value:LibraryState)=>void
    vi.mocked(api.updateLibrary).mockImplementation(async()=>new Promise(resolve=>{resolveMutation=resolve}))
    const wrapper=mount(ArticleReader,{props:{articleId:'article-a',serverApi:api}});await flushPromises();await wrapper.findAll('button').find(item=>item.text()==='Mark read')!.trigger('click');wrapper.unmount();resolveMutation({...library,readAt:'2026-08-14T11:00:00Z'});await flushPromises();expect(api.article).toHaveBeenCalledOnce()
  })

  it('revalidates the current article when an older route mutation settles', async () => {
    const api=fakeApi();let committed=false,resolveMutation!:()=>void
    vi.mocked(api.article).mockImplementation(async id=>({article:{...article(id,id==='article-a'?.9:.8),library:{...library,articleId:id,readAt:id==='article-a'&&committed?'2026-08-14T11:00:00Z':null}},fullContent:null}))
    vi.mocked(api.updateLibrary).mockImplementation(async()=>new Promise(resolve=>{resolveMutation=()=>{committed=true;resolve({...library,readAt:'2026-08-14T11:00:00Z'})}}))
    const wrapper=mount(ArticleReader,{props:{articleId:'article-a',serverApi:api}});await flushPromises();await wrapper.findAll('button').find(item=>item.text()==='Mark read')!.trigger('click');await wrapper.setProps({articleId:'article-b'});await flushPromises();await wrapper.setProps({articleId:'article-a'});await flushPromises();expect(wrapper.text()).toContain('Mark read');resolveMutation();await flushPromises();expect(wrapper.text()).toContain('Mark unread');expect(api.article).toHaveBeenCalledTimes(4);wrapper.unmount()
  })
})
