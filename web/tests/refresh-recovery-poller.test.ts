import { describe, expect, it } from 'vitest'
import type { RefreshRun } from '../src/api/generated/models'
import { RefreshRecoveryPoller } from '../src/features/refresh/refresh-recovery-poller'

describe('feature refresh recovery poller', () => {
  it('reports disposal rather than missing when an in-flight request aborts', async () => {
    const poller = new RefreshRecoveryPoller()
    const promise = poller.poll((signal) => new Promise<RefreshRun>((_, reject) => signal.addEventListener('abort', () => reject(new DOMException('Aborted', 'AbortError')), { once: true })))
    poller.dispose()
    expect(await promise).toEqual({ reason: 'disposed' })
  })

  it('reports transient request errors without calling them missing', async () => {
    const failure = new TypeError('temporary network failure')
    const result = await new RefreshRecoveryPoller().poll(async () => { throw failure })
    expect(result).toEqual({ reason: 'error', error: failure })
  })
})
