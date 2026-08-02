import { describe, expect, it, vi } from 'vitest'

import credentialFixture from '../../../test/fixtures/api/credential-status.json'
import libraryFixture from '../../../test/fixtures/api/library-state.json'
import profileFixture from '../../../test/fixtures/api/profile.json'
import rankingFixture from '../../../test/fixtures/api/ranking-configuration.json'
import sourceFixture from '../../../test/fixtures/api/source.json'
import { ApiRequestError } from '../api/client'
import type { ServerApi } from '../api/server-api'
import { ServerMutations } from './mutations'
import { ServerStateClient } from './query-client'
import { queryKeys, serializeKey, type QueryKey } from './query-keys'

const articleId = libraryFixture.articleId
const otherSourceId = 'source-unrelated'
const feedA = queryKeys.feed({ saved: true })
const feedB = queryKeys.feed({ read: false, sourceId: [sourceFixture.id] })

function apiWith(overrides: Partial<ServerApi>): ServerApi {
  return overrides as ServerApi
}

function seededCache(): ServerStateClient {
  const cache = new ServerStateClient()
  cache.set(queryKeys.profile(), { seed: 'profile' })
  cache.set(queryKeys.ranking(), { seed: 'ranking' })
  cache.set(queryKeys.sources(), { seed: 'sources' })
  cache.set(queryKeys.source(sourceFixture.id), { seed: 'source' })
  cache.set(queryKeys.source(otherSourceId), { seed: 'other-source' })
  cache.set(feedA, { seed: 'feed-a' })
  cache.set(feedB, { seed: 'feed-b' })
  cache.set(queryKeys.article(articleId), { seed: 'article' })
  cache.set(queryKeys.articleLibrary(articleId), { seed: 'library' })
  cache.set(queryKeys.article('article-unrelated'), { seed: 'other-article' })
  return cache
}

const allKeys: QueryKey[] = [
  queryKeys.profile(), queryKeys.ranking(), queryKeys.sources(),
  queryKeys.source(sourceFixture.id), queryKeys.source(otherSourceId),
  feedA, feedB, queryKeys.article(articleId), queryKeys.articleLibrary(articleId),
  queryKeys.article('article-unrelated')
]

interface MutationCase {
  name: string
  api: Partial<ServerApi>
  invoke: (mutations: ServerMutations) => Promise<unknown>
  stale: QueryKey[]
  retained?: { key: QueryKey; value: unknown }
  failureStale?: QueryKey[]
}

function cases(reject = false): MutationCase[] {
  const response = <T>(value: T) => reject ? vi.fn().mockRejectedValue(new TypeError('response lost after commit')) : vi.fn().mockResolvedValue(value)
  return [
    {
      name: 'profile', api: { updateProfile: response(profileFixture) },
      invoke: (mutations) => mutations.updateProfile({} as never), stale: [feedA, feedB],
      retained: { key: queryKeys.profile(), value: profileFixture }, failureStale: [queryKeys.profile(), feedA, feedB]
    },
    {
      name: 'ranking', api: { updateRanking: response(rankingFixture) },
      invoke: (mutations) => mutations.updateRanking({} as never), stale: [feedA, feedB],
      retained: { key: queryKeys.ranking(), value: rankingFixture }, failureStale: [queryKeys.ranking(), feedA, feedB]
    },
    {
      name: 'source create', api: { createSource: response(sourceFixture) },
      invoke: (mutations) => mutations.createSource({} as never), stale: [queryKeys.sources(), feedA, feedB],
      retained: { key: queryKeys.source(sourceFixture.id), value: sourceFixture }
    },
    {
      name: 'source update', api: { updateSource: response(sourceFixture) },
      invoke: (mutations) => mutations.updateSource(sourceFixture.id, {} as never), stale: [queryKeys.sources(), feedA, feedB],
      retained: { key: queryKeys.source(sourceFixture.id), value: sourceFixture }, failureStale: [queryKeys.source(sourceFixture.id), queryKeys.sources(), feedA, feedB]
    },
    {
      name: 'source delete', api: { deleteSource: response(undefined) },
      invoke: (mutations) => mutations.deleteSource(sourceFixture.id), stale: [queryKeys.source(sourceFixture.id), queryKeys.sources(), feedA, feedB]
    },
    {
      name: 'credential write', api: { writeCredential: response(credentialFixture) },
      invoke: (mutations) => mutations.writeCredential(sourceFixture.id, { secret: 'transport-only' }), stale: [queryKeys.source(sourceFixture.id), queryKeys.sources()]
    },
    {
      name: 'credential delete', api: { deleteCredential: response(credentialFixture) },
      invoke: (mutations) => mutations.deleteCredential(sourceFixture.id), stale: [queryKeys.source(sourceFixture.id), queryKeys.sources()]
    },
    {
      name: 'article library', api: { updateLibrary: response(libraryFixture) },
      invoke: (mutations) => mutations.updateLibrary(articleId, { saved: true }), stale: [queryKeys.article(articleId), feedA, feedB],
      retained: { key: queryKeys.articleLibrary(articleId), value: libraryFixture },
      failureStale: [queryKeys.article(articleId), queryKeys.articleLibrary(articleId), feedA, feedB]
    }
  ]
}

describe('ServerMutations invalidation and reconciliation matrix', () => {
  it.each(cases())('$name invalidates exactly its dependencies and retains authoritative data', async ({ api, invoke, stale, retained }) => {
    const cache = seededCache()
    const result = await invoke(new ServerMutations(apiWith(api), cache))

    expect(result).toMatchObject({ status: 'success' })
    for (const key of stale) expect(cache.state(key).status, `${serializeKey(key)} stale`).toBe('idle')
    if (retained) expect(cache.state(retained.key)).toEqual({ status: 'success', data: retained.value })

    const changed = new Set([...stale, ...(retained ? [retained.key] : [])].map(serializeKey))
    for (const key of allKeys) {
      if (!changed.has(serializeKey(key))) expect(cache.state(key).status, `${serializeKey(key)} preserved`).toBe('success')
    }
  })

  it.each(cases(true))('$name invalidates the same dependencies when its response is lost after commit', async ({ api, invoke, stale, failureStale }) => {
    const cache = seededCache()
    const result = await invoke(new ServerMutations(apiWith(api), cache))
    expect(result).toMatchObject({ status: 'error' })
    const expected = failureStale ?? stale
    for (const key of expected) expect(cache.state(key).status, `${serializeKey(key)} stale`).toBe('idle')
    const changed = new Set(expected.map(serializeKey))
    for (const key of allKeys) {
      if (!changed.has(serializeKey(key))) expect(cache.state(key).status, `${serializeKey(key)} preserved`).toBe('success')
    }
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

  it('never places credential input or echoed error detail in cache, errors, logs, or snapshots', async () => {
    const sentinel = 'SENTINEL-CREDENTIAL-NEVER-LEAK'
    const cache = seededCache()
    const logger = vi.spyOn(console, 'error').mockImplementation(() => undefined)
    const writeCredential = vi.fn().mockRejectedValue(new ApiRequestError(400, {
      code: 'validation_failed', message: `Rejected value ${sentinel}`, correlationId: 'credential-test',
      fields: [{ path: 'secret', code: 'invalid', message: sentinel }]
    }))

    const result = await new ServerMutations(apiWith({ writeCredential }), cache).writeCredential(sourceFixture.id, { secret: sentinel })
    const observable = JSON.stringify({ result, cache: cache.serialize(), logs: logger.mock.calls })

    expect(observable).not.toContain(sentinel)
    expect(writeCredential).toHaveBeenCalledOnce()
    expect(logger).not.toHaveBeenCalled()
    logger.mockRestore()
  })
})
