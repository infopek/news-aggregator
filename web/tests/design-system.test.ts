// @vitest-environment happy-dom
import { afterEach, describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import axe from 'axe-core'

import { ActionGroup, DisclosurePanel, EmptyState, StatusBanner, SurfaceCard, TagChip } from '../src/components/shared'

afterEach(() => vi.restoreAllMocks())

describe('shared design-system primitives', () => {
  it('composes accessible surfaces, statuses, actions, disclosures, and empty states', async () => {
    const wrapper = mount({
      components: { ActionGroup, DisclosurePanel, EmptyState, StatusBanner, SurfaceCard, TagChip },
      template: `<main>
        <SurfaceCard title="Personalization" description="Choose what matters to you.">
          <TagChip label="Science" />
          <DisclosurePanel summary="Advanced controls"><p>Relative influence settings.</p></DisclosurePanel>
        </SurfaceCard>
        <StatusBanner title="Refresh complete" tone="success" live><p>12 new stories.</p></StatusBanner>
        <EmptyState title="Nothing saved yet" description="Save stories to read later.">
          <ActionGroup label="Empty library actions"><a class="button-link" href="/">Browse stories</a></ActionGroup>
        </EmptyState>
      </main>`,
    }, { attachTo: document.body })

    expect(wrapper.get('.surface__header').text()).toContain('Choose what matters')
    expect(wrapper.get('details').attributes('open')).toBeUndefined()
    expect(wrapper.get('[role=status]').text()).toContain('12 new stories')
    expect(wrapper.get('[role=group]').attributes('aria-label')).toBe('Empty library actions')
    expect((await axe.run(wrapper.element)).violations).toEqual([])
    wrapper.unmount()
  })

  it('makes removable chips explicit and keyboard-operable native buttons', async () => {
    const wrapper = mount(TagChip, { props: { label: 'Local news', removable: true } })
    const button = wrapper.get('button')
    expect(button.attributes('aria-label')).toBe('Remove Local news')
    await button.trigger('click')
    expect(wrapper.emitted('remove')).toHaveLength(1)
  })

  it('uses alerts only for urgent status and polite status for progress updates', () => {
    const danger = mount(StatusBanner, { props: { title: 'Could not refresh', tone: 'danger' } })
    expect(danger.attributes('role')).toBe('alert')
    const info = mount(StatusBanner, { props: { title: 'Refreshing', live: true } })
    expect(info.attributes('role')).toBe('status')
    expect(info.attributes('aria-live')).toBe('polite')
  })
})
