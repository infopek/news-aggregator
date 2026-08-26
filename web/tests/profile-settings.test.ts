// @vitest-environment happy-dom
import axe from 'axe-core'
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'
import { ApiRequestError } from '../src/api/client'
import type { Profile, RankingConfiguration, Source } from '../src/api/generated/models'
import type { ServerApi } from '../src/api/server-api'
import ProfileSettings from '../src/features/profile/ProfileSettings.vue'
import InterestChipInput from '../src/features/profile/InterestChipInput.vue'
import { applyRankingPreset, detectRankingPreset, profileToForm, profileWrite, rankingToForm, rankingWrite } from '../src/features/profile/profile-form'
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

function fakeApi(initial: Profile = emptyProfile, failure?: Error, rankingValue: RankingConfiguration = ranking) {
  let current = structuredClone(initial)
  const api = {
    profile: vi.fn(async () => current), ranking: vi.fn(async () => rankingValue), starterSources: vi.fn(async () => ({ items: [starter] })),
    updateProfile: vi.fn(async (body) => { if (failure) throw failure; current = { ...body, id: 'local-profile', updatedAt: '2026-08-03T00:00:00Z' }; return current }),
    updateRanking: vi.fn(async (body) => ({ ...body, perDemographicCap: .1, totalDemographicCap: .2, normalizationVersion: 'v1' }))
  }
  return api as unknown as ServerApi
}
async function ready(component: typeof ProfileSettings | typeof FirstRunSetup, api: ServerApi) {
  const wrapper = mount(component, { props: { serverApi: api }, attachTo: document.body })
  await flushPromises(); return wrapper
}
async function addTopic(wrapper: VueWrapper, topic: string) {
  await wrapper.get('#interest-input').setValue(topic)
  await wrapper.get('#interest-input').trigger('keydown', { key: 'Enter' })
}
async function continueSetup(wrapper: VueWrapper) {
  await wrapper.findAll('button').find((item) => item.text() === 'Continue')!.trigger('click')
  await flushPromises()
}

describe('consumer onboarding and settings', () => {
  it('starts with an approachable interest step and no raw ranking controls', async () => {
    const wrapper = await ready(FirstRunSetup, fakeApi())
    expect(wrapper.get('h1').text()).toBe('Make your feed yours')
    expect(wrapper.get('[aria-current=step]').text()).toContain('Interests')
    expect(wrapper.get('#interest-input').attributes('placeholder')).toContain('Try “AI”')
    expect(wrapper.text()).not.toContain('New interest weight')
    expect(wrapper.find('input[type=number]').exists()).toBe(false)
    expect((await axe.run(wrapper.element)).violations).toEqual([])
    wrapper.unmount()
  })

  it('adds/removes chips, rejects duplicates, and supports Enter', async () => {
    const wrapper = mount(InterestChipInput, { props: { modelValue: ['Science'] } })
    await wrapper.get('#interest-input').setValue('science')
    await wrapper.get('#interest-input').trigger('keydown', { key: 'Enter' })
    expect(wrapper.text()).toContain('already in your interests')
    await wrapper.setProps({ modelValue: ['Science', 'AI'] })
    await wrapper.get('button[aria-label="Remove Science"]').trigger('click')
    expect(wrapper.emitted('update:modelValue')?.at(-1)?.[0]).toEqual(['AI'])
  })

  it('completes four-step setup with Hungary, Budapest, optional demographics, sources, and a semantic preset', async () => {
    const api = fakeApi(); const wrapper = await ready(FirstRunSetup, api)
    await addTopic(wrapper, 'Technology'); await addTopic(wrapper, 'science')
    await continueSetup(wrapper)
    await wrapper.get('#country').setValue('HU'); await wrapper.get('#city').setValue('Budapest')
    const additional = wrapper.findAll('details').find((item) => item.text().includes('Additional personalization'))!
    ;(additional.element as HTMLDetailsElement).open = true
    const ageToggle = additional.findAll('input[type=checkbox]')[0]
    await ageToggle.setValue(true); await wrapper.get('#age').setValue('35'); await ageToggle.setValue(false)
    await continueSetup(wrapper)
    await wrapper.get('.source-choice input').setValue(true)
    await continueSetup(wrapper)
    await wrapper.get('input[value=personalized]').setValue(true)
    expect(wrapper.get('button[type=submit]').text()).toBe('Finish setup')
    await wrapper.get('form').trigger('submit'); await flushPromises()
    expect(api.updateProfile).toHaveBeenCalledWith(expect.objectContaining({
      interests: [{ name: 'Technology', weight: .8 }, { name: 'science', weight: .8 }], preferredSourceIds: ['starter-1'],
      location: { present: true, enabled: false, value: { country: 'HU', region: 'Budapest', city: { present: true, enabled: true, value: 'Budapest' } } },
      age: { present: true, enabled: false, value: 35 }
    }), expect.any(AbortSignal))
    expect(api.updateRanking).toHaveBeenCalledWith(expect.objectContaining({ interest: { enabled: true, weight: .35 }, recency: { enabled: true, weight: .15 } }), expect.any(AbortSignal))
    expect(wrapper.text()).toContain('Setup saved on this computer')
    wrapper.unmount()
  })

  it('maps presets exactly and preserves a custom configuration until explicitly changed', () => {
    const balanced = rankingToForm(ranking)
    expect(detectRankingPreset(balanced)).toBe('balanced')
    const personalized = applyRankingPreset(balanced, 'personalized')
    expect(detectRankingPreset(personalized)).toBe('personalized')
    expect(rankingWrite(personalized)).toMatchObject({ interest: { weight: .35 }, recency: { weight: .15 } })
    const custom = { ...balanced, recency: { ...balanced.recency, weight: '.37' } }
    expect(detectRankingPreset(custom)).toBe('custom')
    expect(rankingWrite(custom).recency.weight).toBe(.37)
  })

  it('preserves distinct saved interest weights and unusual region/country values during unrelated edits', async () => {
    const varied = { ...savedProfile, interests: [{ name: 'technology', weight: .9 }, { name: 'science', weight: .4 }], location: { present: true, enabled: true, value: { country: 'XX', region: 'Historic district', city: { present: true, enabled: true, value: 'Locality' } } } } as Profile
    const api = fakeApi(varied); const wrapper = await ready(ProfileSettings, api)
    expect(wrapper.get('#country').text()).toContain('Other (XX)')
    await wrapper.get('#city').setValue('New locality'); await wrapper.get('form').trigger('submit'); await flushPromises()
    expect(api.updateProfile).toHaveBeenCalledWith(expect.objectContaining({ interests: varied.interests, location: expect.objectContaining({ value: expect.objectContaining({ country: 'XX', region: 'Historic district' }) }) }), expect.any(AbortSignal))
    wrapper.unmount()
  })

  it('keeps optional demographics collapsed and explains publisher-declared matching', async () => {
    const wrapper = await ready(ProfileSettings, fakeApi(savedProfile))
    const disclosure = wrapper.findAll('details').find((item) => item.text().includes('Additional personalization'))!
    expect(disclosure.attributes('open')).toBeUndefined()
    expect(disclosure.text()).toContain('publisher explicitly labels')
    expect(disclosure.text()).toContain('never inferred')
    wrapper.unmount()
  })

  it('clears visible profile values while preserving source choices managed on the Sources screen', async () => {
    window.history.replaceState(null, '', '/settings')
    const api = fakeApi(savedProfile); const wrapper = await ready(ProfileSettings, api)
    await wrapper.get('button[aria-label="Remove technology"]').trigger('click')
    await wrapper.get('#country').setValue(''); await wrapper.get('#region').setValue(''); await wrapper.get('#city').setValue('')
    await wrapper.get('#age').setValue(''); await wrapper.get('#gender').setValue('')
    for (const item of wrapper.findAll('input[type=checkbox]')) await item.setValue(false)
    await wrapper.get('form').trigger('submit'); await flushPromises()
    expect(api.updateProfile).toHaveBeenCalledWith(expect.objectContaining({ interests: [], preferredSourceIds: ['starter-1'], location: { present: false, enabled: false }, age: { present: true, enabled: false, value: 35 }, gender: { present: false, enabled: false } }), expect.any(AbortSignal))
    wrapper.unmount()
  })

  it('shows authoritative field errors without clearing chip edits', async () => {
    const error = new ApiRequestError(400, { code: 'validation_failed', message: 'Check values.', correlationId: 'safe-id', fields: [{ path: 'profile.location.value.country', code: 'INVALID_COUNTRY', message: 'Choose a supported country.' }] })
    const wrapper = await ready(ProfileSettings, fakeApi(emptyProfile, error))
    await addTopic(wrapper, 'keep me'); await wrapper.get('#country').setValue('HU'); await wrapper.get('#city').setValue('Budapest')
    await wrapper.get('form').trigger('submit'); await flushPromises()
    expect(wrapper.get('#country-error').text()).toContain('supported country')
    expect(wrapper.text()).toContain('keep me')
    expect(wrapper.text()).toContain('Settings were not saved')
    wrapper.unmount()
  })

  it('rejects invalid advanced weights before mutation and retains edits', async () => {
    const api = fakeApi(); const wrapper = await ready(ProfileSettings, api)
    await addTopic(wrapper, 'still here')
    const advanced = wrapper.findAll('details').find((item) => item.text().includes('Advanced ranking controls'))!
    ;(advanced.element as HTMLDetailsElement).open = true
    await wrapper.get('#ranking-age').setValue('1.1')
    await wrapper.get('form').trigger('submit'); await flushPromises()
    expect(wrapper.get('#ranking-age-error').text()).toContain('0 to 1')
    expect(wrapper.text()).toContain('still here')
    expect(api.updateProfile).not.toHaveBeenCalled(); expect(api.updateRanking).not.toHaveBeenCalled()
    wrapper.unmount()
  })

  it('cancels outstanding loads and ignores obsolete load completion', async () => {
    const cache = new ServerStateClient(); let profileSignal: AbortSignal | undefined
    const api = fakeApi(savedProfile)
    vi.mocked(api.profile).mockImplementationOnce(async (signal) => { profileSignal = signal; await new Promise<void>((resolve) => signal?.addEventListener('abort', () => resolve(), { once: true })); return emptyProfile })
    const pending = mount(ProfileSettings, { props: { serverApi: api, serverState: cache } }); await flushPromises(); pending.unmount(); expect(profileSignal?.aborted).toBe(true)

    let release!: (profile: Profile) => void
    vi.mocked(api.profile).mockImplementationOnce(async () => new Promise<Profile>((resolve) => { release = resolve })).mockImplementation(async () => savedProfile)
    const wrapper = mount(ProfileSettings, { props: { serverApi: api }, attachTo: document.body }); await flushPromises(); await (wrapper.vm as unknown as { load: () => Promise<void> }).load(); await flushPromises(); release(emptyProfile); await flushPromises()
    expect(wrapper.text()).toContain('technology'); wrapper.unmount()
  })

  it('reports partial ranking save and reloads authoritative settings', async () => {
    const api = fakeApi(); vi.mocked(api.updateRanking).mockRejectedValueOnce(new ApiRequestError(503, { code: 'unavailable', message: 'Ranking unavailable.', correlationId: 'safe-id', fields: [] }))
    const wrapper = await ready(ProfileSettings, api); await addTopic(wrapper, 'saved profile'); await wrapper.get('form').trigger('submit'); await flushPromises()
    expect(wrapper.text()).toContain('Profile saved; ranking settings not saved.')
    await wrapper.get('.error-summary button').trigger('click'); await flushPromises()
    expect(wrapper.find('.error-summary').exists()).toBe(false); expect(wrapper.text()).toContain('saved profile'); wrapper.unmount()
  })

  it('adapts a simple city to the existing region contract without losing saved advanced detail', () => {
    const simple = profileToForm(emptyProfile); simple.country = 'HU'; simple.city = 'Budapest'; simple.locationEnabled = true
    expect(profileWrite(simple).location).toEqual({ present: true, enabled: true, value: { country: 'HU', region: 'Budapest', city: { present: true, enabled: true, value: 'Budapest' } } })
  })
})
