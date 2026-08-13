import type { RefreshRun } from '../../api/generated/models'

export type RecoveryReason = RefreshRun['status'] | 'missing' | 'error' | 'timeout' | 'disposed' | 'obsolete'
export interface RecoveryResult { reason: RecoveryReason; refresh?: RefreshRun; error?: unknown }

const terminal = new Set<RefreshRun['status']>(['succeeded', 'partial_success', 'failed', 'cancelled'])

export class RefreshRecoveryPoller {
  private generation = 0
  private disposed = false
  private active?: AbortController

  async poll(load: (signal: AbortSignal) => Promise<RefreshRun | undefined>, intervalMs = 1_000, timeoutMs = 120_000): Promise<RecoveryResult> {
    const generation = ++this.generation
    const controller = new AbortController()
    this.active?.abort()
    this.active = controller
    const deadline = Date.now() + timeoutMs
    try {
      while (true) {
        if (this.disposed) return { reason: 'disposed' }
        if (generation !== this.generation) return { reason: 'obsolete' }
        if (Date.now() >= deadline) return { reason: 'timeout' }
        let refresh: RefreshRun | undefined
        try { refresh = await load(controller.signal) } catch (error) {
          if (this.disposed) return { reason: 'disposed' }
          if (generation !== this.generation) return { reason: 'obsolete' }
          return { reason: 'error', error }
        }
        if (!refresh) return { reason: 'missing' }
        if (terminal.has(refresh.status)) return { reason: refresh.status, refresh }
        await delay(intervalMs, controller.signal)
      }
    } finally {
      controller.abort()
      if (this.active === controller) this.active = undefined
    }
  }

  dispose() {
    this.disposed = true
    this.generation++
    this.active?.abort()
  }
}

function delay(milliseconds: number, signal: AbortSignal): Promise<void> {
  return new Promise((resolve) => {
    const timer = setTimeout(resolve, milliseconds)
    signal.addEventListener('abort', () => { clearTimeout(timer); resolve() }, { once: true })
  })
}
