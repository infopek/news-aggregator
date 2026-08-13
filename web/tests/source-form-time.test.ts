import { afterAll, beforeAll, describe, expect, it } from 'vitest'
import type { Source } from '../src/api/generated/models'
import { emptySource, sourceForm, sourceWrite, validateSource } from '../src/features/sources/source-form'

const originalTimezone = process.env.TZ
beforeAll(() => { process.env.TZ = 'Europe/Budapest' })
afterAll(() => { process.env.TZ = originalTimezone })

describe('scraper review instant', () => {
  it('round-trips unchanged through datetime-local in a non-UTC timezone', () => {
    const reviewedAt = '2026-08-13T10:15:00.000Z'
    const source: Source = { id: 'scraper', name: 'Scraper', url: 'https://example.com/news', kind: 'scraper', enabled: true, contentPermission: 'metadata_only', adapterConfig: { articleSelector: 'article', titleSelector: 'h1' }, scraperPolicy: { status: 'approved', termsUrl: 'https://example.com/terms', robotsUrl: 'https://example.com/robots.txt', reviewedAt, reviewNotes: null }, credentialConfigured: false, lastSuccessAt: null, lastError: null, retryAfter: null }
    const write = sourceWrite(sourceForm(source))
    expect(write.scraperPolicy.reviewedAt).toBe(reviewedAt)
  })

  it.each([['atom', 20], ['auto', 75]] as const)('preserves adapter configuration during harmless edits (%s, %d)', (format, pageSize) => {
    const base = { id: 'source', name: 'Source', url: 'https://example.com/news', enabled: true, contentPermission: 'metadata_only' as const, scraperPolicy: { status: 'not_applicable' as const, termsUrl: null, robotsUrl: null, reviewedAt: null, reviewNotes: null }, credentialConfigured: false, lastSuccessAt: null, lastError: null, retryAfter: null }
    expect(sourceWrite(sourceForm({ ...base, kind: 'feed', adapterConfig: { format } })).adapterConfig).toEqual({ format })
    expect(sourceWrite(sourceForm({ ...base, kind: 'api', adapterConfig: { provider: 'fictional', pageSize } })).adapterConfig).toEqual({ provider: 'fictional', pageSize })
  })

  it.each([
    ['review timestamp', { reviewedAt: '' }],
    ['Terms URL', { termsUrl: 'ftp://example.com/terms' }],
    ['robots URL', { robotsUrl: 'not-a-url' }],
    ['review notes', { reviewNotes: ' ' }]
  ])('validates approved scraper %s', (message, override) => {
    const form = { ...emptySource(), name: 'Scraper', url: 'https://example.com/pages', kind: 'scraper' as const, enabled: true, policyStatus: 'approved' as const, reviewedAt: '2026-08-13T10:00', termsUrl: 'https://example.com/terms', robotsUrl: 'https://example.com/robots.txt', reviewNotes: 'Reviewed manually', ...override }
    expect(validateSource(form).join(' ')).toContain(message)
  })
})
