import { describe, expect, it, vi } from 'vitest'

import { LocalApiClient } from './client'
import { apiContract, type ApiOperation } from './generated/operations'

const pathValues = {
  sourceId: 'source/with spaces',
  articleId: 'article/with spaces',
  refreshId: 'refresh/with spaces'
}

describe('LocalApiClient', () => {
  it('covers every generated operation with same-origin versioned requests', async () => {
    const calls: Array<[RequestInfo | URL, RequestInit | undefined]> = []
    const fetcher = vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      calls.push([input, init])
      return new Response(null, { status: 204 })
    })
    const client = new LocalApiClient(fetcher)
    const request = client.request.bind(client) as (operation: ApiOperation, value: unknown) => Promise<unknown>

    for (const [operation, contract] of Object.entries(apiContract) as Array<[ApiOperation, (typeof apiContract)[ApiOperation]]>) {
      const path = Object.fromEntries([...contract.path.matchAll(/\{([^}]+)\}/g)].map((match) => [match[1], pathValues[match[1] as keyof typeof pathValues]]))
      await request(operation, {
        ...(Object.keys(path).length > 0 ? { path } : {}),
        ...(operation === 'getFeed' ? { query: { sourceId: ['one', 'two'], includeHidden: false } } : {}),
        ...(['POST', 'PUT', 'PATCH'].includes(contract.method) && operation !== 'startRefresh' ? { body: {} } : {})
      })
    }

    expect(calls).toHaveLength(Object.keys(apiContract).length)
    for (const [input] of calls) {
      expect(input).toEqual(expect.stringMatching(/^\/api\/v1\//))
      expect(String(input)).not.toMatch(/^https?:/)
    }
    expect(calls.find(([input]) => String(input).startsWith('/api/v1/feed'))?.[0])
      .toBe('/api/v1/feed?sourceId=one&sourceId=two&includeHidden=false')
    expect(calls.some(([input]) => String(input).includes('with%20spaces'))).toBe(true)
  })

  it('uses the current origin implicitly, including a non-default loopback port', async () => {
    let requested: RequestInfo | URL | undefined
    const fetcher = vi.fn(async (input: RequestInfo | URL) => {
      requested = input
      return new Response(null, { status: 204 })
    })
    const client = new LocalApiClient(fetcher)

    await client.request('getHealth', undefined)

    expect(fetcher).toHaveBeenCalledWith('/api/v1/health', expect.any(Object))
    expect(requested).not.toContain('8787')
  })

  it('reports structured API failures without turning them into stale success', async () => {
    const detail = {
      code: 'unavailable' as const,
      message: 'database unavailable',
      correlationId: 'correlation-1',
      fields: []
    }
    const client = new LocalApiClient(async () => new Response(JSON.stringify(detail), {
      status: 503,
      headers: { 'Content-Type': 'application/json' }
    }))

    await expect(client.request('getHealth', undefined)).rejects.toEqual(
      expect.objectContaining({ status: 503, detail })
    )
  })

  it('propagates initial network unavailability', async () => {
    const unavailable = new TypeError('Failed to fetch')
    const client = new LocalApiClient(async () => { throw unavailable })

    await expect(client.request('getHealth', undefined)).rejects.toBe(unavailable)
  })
})
