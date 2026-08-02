import type { RefreshRun } from '../api/generated/models'

export type PollStopReason = RefreshRun['status'] | 'missing' | 'timeout' | 'disposed' | 'obsolete'

export interface PollResult {
  reason: PollStopReason
  refresh?: RefreshRun
}

export interface PollOptions {
  intervalMs?: number
  timeoutMs?: number
  signal?: AbortSignal
}

const terminal = new Set<RefreshRun['status']>(['succeeded', 'partial_success', 'failed', 'cancelled'])

export class RefreshPoller {
  private generation = 0
  private disposed = false
  private active?: AbortController

  async poll(load: (signal: AbortSignal) => Promise<RefreshRun | undefined>, options: PollOptions = {}): Promise<PollResult> {
    const generation = ++this.generation
    const controller = new AbortController()
    this.active?.abort()
    this.active = controller
    const interval = options.intervalMs ?? 1_000
    const deadline = Date.now() + (options.timeoutMs ?? 120_000)
    const abort = () => controller.abort()
    options.signal?.addEventListener('abort', abort, { once: true })
    try {
      while (true) {
        if (this.disposed || options.signal?.aborted) return { reason: 'disposed' }
        if (generation !== this.generation) return { reason: 'obsolete' }
        if (Date.now() >= deadline) return { reason: 'timeout' }
        let refresh: RefreshRun | undefined
        try {
          refresh = await load(controller.signal)
        } catch {
          refresh = undefined
        }
        if (!refresh) return { reason: 'missing' }
        if (terminal.has(refresh.status)) return { reason: refresh.status, refresh }
        await delay(interval, controller.signal)
      }
    } finally {
      options.signal?.removeEventListener('abort', abort)
      controller.abort()
      if (this.active === controller) this.active = undefined
    }
  }

  obsolete(): void {
    this.generation += 1
    this.active?.abort()
  }

  dispose(): void {
    this.disposed = true
    this.obsolete()
  }
}

function delay(milliseconds: number, signal: AbortSignal): Promise<void> {
  return new Promise((resolve) => {
    const timer = setTimeout(resolve, milliseconds)
    signal.addEventListener('abort', () => {
      clearTimeout(timer)
      resolve()
    }, { once: true })
  })
}
