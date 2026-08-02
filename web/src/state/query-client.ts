import type { UserSafeError } from './errors'
import { isRetryable, toUserSafeError } from './errors'
import type { QueryKey } from './query-keys'
import { serializeKey } from './query-keys'

export type QueryState<T> =
  | { status: 'idle'; data?: undefined; error?: undefined }
  | { status: 'loading'; data?: T; error?: undefined }
  | { status: 'empty'; data: T; error?: undefined }
  | { status: 'success'; data: T; error?: undefined }
  | { status: 'error'; data?: T; error: UserSafeError }

interface Entry<T = unknown> {
  key: QueryKey
  state: QueryState<T>
  generation: number
  controller?: AbortController
}

export interface QueryOptions<T> {
  retries?: number
  isEmpty?: (value: T) => boolean
}

export class ServerStateClient {
  private readonly entries = new Map<string, Entry>()

  state<T>(key: QueryKey): QueryState<T> {
    return (this.entries.get(serializeKey(key))?.state as QueryState<T> | undefined) ?? { status: 'idle' }
  }

  async query<T>(key: QueryKey, loader: (signal: AbortSignal) => Promise<T>, options: QueryOptions<T> = {}): Promise<QueryState<T>> {
    const id = serializeKey(key)
    const previous = this.entries.get(id) as Entry<T> | undefined
    previous?.controller?.abort()
    const controller = new AbortController()
    const entry: Entry<T> = {
      key,
      state: { status: 'loading', data: previous?.state.data },
      generation: (previous?.generation ?? 0) + 1,
      controller
    }
    this.entries.set(id, entry)

    try {
      const value = await retry(() => loader(controller.signal), options.retries ?? 1, controller.signal)
      if (!this.isCurrent(id, entry)) return entry.state
      entry.state = options.isEmpty?.(value) ? { status: 'empty', data: value } : { status: 'success', data: value }
    } catch (error) {
      if (controller.signal.aborted || !this.isCurrent(id, entry)) return entry.state
      entry.state = { status: 'error', data: entry.state.data, error: toUserSafeError(error) }
    } finally {
      if (this.isCurrent(id, entry)) entry.controller = undefined
    }
    return entry.state
  }

  set<T>(key: QueryKey, value: T, isEmpty: (value: T) => boolean = () => false): void {
    const id = serializeKey(key)
    const previous = this.entries.get(id)
    previous?.controller?.abort()
    this.entries.set(id, { key, generation: (previous?.generation ?? 0) + 1, state: isEmpty(value) ? { status: 'empty', data: value } : { status: 'success', data: value } })
  }

  invalidate(...prefixes: QueryKey[]): void {
    for (const [id, entry] of this.entries) {
      if (prefixes.some((prefix) => hasPrefix(entry.key, prefix))) {
        entry.controller?.abort()
        this.entries.delete(id)
      }
    }
  }

  cancel(key: QueryKey): void {
    const id = serializeKey(key)
    const entry = this.entries.get(id)
    entry?.controller?.abort()
    this.entries.delete(id)
  }

  serialize(): string {
    return JSON.stringify([...this.entries].map(([key, entry]) => [key, entry.state]))
  }

  private isCurrent(id: string, entry: Entry): boolean {
    return this.entries.get(id) === entry
  }
}

function hasPrefix(key: QueryKey, prefix: QueryKey): boolean {
  return prefix.every((part, index) => Object.is(key[index], part))
}

async function retry<T>(operation: () => Promise<T>, retries: number, signal: AbortSignal): Promise<T> {
  let attempt = 0
  while (true) {
    try {
      return await operation()
    } catch (error) {
      if (signal.aborted || attempt >= retries || !isRetryable(error)) throw error
      attempt += 1
    }
  }
}
