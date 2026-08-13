import { afterAll, beforeAll, describe, expect, it } from 'vitest'
import type { Source } from '../src/api/generated/models'
import { sourceForm, sourceWrite } from '../src/features/sources/source-form'

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
})
