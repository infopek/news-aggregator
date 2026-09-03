// @vitest-environment happy-dom
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import axe from 'axe-core'

import ApplicationShell from '../src/app/ApplicationShell.vue'
import { setShellStatus } from '../src/app/shell-status'

describe('application shell', () => {
  beforeEach(() => {
    window.history.replaceState(null, '', '/')
    setShellStatus({ kind: 'ready' })
    vi.stubGlobal('fetch', vi.fn(async () => new Response(JSON.stringify({
      id: 'local-profile', interests: [{ name: 'news', weight: 1 }], preferredSourceIds: [],
      location: { present: true, enabled: true, value: { country: 'HU', region: 'Pest', city: { present: false, enabled: false } } }, age: { present: false, enabled: false }, gender: { present: false, enabled: false }, updatedAt: '2026-08-03T00:00:00Z'
    }), { status: 200, headers: { 'content-type': 'application/json' } })))
  })
  afterEach(() => { vi.restoreAllMocks(); vi.unstubAllGlobals() })

  it('provides named navigation, landmarks, and no automated accessibility violations', async () => {
    const wrapper = mount(ApplicationShell, { attachTo: document.body })
    expect(wrapper.find('header').exists()).toBe(true)
    expect(wrapper.find('nav[aria-label="Primary navigation"]').exists()).toBe(true)
    expect(wrapper.find('main').exists()).toBe(true)
    expect(wrapper.find('footer').exists()).toBe(true)
    expect(wrapper.findAll('nav a').map((link) => link.text())).toEqual(['Ranked feed', 'Library', 'Sources', 'Settings'])
    const result = await axe.run(wrapper.element)
    expect(result.violations).toEqual([])
    wrapper.unmount()
  })

  it('navigates by keyboard-operable links and handles history changes', async () => {
    const wrapper = mount(ApplicationShell, { attachTo: document.body })
    const library = wrapper.find('a[href="/library"]')
    expect(library.attributes('href')).toBe('/library')
    await library.trigger('click', { button: 0 })
    await flushPromises()
    expect(window.location.pathname).toBe('/library')
    expect(wrapper.get('h1').text()).toBe('Library')
    expect(document.activeElement?.id).toBe('library-title')

    window.history.pushState(null, '', '/sources')
    window.dispatchEvent(new PopStateEvent('popstate'))
    await flushPromises()
    expect(wrapper.get('h1').text()).toBe('Sources and refresh')
    wrapper.unmount()
  })

  it('renders direct article links and unknown-route recovery', async () => {
    window.history.replaceState(null, '', '/articles/article%2042')
    const article = mount(ApplicationShell)
    await flushPromises()
    expect(article.get('[role=status]').text()).toBe('Loading article…')
    article.unmount()

    window.history.replaceState(null, '', '/does-not-exist')
    const missing = mount(ApplicationShell)
    await flushPromises()
    expect(missing.get('h1').text()).toBe('Page not found')
    expect(missing.get('.button-link[href="/"]').text()).toBe('Go to ranked feed')
    missing.unmount()
  })

  it('shows finite loading, recoverable error, and API-down relaunch states', async () => {
    const retry = vi.fn()
    setShellStatus({ kind: 'loading', message: 'Starting the local service…' })
    const wrapper = mount(ApplicationShell)
    expect(wrapper.get('[aria-busy="true"]').text()).toContain('Starting the local service')

    setShellStatus({ kind: 'error', message: 'The request was interrupted.', retry })
    await flushPromises()
    await wrapper.get('button').trigger('click')
    expect(retry).toHaveBeenCalledOnce()

    setShellStatus({ kind: 'api-down', retry })
    await flushPromises()
    expect(wrapper.get('[role="alert"]').text()).toContain('relaunch News Aggregator')
    expect(wrapper.get('[role="alert"] button').text()).toBe('Retry connection')
    wrapper.unmount()
  })

  it('exposes unsupported, install-available, and update-waiting states', async () => {
    const wrapper = mount(ApplicationShell)
    expect(wrapper.text()).toContain('Installation is not available')

    const prompt = vi.fn(async () => undefined)
    const installEvent = new Event('beforeinstallprompt') as Event & { prompt: typeof prompt; userChoice: Promise<{ outcome: 'accepted' }> }
    installEvent.prompt = prompt
    installEvent.userChoice = Promise.resolve({ outcome: 'accepted' })
    window.dispatchEvent(installEvent)
    await flushPromises()
    await wrapper.get('footer button').trigger('click')
    expect(prompt).toHaveBeenCalledOnce()
    expect(wrapper.text()).toContain('Installed on this computer')

    const apply = vi.fn(async () => undefined)
    window.dispatchEvent(new CustomEvent('app:update-waiting', { detail: { apply } }))
    await flushPromises()
    await wrapper.get('.update-banner button').trigger('click')
    expect(apply).toHaveBeenCalledOnce()
    wrapper.unmount()
  })

  it('removes a dismissed install action until the browser offers a new prompt', async () => {
    const wrapper = mount(ApplicationShell)
    const firstPrompt = vi.fn(async () => undefined)
    const dismissed = new Event('beforeinstallprompt') as Event & { prompt: typeof firstPrompt; userChoice: Promise<{ outcome: 'dismissed' }> }
    dismissed.prompt = firstPrompt
    dismissed.userChoice = Promise.resolve({ outcome: 'dismissed' })
    window.dispatchEvent(dismissed)
    await flushPromises()
    await wrapper.get('footer button').trigger('click')
    await flushPromises()
    expect(wrapper.find('footer button').exists()).toBe(false)
    expect(wrapper.get('footer').text()).toContain('Installation was dismissed')

    const secondPrompt = vi.fn(async () => undefined)
    const rearmed = new Event('beforeinstallprompt') as Event & { prompt: typeof secondPrompt; userChoice: Promise<{ outcome: 'accepted' }> }
    rearmed.prompt = secondPrompt
    rearmed.userChoice = Promise.resolve({ outcome: 'accepted' })
    window.dispatchEvent(rearmed)
    await flushPromises()
    expect(wrapper.get('footer button').text()).toBe('Install app')
    await wrapper.get('footer button').trigger('click')
    expect(secondPrompt).toHaveBeenCalledOnce()
    wrapper.unmount()
  })
})
