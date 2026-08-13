import { afterEach, describe, expect, it, vi } from 'vitest'

import refreshFixture from '../../../test/fixtures/api/refresh-partial.json'
import type { RefreshRun } from '../api/generated/models'
import { RefreshPoller } from './refresh-poller'

const running: RefreshRun = { ...refreshFixture, status: 'running', finishedAt: null }

afterEach(() => vi.useRealTimers())

describe('RefreshPoller', () => {
  it.each(['succeeded', 'partial_success', 'failed', 'cancelled'] as const)('stops on %s', async (status) => {
    const poller = new RefreshPoller()
    const result = await poller.poll(async () => ({ ...refreshFixture, status }))
    expect(result.reason).toBe(status)
  })

  it('stops when status is missing', async () => {
    expect(await new RefreshPoller().poll(async () => undefined)).toEqual({ reason: 'missing' })
  })

  it('stops at the timeout with fake timers', async () => {
    vi.useFakeTimers()
    const promise = new RefreshPoller().poll(async () => running, { intervalMs: 100, timeoutMs: 250 })
    await vi.advanceTimersByTimeAsync(300)
    expect(await promise).toEqual({ reason: 'timeout' })
  })

  it('stops immediately on disposal', async () => {
    vi.useFakeTimers()
    const poller = new RefreshPoller()
    const promise = poller.poll(async () => running, { intervalMs: 10_000 })
    await vi.advanceTimersByTimeAsync(0)
    poller.dispose()
    await vi.advanceTimersByTimeAsync(0)
    expect(await promise).toEqual({ reason: 'disposed' })
  })

  it('makes an older poll obsolete when a new poll starts', async () => {
    vi.useFakeTimers()
    const poller = new RefreshPoller()
    const old = poller.poll(async () => running, { intervalMs: 10_000 })
    await vi.advanceTimersByTimeAsync(0)
    const current = poller.poll(async () => ({ ...refreshFixture, status: 'succeeded' }))
    await vi.advanceTimersByTimeAsync(0)
    expect(await old).toEqual({ reason: 'obsolete' })
    expect((await current).reason).toBe('succeeded')
  })

  it('stops on external cancellation', async () => {
    vi.useFakeTimers()
    const controller = new AbortController()
    const promise = new RefreshPoller().poll(async () => running, { intervalMs: 10_000, signal: controller.signal })
    await vi.advanceTimersByTimeAsync(0)
    controller.abort()
    await vi.advanceTimersByTimeAsync(0)
    expect(await promise).toEqual({ reason: 'disposed' })
  })

  it('reports disposal rather than missing when an in-flight request aborts', async () => {
    const poller = new RefreshPoller()
    const promise = poller.poll((signal) => new Promise<RefreshRun>((_, reject) => signal.addEventListener('abort', () => reject(new DOMException('Aborted', 'AbortError')), { once: true })))
    poller.dispose()
    expect(await promise).toEqual({ reason: 'disposed' })
  })

  it('reports transient request errors without calling them missing', async () => {
    const failure = new TypeError('temporary network failure')
    const result = await new RefreshPoller().poll(async () => { throw failure })
    expect(result).toEqual({ reason: 'error', error: failure })
  })
})
