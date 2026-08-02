import { describe, expect, it, vi } from 'vitest'

import sourceFixture from '../../../test/fixtures/api/source.json'
import { ApiRequestError } from '../api/client'
import type { ServerApi } from '../api/server-api'
import { ServerMutations } from './mutations'
import { ServerStateClient } from './query-client'
import { queryKeys } from './query-keys'

function apiWith(overrides: Partial<ServerApi>): ServerApi {
  return overrides as ServerApi
}

describe('ServerMutations', () => {
  it('invalidates only source/feed state and reconciles the authoritative source', async () => {
    const cache = new ServerStateClient()
    cache.set(queryKeys.profile(), { id: 'profile' })
    cache.set(queryKeys.sources(), { items: [] })
    cache.set(queryKeys.feed({ saved: true }), { items: ['old'] })
    cache.set(queryKeys.article('unrelated'), { title: 'keep' })
    const authoritative = { ...sourceFixture, name: 'Server normalized' }
    const mutations = new ServerMutations(apiWith({ createSource: vi.fn().mockResolvedValue(authoritative) }), cache)

    const result = await mutations.createSource({} as never)

    expect(result).toMatchObject({ status: 'success', data: authoritative })
    expect(cache.state(queryKeys.sources()).status).toBe('idle')
    expect(cache.state(queryKeys.feed({ saved: true })).status).toBe('idle')
    expect(cache.state(queryKeys.profile()).status).toBe('success')
    expect(cache.state(queryKeys.article('unrelated')).status).toBe('success')
    expect(cache.state(queryKeys.source(sourceFixture.id))).toMatchObject({ status: 'success', data: authoritative })
  })

  it('invalidates after a lost response because the server may have committed', async () => {
    const cache = new ServerStateClient()
    cache.set(queryKeys.sources(), { items: [sourceFixture] })
    const mutations = new ServerMutations(apiWith({ updateSource: vi.fn().mockRejectedValue(new TypeError('response lost')) }), cache)

    expect(await mutations.updateSource(sourceFixture.id, {} as never)).toMatchObject({ status: 'error', error: { family: 'unavailable' } })
    expect(cache.state(queryKeys.sources()).status).toBe('idle')
  })

  it('sequences rapid mutations and aborts the superseded request', async () => {
    let firstSignal!: AbortSignal
    let rejectFirst!: (error: unknown) => void
    const updateSource = vi.fn()
      .mockImplementationOnce((_id, _body, signal: AbortSignal) => {
        firstSignal = signal
        return new Promise((_resolve, reject) => { rejectFirst = reject })
      })
      .mockResolvedValueOnce(sourceFixture)
    const mutations = new ServerMutations(apiWith({ updateSource }), new ServerStateClient())
    const first = mutations.updateSource(sourceFixture.id, {} as never)
    const second = mutations.updateSource(sourceFixture.id, {} as never)
    rejectFirst(new DOMException('aborted', 'AbortError'))

    expect((await first).status).toBe('error')
    expect((await second).status).toBe('success')
    expect(firstSignal.aborted).toBe(true)
  })

  it('never places credential input in cache, error, logging, or serialization', async () => {
    const sentinel = 'SENTINEL-CREDENTIAL-NEVER-LEAK'
    const cache = new ServerStateClient()
    const logger = vi.spyOn(console, 'error').mockImplementation(() => undefined)
    const writeCredential = vi.fn().mockRejectedValue(new ApiRequestError(400, {
      code: 'validation_failed',
      message: `Rejected value ${sentinel}`,
      correlationId: 'credential-test',
      fields: [{ path: 'secret', code: 'invalid', message: sentinel }]
    }))
    const mutations = new ServerMutations(apiWith({ writeCredential }), cache)

    const result = await mutations.writeCredential(sourceFixture.id, { secret: sentinel })
    const observable = JSON.stringify({ result, cache: cache.serialize(), logs: logger.mock.calls })

    expect(observable).not.toContain(sentinel)
    expect(writeCredential).toHaveBeenCalledOnce()
    expect(logger).not.toHaveBeenCalled()
    logger.mockRestore()
  })
})
