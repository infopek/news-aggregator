import { describe, expect, it, vi } from 'vitest'

import { ApiRequestError } from '../api/client'
import conflict from '../../../test/fixtures/api/conflict-error.json'
import unavailable from '../../../test/fixtures/api/unavailable-error.json'
import unexpected from '../../../test/fixtures/api/unexpected-error.json'
import validation from '../../../test/fixtures/api/validation-error.json'
import { toUserSafeError } from './errors'
import { ServerStateClient } from './query-client'
import { queryKeys } from './query-keys'

describe('ServerStateClient', () => {
  it('exposes loading, empty, success and keeps stale data during reload', async () => {
    const cache = new ServerStateClient()
    let resolve!: (value: { items: string[] }) => void
    const pending = new Promise<{ items: string[] }>((done) => { resolve = done })
    const promise = cache.query(queryKeys.sources(), () => pending, { isEmpty: (value) => value.items.length === 0 })
    expect(cache.state(queryKeys.sources()).status).toBe('loading')
    resolve({ items: [] })
    expect((await promise).status).toBe('empty')

    const reload = cache.query(queryKeys.sources(), async () => ({ items: ['one'] }), { isEmpty: (value) => value.items.length === 0 })
    expect(cache.state<{ items: string[] }>(queryKeys.sources())).toMatchObject({ status: 'loading', data: { items: [] } })
    expect(await reload).toMatchObject({ status: 'success', data: { items: ['one'] } })
  })

  it('aborts and ignores an obsolete request for the same key', async () => {
    const cache = new ServerStateClient()
    let firstSignal!: AbortSignal
    let finish!: (value: string) => void
    const first = cache.query(queryKeys.profile(), (signal) => {
      firstSignal = signal
      return new Promise<string>((resolve) => { finish = resolve })
    })
    const second = cache.query(queryKeys.profile(), async () => 'new')
    finish('old')
    await Promise.all([first, second])
    expect(firstSignal.aborted).toBe(true)
    expect(cache.state<string>(queryKeys.profile())).toMatchObject({ status: 'success', data: 'new' })
  })

  it('retries only transient failures within the configured bound', async () => {
    const cache = new ServerStateClient()
    const loader = vi.fn().mockRejectedValueOnce(new TypeError('network')).mockResolvedValue('ready')
    expect(await cache.query(queryKeys.health(), loader, { retries: 1 })).toMatchObject({ status: 'success' })
    expect(loader).toHaveBeenCalledTimes(2)

    const validationError = new ApiRequestError(400, validation as never)
    const invalid = vi.fn().mockRejectedValue(validationError)
    expect(await cache.query(queryKeys.profile(), invalid, { retries: 3 })).toMatchObject({ status: 'error', error: { family: 'validation' } })
    expect(invalid).toHaveBeenCalledOnce()
  })
})

describe('safe API errors', () => {
  it.each([
    [validation, 400, 'validation'],
    [conflict, 409, 'conflict'],
    [unavailable, 503, 'unavailable'],
    [unexpected, 500, 'unexpected']
  ])('maps the fixture error family %#', (fixture, status, family) => {
    expect(toUserSafeError(new ApiRequestError(status, fixture as never)).family).toBe(family)
  })

  it('does not expose messages from unknown server error codes', () => {
    const mapped = toUserSafeError(new ApiRequestError(500, unexpected as never))
    expect(JSON.stringify(mapped)).not.toContain(unexpected.message)
  })
})
