import { ApiRequestError } from '../api/client'
import type { APIErrorField } from '../api/generated/models'

export type UserSafeError =
  | { family: 'validation'; message: string; fields: APIErrorField[] }
  | { family: 'conflict'; message: string; fields: [] }
  | { family: 'unavailable'; message: string; fields: [] }
  | { family: 'unexpected'; message: string; fields: [] }

const unavailableCodes = new Set(['unavailable', 'unsupported_platform'])
const conflictCodes = new Set(['conflict', 'refresh_active'])

export function toUserSafeError(error: unknown): UserSafeError {
  if (error instanceof ApiRequestError) {
    const code: string | undefined = error.detail?.code
    if (code === 'validation_failed') {
      return { family: 'validation', message: error.detail?.message ?? 'Check the highlighted values.', fields: error.detail?.fields ?? [] }
    }
    if (code && conflictCodes.has(code)) return { family: 'conflict', message: error.detail?.message ?? 'The item changed. Reload and try again.', fields: [] }
    if (code && unavailableCodes.has(code)) return { family: 'unavailable', message: 'The local service is temporarily unavailable.', fields: [] }
  }
  if (error instanceof TypeError) return { family: 'unavailable', message: 'The local service is temporarily unavailable.', fields: [] }
  return { family: 'unexpected', message: 'Something unexpected happened. Try again.', fields: [] }
}

export function isRetryable(error: unknown): boolean {
  if (error instanceof TypeError) return true
  return error instanceof ApiRequestError && (error.status >= 500 || error.status === 429)
}
