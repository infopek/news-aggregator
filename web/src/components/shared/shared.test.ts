// @vitest-environment happy-dom
import axe from 'axe-core'
import { mount } from '@vue/test-utils'
import { afterEach, describe, expect, it, vi } from 'vitest'
import { nextTick } from 'vue'
import ArticleSummaryCard from './ArticleSummaryCard.vue'
import AsyncState from './AsyncState.vue'
import ConfirmAction from './ConfirmAction.vue'
import CredentialInput from './CredentialInput.vue'
import DemographicSignalField from './DemographicSignalField.vue'
import RankingExplanation from './RankingExplanation.vue'
import RefreshStatus from './RefreshStatus.vue'
import { articleSummary, contribution, partialRefresh } from '../../testing/fixtures'

afterEach(() => document.body.replaceChildren())
async function expectAccessible(element: Element) { expect((await axe.run(element)).violations).toEqual([]) }

describe('shared accessible primitives', () => {
  it('explains optional local low-weight independently disableable demographics', async () => {
    const wrapper = mount(DemographicSignalField, { attachTo: document.body, props: { id: 'age', label: 'Age', modelValue: '42', enabled: true } })
    expect(wrapper.text()).toContain('Optional and stored only on this device')
    expect(wrapper.text()).toContain('low influence')
    await wrapper.get('input[type=checkbox]').setValue(false)
    expect(wrapper.emitted('update:enabled')).toEqual([[false]])
    await expectAccessible(wrapper.element)
  })

  it('clears a pasted credential from memory and DOM after submit and cancel', async () => {
    const log = vi.spyOn(console, 'log')
    const wrapper = mount(CredentialInput, { attachTo: document.body, props: { id: 'secret' } })
    const input = wrapper.get('input')
    await input.setValue('SENTINEL_DO_NOT_RETAIN_8c3f')
    await wrapper.get('form').trigger('submit')
    expect(wrapper.emitted('submit')?.[0]).toEqual(['SENTINEL_DO_NOT_RETAIN_8c3f'])
    expect((input.element as HTMLInputElement).value).toBe('')
    expect(wrapper.html()).not.toContain('SENTINEL_DO_NOT_RETAIN_8c3f')
    expect(localStorage.length).toBe(0); expect(sessionStorage.length).toBe(0); expect(log).not.toHaveBeenCalled()
    await input.setValue('SENTINEL_CANCEL_9e1a'); await wrapper.get('form').trigger('reset')
    expect((input.element as HTMLInputElement).value).toBe(''); expect(wrapper.html()).not.toContain('SENTINEL_CANCEL_9e1a')
    expect(document.activeElement).toBe(input.element)
    log.mockRestore(); await expectAccessible(wrapper.element)
  })

  it('renders malicious article content only as text and preserves canonical links', async () => {
    const hostile = '<img src=x onerror="globalThis.pwned=true">مرحبا世界'.repeat(20)
    const wrapper = mount(ArticleSummaryCard, { attachTo: document.body, props: { article: articleSummary({ title: hostile, excerpt: '<script>alert(1)</script>', language: 'ar' }), sourceName: 'ناشر طويل جدًا' } })
    expect(wrapper.find('img').exists()).toBe(false); expect(wrapper.find('script').exists()).toBe(false)
    expect(wrapper.findAll('a')).toHaveLength(2)
    for (const link of wrapper.findAll('a')) expect(link.attributes('href')).toBe('https://publisher.example/story')
    expect(wrapper.text()).toContain('<script>alert(1)</script>'); expect(wrapper.attributes('dir')).toBe('rtl')
    await expectAccessible(wrapper.element)
  })

  it.each(['javascript:alert(1)', 'data:text/html,pwned', '//publisher.example/story', 'not a URL'])('fails closed for unsafe publisher URL %s', async (canonicalUrl) => {
    const wrapper = mount(ArticleSummaryCard, { attachTo: document.body, props: { article: articleSummary({ canonicalUrl }) } })
    expect(wrapper.find('a').exists()).toBe(false)
    expect(wrapper.text()).toContain('Publisher link unavailable')
    expect(wrapper.html()).not.toContain('href=')
    await expectAccessible(wrapper.element)
  })

  it('reports API contributions exactly and safely labels unknown/no reasons', async () => {
    const unknown = contribution({ signal: 'recency', weightedScore: -0.125, reasonCode: '<unknown>' })
    const wrapper = mount(RankingExplanation, { attachTo: document.body, props: { contributions: [unknown] } })
    expect(wrapper.text()).toContain('Another ranking signal contributed'); expect(wrapper.text()).toContain('-0.125')
    expect(wrapper.text()).not.toContain('<unknown>'); expect(wrapper.text()).toContain('not guarantees')
    await wrapper.setProps({ contributions: [] }); expect(wrapper.text()).toContain('No ranking explanation was provided')
    await expectAccessible(wrapper.element)
  })

  it.each([
    ['explicit_interest_match', 'Matches an explicit interest'],
    ['explicit_location_match', 'Matches your optional location'],
    ['explicit_age_adjustment', 'Matches your optional age'],
    ['explicit_gender_adjustment', 'Matches your optional gender'],
    ['local_text_match', 'Local text is similar'],
  ])('renders production reason code %s', (reasonCode, label) => {
    const wrapper = mount(RankingExplanation, { props: { contributions: [contribution({ reasonCode })] } })
    expect(wrapper.text()).toContain(label)
    expect(wrapper.text()).not.toContain('Another ranking signal contributed')
  })

  it('announces loading, errors, success and mixed partial refresh outcomes', async () => {
    const wrapper = mount(RefreshStatus, { attachTo: document.body, props: { loading: true } })
    expect(wrapper.get('[role=status]').text()).toContain('Refreshing')
    await wrapper.setProps({ loading: false, refresh: partialRefresh(), sourceNames: { 'source-good': 'City News', 'source-bad': 'Official Wire' } })
    expect(wrapper.text()).toContain('some source failures'); expect(wrapper.text()).toContain('City News: Succeeded — 2 fetched, 2 inserted, 0 updated, 0 skipped, 0 failed'); expect(wrapper.text()).toContain('Official Wire: Failed'); expect(wrapper.text()).toContain('Timed out')
    const outcome = partialRefresh().outcomes[0]
    await wrapper.setProps({ refresh: { ...partialRefresh(), outcomes: [{ ...outcome, fetched: 0, inserted: 0 }] } })
    expect(wrapper.text()).toContain('Unchanged — 0 fetched')
    await wrapper.setProps({ refresh: { ...partialRefresh(), outcomes: [{ ...outcome, failed: 1, errorCode: 'rate_limited', errorSummary: 'Retry later.' }] } })
    expect(wrapper.text()).toContain('Rate limited')
    await wrapper.setProps({ refresh: { ...partialRefresh(), outcomes: [{ ...outcome, failed: 1, errorCode: 'cancelled', errorSummary: 'Refresh was cancelled.' }] } })
    expect(wrapper.text()).toContain('Cancelled —'); expect(wrapper.text()).not.toContain('Failed —')
    await wrapper.setProps({ sourceNames: {}, refresh: { ...partialRefresh(), outcomes: [{ ...outcome, sourceId: '9f36c8a18b4e72d940c3ab11' }] } })
    expect(wrapper.text()).toContain('Deleted or unavailable source (9f36c8a18b4e72d940c3ab11)')
    await wrapper.setProps({ refresh: undefined, error: 'Offline' }); expect(wrapper.get('[role=alert]').text()).toContain('Offline')
    await expectAccessible(wrapper.element)
  })

  it.each(['loading', 'empty', 'error', 'success'] as const)('exposes the %s state', async (state) => {
    const wrapper = mount(AsyncState, { attachTo: document.body, props: { state, error: 'Unavailable' }, slots: { default: 'Loaded' } })
    expect(wrapper.text()).toBe(state === 'loading' ? 'Loading…' : state === 'empty' ? 'Nothing to show yet.' : state === 'error' ? 'Unavailable' : 'Loaded')
    await expectAccessible(wrapper.element)
  })

  it('moves focus into confirmation and restores it on escape', async () => {
    const wrapper = mount(ConfirmAction, { attachTo: document.body, props: { label: 'Remove source' } })
    await wrapper.get('button').trigger('click'); expect((document.activeElement as HTMLElement).textContent).toBe('Confirm')
    await wrapper.get('[role=alertdialog]').trigger('keydown', { key: 'Escape' }); await nextTick()
    expect((document.activeElement as HTMLElement).textContent).toBe('Remove source')
    await expectAccessible(wrapper.element)
  })

  it('traps forward and reverse tab focus and gives repeated instances unique labels', async () => {
    const host = mount({ components: { ConfirmAction, RankingExplanation, RefreshStatus }, template: `<div><ConfirmAction label="First"/><ConfirmAction label="Second"/><RankingExplanation :contributions="[]"/><RankingExplanation :contributions="[]"/><RefreshStatus/><RefreshStatus/></div>` }, { attachTo: document.body })
    const triggers = host.findAll('.confirm > button')
    await triggers[0].trigger('click')
    const dialog = host.get('[role=alertdialog]'); const buttons = dialog.findAll('button')
    buttons.at(-1)?.element.focus(); await dialog.trigger('keydown', { key: 'Tab' })
    expect(document.activeElement).toBe(buttons[0].element)
    buttons[0].element.focus(); await dialog.trigger('keydown', { key: 'Tab', shiftKey: true })
    expect(document.activeElement).toBe(buttons.at(-1)?.element)
    const labelled = host.findAll('[aria-labelledby]')
    const ids = labelled.map(node => node.attributes('aria-labelledby')).filter((id): id is string => Boolean(id))
    expect(new Set(ids).size).toBe(ids.length)
    for (const id of ids) expect(document.getElementById(id)).not.toBeNull()
    await buttons.at(-1)?.trigger('click'); expect(document.activeElement).toBe(triggers[0].element)
    await expectAccessible(host.element)
  })
})
