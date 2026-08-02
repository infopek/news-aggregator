import type { FeedQuery } from '../api/generated/models'

export type QueryKey = readonly [string, ...unknown[]]

function normalizedFeed(query: FeedQuery): string {
  return JSON.stringify({
    ...query,
    sourceId: query.sourceId ? [...query.sourceId].sort() : undefined
  })
}

export const queryKeys = {
  health: (): QueryKey => ['health'],
  profile: (): QueryKey => ['profile'],
  ranking: (): QueryKey => ['ranking'],
  starterSources: (): QueryKey => ['starter-sources'],
  sources: (): QueryKey => ['sources'],
  source: (sourceId: string): QueryKey => ['source', sourceId],
  feeds: (): QueryKey => ['feed'],
  feed: (query: FeedQuery): QueryKey => ['feed', normalizedFeed(query)],
  article: (articleId: string): QueryKey => ['article', articleId],
  refresh: (refreshId: string): QueryKey => ['refresh', refreshId]
} as const

export function serializeKey(key: QueryKey): string {
  return JSON.stringify(key)
}
