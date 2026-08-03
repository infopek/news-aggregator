// @vitest-environment happy-dom
import axe from 'axe-core'
import { flushPromises, mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'
import { ApiRequestError } from '../src/api/client'
import type { Profile, RankingConfiguration, Source } from '../src/api/generated/models'
import type { ServerApi } from '../src/api/server-api'
import ProfileSettings from '../src/features/profile/ProfileSettings.vue'
import FirstRunSetup from '../src/features/setup/FirstRunSetup.vue'
import { ServerStateClient } from '../src/state/query-client'

const emptyProfile: Profile = { id: 'local-profile', interests: [], preferredSourceIds: [], location: { present: false, enabled: false }, age: { present: false, enabled: false }, gender: { present: false, enabled: false }, updatedAt: '2026-08-01T00:00:00Z' }
const savedProfile: Profile = { ...emptyProfile, interests: [{ name: 'technology', weight: .8 }], preferredSourceIds: ['starter-1'], location: { present: true, enabled: true, value: { country: 'HU', region: 'Pest', city: { present: true, enabled: true, value: 'Budapest' } } }, age: { present: true, enabled: false, value: 35 }, gender: { present: true, enabled: true, value: 'nonbinary' } }
const ranking: RankingConfiguration = {
  recency: { enabled: true, weight: .25 }, interest: { enabled: true, weight: .25 }, sourcePreference: { enabled: true, weight: .1 }, behavior: { enabled: true, weight: .1 },
  location: { enabled: false, weight: .05 }, age: { enabled: false, weight: .05 }, gender: { enabled: false, weight: .05 }, textSimilarity: { enabled: true, weight: .15 },
  perDemographicCap: .1, totalDemographicCap: .2, normalizationVersion: 'v1'
}
const starter: Source = { id: 'starter-1', name: 'Starter feed', url: 'https://example.com/feed', kind: 'feed', enabled: true, contentPermission: 'metadata_only', adapterConfig: { format: 'rss' }, scraperPolicy: { status: 'not_applicable', termsUrl: null, robotsUrl: null, reviewedAt: null, reviewNotes: null }, credentialConfigured: false, lastSuccessAt: null, lastError: null, retryAfter: null }

function fakeApi(initial: Profile = emptyProfile, failure?: Error) {
  let current = structuredClone(initial)
  const api = {
    profile: vi.fn(async () => current), ranking: vi.fn(async () => ranking), starterSources: vi.fn(async () => ({ items: [starter] })),
    updateProfile: vi.fn(async (body) => { if (failure) throw failure; current = { ...body, id: 'local-profile', updatedAt: '2026-08-03T00:00:00Z' }; return current }),
    updateRanking: vi.fn(async (body) => ({ ...body, perDemographicCap: .1, totalDemographicCap: .2, normalizationVersion: 'v1' }))
  }
  return api as unknown as ServerApi
}
async function ready(component: typeof ProfileSettings | typeof FirstRunSetup, api: ServerApi) {
  const wrapper = mount(component, { props: { serverApi: api }, attachTo: document.body })
  await flushPromises(); return wrapper
}

describe('profile and first-run workflow', () => {
  it('renders a new, optional-empty, keyboard-accessible setup without violations', async () => {
    const wrapper = await ready(FirstRunSetup, fakeApi())
    expect(wrapper.get('button[type=submit]').text()).toBe('Save setup')
    expect(wrapper.text()).toContain('Selecting none is allowed')
    expect(wrapper.text()).toContain('Each demographic contribution is capped at 0.1')
    expect((await axe.run(wrapper.element)).violations).toEqual([])
    const controls = wrapper.findAll('input,button'); controls[0].element.focus()
    await wrapper.get('form').trigger('keydown', { key: 'Tab' }); expect(controls.length).toBeGreaterThan(10)
    wrapper.unmount()
  })

  it('preserves a disabled entered demographic and submits valid explicit state', async () => {
    const api = fakeApi(); const wrapper = await ready(FirstRunSetup, api)
    await wrapper.get('#interests').setValue('technology, science')
    await wrapper.get('#country').setValue('hu'); await wrapper.get('#region').setValue('Pest')
    const ageToggle = wrapper.findAll('input[type=checkbox]').find((item) => item.element.parentElement?.textContent?.includes('Use this signal'))!
    await ageToggle.setValue(true); await wrapper.get('#age').setValue('35')
    await ageToggle.setValue(false)
    await wrapper.get('form').trigger('submit'); await flushPromises()
    expect(api.updateProfile).toHaveBeenCalledWith(expect.objectContaining({
      interests: [{ name: 'technology', weight: .8 }, { name: 'science', weight: .8 }],
      location: expect.objectContaining({ present: true, value: expect.objectContaining({ country: 'HU', region: 'Pest' }) }),
      age: { present: true, enabled: false, value: 35 }
    }), expect.any(AbortSignal))
    expect(wrapper.text()).toContain('Setup saved on this computer')
    wrapper.unmount()
  })

  it('bypasses completed first-run, supports clearing all values, and reloads authoritative state', async () => {
    window.history.replaceState(null, '', '/setup')
    const api = fakeApi(savedProfile); const setup = await ready(FirstRunSetup, api)
    expect(window.location.pathname).toBe('/settings')
    setup.unmount()
    const wrapper = await ready(ProfileSettings, api)
    expect(wrapper.get('#city').element).toHaveProperty('value', 'Budapest')
    await wrapper.get('#interests').setValue(''); await wrapper.get('#country').setValue(''); await wrapper.get('#region').setValue(''); await wrapper.get('#city').setValue('')
    const ageToggle = wrapper.findAll('input[type=checkbox]').find((item) => item.element.parentElement?.textContent?.includes('Use this signal'))!
    await ageToggle.setValue(true); await wrapper.get('#age').setValue(''); await wrapper.get('#gender').setValue('')
    for (const item of wrapper.findAll('input[type=checkbox]')) await item.setValue(false)
    await wrapper.get('form').trigger('submit'); await flushPromises()
    expect(api.updateProfile).toHaveBeenCalledWith(expect.objectContaining({ interests: [], preferredSourceIds: [], location: { present: false, enabled: false }, age: { present: false, enabled: false }, gender: { present: false, enabled: false } }), expect.any(AbortSignal))
    wrapper.unmount()
    window.history.replaceState(null, '', '/setup')
    const reloaded = await ready(FirstRunSetup, api); expect(reloaded.get('button[type=submit]').text()).toBe('Save setup'); reloaded.unmount()
  })

  it('preserves distinct authoritative interest weights when editing other settings', async () => {
    const varied = { ...savedProfile, interests: [{ name: 'technology', weight: .9 }, { name: 'science', weight: .4 }] }
    const api = fakeApi(varied); const wrapper = await ready(ProfileSettings, api)
    await wrapper.get('#city').setValue('Buda'); await wrapper.get('form').trigger('submit'); await flushPromises()
    expect(api.updateProfile).toHaveBeenCalledWith(expect.objectContaining({ interests: varied.interests }), expect.any(AbortSignal))
    wrapper.unmount()
  })

  it('shows contract fields while retaining other unsaved entries', async () => {
    const error = new ApiRequestError(400, { code: 'validation_failed', message: 'Check values.', correlationId: 'safe-id', fields: [{ path: 'location.value.country', code: 'INVALID_COUNTRY', message: 'Use a two-letter country code.' }] })
    const wrapper = await ready(ProfileSettings, fakeApi(emptyProfile, error))
    await wrapper.get('#interests').setValue('keep me'); await wrapper.get('#country').setValue('bad')
    await wrapper.get('form').trigger('submit'); await flushPromises()
    expect(wrapper.get('#country-error').text()).toContain('two-letter')
    expect(wrapper.get('#interests').element).toHaveProperty('value', 'keep me')
    expect(wrapper.text()).toContain('Settings were not saved')
    wrapper.unmount()
  })

  it('rejects invalid bounded weights before mutation and keeps partial form state', async () => {
    const api = fakeApi(); const wrapper = await ready(ProfileSettings, api)
    await wrapper.get('#interests').setValue('still here')
    await wrapper.get('#ranking-age').setValue('1.1')
    await wrapper.get('form').trigger('submit'); await flushPromises()
    expect(wrapper.get('#ranking-age-error').text()).toContain('0 to 1')
    expect(wrapper.get('#interests').element).toHaveProperty('value', 'still here')
    expect(document.activeElement).toBe(wrapper.get('.error-summary').element)
    expect(api.updateProfile).not.toHaveBeenCalled(); expect(api.updateRanking).not.toHaveBeenCalled()
    wrapper.unmount()
  })

  it.each([
    ['conflict', 409, 'conflict', 'Saved settings changed elsewhere.'],
    ['unavailable', 503, 'unavailable', 'Service down.']
  ] as const)('reports %s save failures without clearing edits', async (_, status, code, message) => {
    const error = new ApiRequestError(status, { code, message, correlationId: 'safe-id', fields: [] })
    const wrapper = await ready(ProfileSettings, fakeApi(emptyProfile, error)); await wrapper.get('#interests').setValue('unsaved')
    await wrapper.get('form').trigger('submit'); await flushPromises()
    expect(wrapper.get('#interests').element).toHaveProperty('value', 'unsaved')
    expect(wrapper.text()).toContain(code === 'unavailable' ? 'temporarily unavailable' : message)
    wrapper.unmount()
  })

  it('cancels outstanding state-layer loads on unmount', async () => {
    const cache = new ServerStateClient()
    let profileSignal: AbortSignal | undefined
    const api = fakeApi()
    vi.mocked(api.profile).mockImplementation(async (signal) => {
      profileSignal = signal
      await new Promise<void>((resolve) => signal?.addEventListener('abort', () => resolve(), { once: true }))
      return emptyProfile
    })
    const wrapper = mount(ProfileSettings, { props: { serverApi: api, serverState: cache } })
    await flushPromises()
    wrapper.unmount()
    expect(profileSignal?.aborted).toBe(true)
  })

  it('ignores an older load that resolves after an authoritative retry', async () => {
    let releaseFirst!: (profile: Profile) => void
    const first = new Promise<Profile>((resolve) => { releaseFirst = resolve })
    const api = fakeApi(savedProfile)
    vi.mocked(api.profile).mockImplementationOnce(async () => first).mockImplementation(async () => savedProfile)
    const wrapper = mount(ProfileSettings, { props: { serverApi: api }, attachTo: document.body })
    await flushPromises()
    await (wrapper.vm as unknown as { load: () => Promise<void> }).load()
    await flushPromises()
    releaseFirst(emptyProfile)
    await flushPromises()
    expect(wrapper.get('#interests').element).toHaveProperty('value', 'technology')
    wrapper.unmount()
  })

  it('reconciles a successful profile when ranking save fails and offers authoritative reload', async () => {
    const api = fakeApi()
    vi.mocked(api.updateRanking).mockRejectedValueOnce(new ApiRequestError(503, { code: 'unavailable', message: 'Ranking unavailable.', correlationId: 'safe-id', fields: [] }))
    const wrapper = await ready(ProfileSettings, api)
    await wrapper.get('#interests').setValue('saved profile')
    await wrapper.get('form').trigger('submit'); await flushPromises()
    expect(wrapper.text()).toContain('Profile saved; ranking settings not saved.')
    expect(wrapper.get('#interests').element).toHaveProperty('value', 'saved profile')
    await wrapper.get('.error-summary button').trigger('click'); await flushPromises()
    expect(wrapper.find('.error-summary').exists()).toBe(false)
    expect(wrapper.get('#interests').element).toHaveProperty('value', 'saved profile')
    wrapper.unmount()
  })
})
