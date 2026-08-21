import type {
  CredentialStatus,
  CredentialWrite,
  FeedQuery,
  FeedFilterWrite,
  LibraryStateWrite,
  ProfileWrite,
  RankingConfigurationWrite,
  SourceWrite
} from './generated/models'
import type { ApiOperation, ApiOperationMap } from './generated/operations'

export interface RequestClient {
  request<Operation extends ApiOperation>(
    operation: Operation,
    request: ApiOperationMap[Operation]['request'],
    signal?: AbortSignal
  ): Promise<ApiOperationMap[Operation]['response']>
}

export function createServerApi(client: RequestClient) {
  return {
    health: (signal?: AbortSignal) => client.request('getHealth', undefined, signal),
    profile: (signal?: AbortSignal) => client.request('getProfile', undefined, signal),
    updateProfile: (body: ProfileWrite, signal?: AbortSignal) => client.request('putProfile', { body }, signal),
    ranking: (signal?: AbortSignal) => client.request('getRankingConfig', undefined, signal),
    updateRanking: (body: RankingConfigurationWrite, signal?: AbortSignal) => client.request('putRankingConfig', { body }, signal),
    starterSources: (signal?: AbortSignal) => client.request('listStarterSources', undefined, signal),
    sources: (signal?: AbortSignal) => client.request('listSources', undefined, signal),
    source: (sourceId: string, signal?: AbortSignal) => client.request('getSource', { path: { sourceId } }, signal),
    createSource: (body: SourceWrite, signal?: AbortSignal) => client.request('createSource', { body }, signal),
    updateSource: (sourceId: string, body: SourceWrite, signal?: AbortSignal) => client.request('updateSource', { path: { sourceId }, body }, signal),
    deleteSource: (sourceId: string, signal?: AbortSignal) => client.request('deleteSource', { path: { sourceId } }, signal),
    writeCredential: (sourceId: string, body: CredentialWrite, signal?: AbortSignal): Promise<CredentialStatus> =>
      client.request('putSourceCredential', { path: { sourceId }, body }, signal),
    deleteCredential: (sourceId: string, signal?: AbortSignal) => client.request('deleteSourceCredential', { path: { sourceId } }, signal),
    startRefresh: (signal?: AbortSignal) => client.request('startRefresh', undefined, signal),
    refresh: (refreshId: string, signal?: AbortSignal) => client.request('getRefresh', { path: { refreshId } }, signal),
    feed: (query: FeedQuery, signal?: AbortSignal) => client.request('getFeed', { query }, signal),
    feedFilter: (signal?: AbortSignal) => client.request('getFeedFilter', undefined, signal),
    updateFeedFilter: (body: FeedFilterWrite, signal?: AbortSignal) => client.request('putFeedFilter', { body }, signal),
    article: (articleId: string, signal?: AbortSignal) => client.request('getArticle', { path: { articleId } }, signal),
    updateLibrary: (articleId: string, body: LibraryStateWrite, signal?: AbortSignal) =>
      client.request('patchLibraryState', { path: { articleId }, body }, signal)
  }
}

export type ServerApi = ReturnType<typeof createServerApi>
