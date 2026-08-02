import type { APIError } from './generated/models'
import {
  apiContract,
  type ApiClient,
  type ApiOperation,
  type ApiOperationMap
} from './generated/operations'

type Fetch = typeof globalThis.fetch
type OperationRequest<Operation extends ApiOperation> = ApiOperationMap[Operation]['request']

export class ApiRequestError extends Error {
  constructor(
    readonly status: number,
    readonly detail: APIError | undefined
  ) {
    super(detail?.message ?? `Local API request failed with status ${status}`)
    this.name = 'ApiRequestError'
  }
}

export class LocalApiClient implements ApiClient {
  constructor(private readonly fetcher: Fetch = globalThis.fetch.bind(globalThis)) {}

  async request<Operation extends ApiOperation>(
    operation: Operation,
    request: OperationRequest<Operation>
  ): Promise<ApiOperationMap[Operation]['response']> {
    const contract = apiContract[operation]
    const parts = (request ?? {}) as {
      path?: Record<string, string>
      query?: Record<string, unknown>
      body?: unknown
    }
    const path = interpolatePath(contract.path, parts.path)
    const url = appendQuery(path, parts.query)
    const hasBody = Object.hasOwn(parts, 'body')
    const response = await this.fetcher(url, {
      method: contract.method,
      headers: hasBody ? { 'Content-Type': 'application/json' } : undefined,
      body: hasBody ? JSON.stringify(parts.body) : undefined,
      cache: contract.method === 'GET' ? 'no-store' : undefined
    })

    if (!response.ok) {
      throw new ApiRequestError(response.status, await readApiError(response))
    }
    if (response.status === 204) return undefined as ApiOperationMap[Operation]['response']
    return await response.json() as ApiOperationMap[Operation]['response']
  }
}

export const api = new LocalApiClient()

function interpolatePath(template: string, values: Record<string, string> | undefined): string {
  const path = template.replace(/\{([^}]+)\}/g, (_, key: string) => {
    const value = values?.[key]
    if (value === undefined) throw new TypeError(`Missing API path parameter: ${key}`)
    return encodeURIComponent(value)
  })
  if (!path.startsWith('/api/v1/')) throw new TypeError(`Refusing non-local API path: ${path}`)
  return path
}

function appendQuery(path: string, query: Record<string, unknown> | undefined): string {
  if (!query) return path
  const parameters = new URLSearchParams()
  for (const [key, rawValue] of Object.entries(query)) {
    if (rawValue === undefined) continue
    const values = Array.isArray(rawValue) ? rawValue : [rawValue]
    for (const value of values) parameters.append(key, String(value))
  }
  const encoded = parameters.toString()
  return encoded ? `${path}?${encoded}` : path
}

async function readApiError(response: Response): Promise<APIError | undefined> {
  if (!response.headers.get('content-type')?.includes('application/json')) return undefined
  try {
    return await response.json() as APIError
  } catch {
    return undefined
  }
}
