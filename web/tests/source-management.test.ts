// @vitest-environment happy-dom
import axe from 'axe-core'
import { flushPromises, mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'
import type { RefreshRun, Source, SourceWrite } from '../src/api/generated/models'
import type { ServerApi } from '../src/api/server-api'
import SourceManagement from '../src/features/sources/SourceManagement.vue'
import RefreshControl from '../src/features/refresh/RefreshControl.vue'

const policy = { status: 'not_applicable' as const, termsUrl: null, robotsUrl: null, reviewedAt: null, reviewNotes: null }
const feed: Source = { id: 'feed-1', name: 'Local feed', url: 'https://example.com/feed', kind: 'feed', enabled: true, contentPermission: 'metadata_only', adapterConfig: { format: 'rss' }, scraperPolicy: policy, credentialConfigured: false, lastSuccessAt: null, lastError: null, retryAfter: null }
const apiSource: Source = { ...feed, id: 'api-1', name: 'Official API', kind: 'api', adapterConfig: { provider: 'fictional', pageSize: 50 }, credentialConfigured: true }
const scraper: Source = { ...feed, id: 'scraper-1', name: 'Approved pages', kind: 'scraper', enabled: false, adapterConfig: { articleSelector: 'article', titleSelector: 'h1' }, scraperPolicy: { status: 'pending', termsUrl: 'https://example.com/terms', robotsUrl: 'https://example.com/robots.txt', reviewedAt: null, reviewNotes: null } }
const starter: Source = { ...feed, id: 'starter-2', name: 'Starter science', url: 'https://example.com/science.xml' }

function fakeApi() {
  let items = [structuredClone(feed), structuredClone(apiSource), structuredClone(scraper)]
  const terminal: RefreshRun = { id: 'refresh-1', status: 'partial_success', startedAt: '2026-08-13T10:00:00Z', finishedAt: '2026-08-13T10:00:01Z', outcomes: [{ sourceId: 'feed-1', fetched: 3, inserted: 2, updated: 0, skipped: 0, failed: 1, errorCode: 'rate_limited', errorSummary: 'Retry later.' }] }
  return {
    sources: vi.fn(async () => ({ items })), starterSources: vi.fn(async () => ({ items: [starter] })),
    createSource: vi.fn(async (body: SourceWrite) => { const created = { ...body, id: `created-${items.length}`, credentialConfigured: false, lastSuccessAt: null, lastError: null, retryAfter: null } as Source; items = [...items, created]; return created }),
    updateSource: vi.fn(async (id: string, body: SourceWrite) => ({ ...items.find((item) => item.id === id)!, ...body } as Source)),
    deleteSource: vi.fn(async (id: string) => { items = items.filter((item) => item.id !== id) }),
    writeCredential: vi.fn(async () => ({ configured: true })), deleteCredential: vi.fn(async () => ({ configured: false })),
    startRefresh: vi.fn(async () => ({ ...terminal, status: 'running', finishedAt: null } as RefreshRun)), refresh: vi.fn(async () => terminal)
  } as unknown as ServerApi
}

async function ready(api = fakeApi()) {
  localStorage.clear()
  const wrapper = mount(SourceManagement, { props: { serverApi: api }, attachTo: document.body })
  await flushPromises()
  return { wrapper, api }
}

describe('source management and refresh', () => {
  it('shows all source kinds, policy, permission, and credential state accessibly', async () => {
    const { wrapper } = await ready()
    expect(wrapper.text()).toContain('Local feedFEED')
    expect(wrapper.text()).toContain('Official API')
    expect(wrapper.text()).toContain('Policy: pending')
    expect(wrapper.text()).toContain('Publisher link only')
    expect(wrapper.text()).toContain('Configured')
    expect((await axe.run(wrapper.element)).violations).toEqual([])
    wrapper.unmount()
  })

  it('validates each custom source family and refuses an unapproved enabled scraper', async () => {
    const { wrapper, api } = await ready()
    await wrapper.findAll('button').find((item) => item.text() === 'Add custom source')!.trigger('click')
    const form = wrapper.get('.source-form')
    await form.get('input[type=url]').setValue('https://example.com/custom')
    await form.get('select').setValue('api')
    await form.trigger('submit'); expect(wrapper.text()).toContain('Enter a source name')
    await form.get('input:not([type=url]):not([type=checkbox])').setValue('Custom API')
    await form.trigger('submit'); expect(wrapper.text()).toContain('provider identifier')
    await form.get('select').setValue('scraper'); await flushPromises()
    await form.trigger('submit')
    expect(wrapper.text()).toContain('approved policy review')
    expect(api.createSource).not.toHaveBeenCalled()
    wrapper.unmount()
  })

  it('clears submitted credentials and never renders the secret', async () => {
    const { wrapper, api } = await ready()
    const apiCard = wrapper.findAll('.source-card').find((item) => item.text().includes('Official API'))!
    await apiCard.findAll('button').find((item) => item.text().includes('Replace credential'))!.trigger('click')
    const input = apiCard.get('input[type=password]')
    await input.setValue('SENTINEL-PRIVATE-CREDENTIAL')
    await apiCard.get('form.credential').trigger('submit'); await flushPromises()
    expect(api.writeCredential).toHaveBeenCalledWith('api-1', { secret: 'SENTINEL-PRIVATE-CREDENTIAL' })
    expect(wrapper.html()).not.toContain('SENTINEL-PRIVATE-CREDENTIAL')
    expect(wrapper.text()).toContain('saved and cleared')
    wrapper.unmount()
  })

  it('starts one refresh, polls to terminal mixed outcome, and presents sanitized counts', async () => {
    const { wrapper, api } = await ready()
    await wrapper.findAll('button').find((item) => item.text().includes('Refresh all'))!.trigger('click')
    await flushPromises()
    expect(api.startRefresh).toHaveBeenCalledOnce()
    expect(api.refresh).toHaveBeenCalledWith('refresh-1', expect.any(AbortSignal))
    expect(wrapper.text()).toContain('completed with some source failures')
    expect(wrapper.text()).toContain('Rate limited — 3 fetched, 2 inserted, 0 updated, 0 skipped, 1 failed — Retry later.')
    expect(localStorage.getItem('news-aggregator:last-refresh-id')).toBe('refresh-1')
    wrapper.unmount()
  })

  it('recovers a persisted refresh after route disposal or reload', async () => {
    const api = fakeApi()
    localStorage.setItem('news-aggregator:last-refresh-id', 'refresh-1')
    const wrapper = mount(SourceManagement, { props: { serverApi: api } })
    await flushPromises()
    expect(api.refresh).toHaveBeenCalledWith('refresh-1', expect.any(AbortSignal))
    expect(wrapper.text()).toContain('Refresh completed with some source failures.')
    wrapper.unmount()
  })

  it('retains the recovery ID after transient status failure and succeeds later', async () => {
    const api = fakeApi()
    vi.mocked(api.refresh).mockRejectedValueOnce(new TypeError('temporary network failure'))
    localStorage.setItem('news-aggregator:last-refresh-id', 'refresh-1')
    const failed = mount(RefreshControl, { props: { server: api } })
    await flushPromises()
    expect(failed.text()).toContain('temporarily unavailable')
    expect(localStorage.getItem('news-aggregator:last-refresh-id')).toBe('refresh-1')
    failed.unmount()
    const recovered = mount(RefreshControl, { props: { server: api } })
    await flushPromises()
    expect(recovered.text()).toContain('Refresh completed with some source failures.')
    recovered.unmount()
  })

  it('retains the recovery ID when route disposal aborts an in-flight status request', async () => {
    const api = fakeApi()
    vi.mocked(api.refresh).mockImplementation(async (_id, signal) => new Promise((_, reject) => signal?.addEventListener('abort', () => reject(new DOMException('Aborted', 'AbortError')), { once: true })))
    localStorage.setItem('news-aggregator:last-refresh-id', 'refresh-1')
    const wrapper = mount(RefreshControl, { props: { server: api } })
    await flushPromises()
    wrapper.unmount(); await flushPromises()
    expect(localStorage.getItem('news-aggregator:last-refresh-id')).toBe('refresh-1')
  })

  it('marks a newly created starter configured by stable URL identity', async () => {
    const { wrapper, api } = await ready()
    const button = wrapper.findAll('button').find((item) => item.text() === 'Add starter')!
    await button.trigger('click'); await flushPromises()
    expect(api.createSource).toHaveBeenCalledOnce()
    expect(wrapper.findAll('button').find((item) => item.text() === 'Configured')?.attributes('disabled')).toBeDefined()
    wrapper.unmount()
  })
})
