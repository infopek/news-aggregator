import type { ArticleSummary, RefreshRun, ScoreContribution } from '../api/generated/models'

export const contribution = (overrides: Partial<ScoreContribution> = {}): ScoreContribution => ({
  signal: 'interest', rawScore: 0.8, weight: 0.5, weightedScore: 0.4,
  reasonCode: 'explicit_interest_match', reasonValues: {}, ...overrides,
})

export const articleSummary = (overrides: Partial<ArticleSummary> = {}): ArticleSummary => ({
  id: 'article-1', sourceId: 'source-1', canonicalUrl: 'https://publisher.example/story',
  title: 'A useful local story', publishedAt: '2026-08-01T12:00:00Z', fetchedAt: '2026-08-01T12:05:00Z',
  excerpt: 'A plain-text summary.', contentPermission: 'metadata_only', language: 'en', topics: ['local'],
  library: { articleId: 'article-1', readAt: null, savedAt: null, hiddenAt: null },
  ranking: { score: 0.4, contributions: [contribution()], algorithmVersion: 'v1', calculatedAt: '2026-08-01T12:05:00Z' },
  ...overrides,
})

export const partialRefresh = (overrides: Partial<RefreshRun> = {}): RefreshRun => ({
  id: 'refresh-1', status: 'partial_success', startedAt: '2026-08-01T12:00:00Z', finishedAt: '2026-08-01T12:01:00Z',
  outcomes: [
    { sourceId: 'source-good', fetched: 2, inserted: 2, updated: 0, skipped: 0, failed: 0, errorCode: null, errorSummary: null },
    { sourceId: 'source-bad', fetched: 0, inserted: 0, updated: 0, skipped: 0, failed: 1, errorCode: 'unavailable', errorSummary: 'Timed out' },
  ], ...overrides,
})
