// Generated from api/openapi.yaml (dd8be64ea54d6638). Do not edit.
import type * as Models from './models'

export interface ApiOperationMap {
  getHealth: { method: "GET"; path: "/api/v1/health"; request: undefined; response: Models.Health }
  getProfile: { method: "GET"; path: "/api/v1/profile"; request: undefined; response: Models.Profile }
  putProfile: { method: "PUT"; path: "/api/v1/profile"; request: { body: Models.ProfileWrite }; response: Models.Profile }
  getRankingConfig: { method: "GET"; path: "/api/v1/ranking-config"; request: undefined; response: Models.RankingConfiguration }
  putRankingConfig: { method: "PUT"; path: "/api/v1/ranking-config"; request: { body: Models.RankingConfigurationWrite }; response: Models.RankingConfiguration }
  listStarterSources: { method: "GET"; path: "/api/v1/starter-sources"; request: undefined; response: Models.SourceList }
  listSources: { method: "GET"; path: "/api/v1/sources"; request: undefined; response: Models.SourceList }
  createSource: { method: "POST"; path: "/api/v1/sources"; request: { body: Models.SourceWrite }; response: Models.Source }
  getSource: { method: "GET"; path: "/api/v1/sources/{sourceId}"; request: { path: Models.SourceIDRequest }; response: Models.Source }
  updateSource: { method: "PATCH"; path: "/api/v1/sources/{sourceId}"; request: { path: Models.SourceIDRequest; body: Models.SourceWrite }; response: Models.Source }
  deleteSource: { method: "DELETE"; path: "/api/v1/sources/{sourceId}"; request: { path: Models.SourceIDRequest }; response: undefined }
  putSourceCredential: { method: "PUT"; path: "/api/v1/sources/{sourceId}/credential"; request: { path: Models.SourceIDRequest; body: Models.CredentialWrite }; response: Models.CredentialStatus }
  deleteSourceCredential: { method: "DELETE"; path: "/api/v1/sources/{sourceId}/credential"; request: { path: Models.SourceIDRequest }; response: Models.CredentialStatus }
  startRefresh: { method: "POST"; path: "/api/v1/refresh"; request: undefined; response: Models.RefreshRun }
  getRefresh: { method: "GET"; path: "/api/v1/refresh/{refreshId}"; request: { path: Models.RefreshIDRequest }; response: Models.RefreshRun }
  getFeed: { method: "GET"; path: "/api/v1/feed"; request: { query: Models.FeedQuery }; response: Models.FeedPage }
  getFeedFilter: { method: "GET"; path: "/api/v1/feed-filter"; request: undefined; response: Models.FeedFilterState }
  putFeedFilter: { method: "PUT"; path: "/api/v1/feed-filter"; request: { body: Models.FeedFilterWrite }; response: Models.FeedFilterState }
  getArticle: { method: "GET"; path: "/api/v1/articles/{articleId}"; request: { path: Models.ArticleIDRequest }; response: Models.ArticleDetail }
  patchLibraryState: { method: "PATCH"; path: "/api/v1/articles/{articleId}/library-state"; request: { path: Models.ArticleIDRequest; body: Models.LibraryStateWrite }; response: Models.LibraryState }
}

export type ApiOperation = keyof ApiOperationMap
export interface ApiClient {
  request<Operation extends ApiOperation>(operation: Operation, request: ApiOperationMap[Operation]["request"]): Promise<ApiOperationMap[Operation]["response"]>
}

export const apiContract = {"getHealth":{"method":"GET","path":"/api/v1/health"},"getProfile":{"method":"GET","path":"/api/v1/profile"},"putProfile":{"method":"PUT","path":"/api/v1/profile"},"getRankingConfig":{"method":"GET","path":"/api/v1/ranking-config"},"putRankingConfig":{"method":"PUT","path":"/api/v1/ranking-config"},"listStarterSources":{"method":"GET","path":"/api/v1/starter-sources"},"listSources":{"method":"GET","path":"/api/v1/sources"},"createSource":{"method":"POST","path":"/api/v1/sources"},"getSource":{"method":"GET","path":"/api/v1/sources/{sourceId}"},"updateSource":{"method":"PATCH","path":"/api/v1/sources/{sourceId}"},"deleteSource":{"method":"DELETE","path":"/api/v1/sources/{sourceId}"},"putSourceCredential":{"method":"PUT","path":"/api/v1/sources/{sourceId}/credential"},"deleteSourceCredential":{"method":"DELETE","path":"/api/v1/sources/{sourceId}/credential"},"startRefresh":{"method":"POST","path":"/api/v1/refresh"},"getRefresh":{"method":"GET","path":"/api/v1/refresh/{refreshId}"},"getFeed":{"method":"GET","path":"/api/v1/feed"},"getFeedFilter":{"method":"GET","path":"/api/v1/feed-filter"},"putFeedFilter":{"method":"PUT","path":"/api/v1/feed-filter"},"getArticle":{"method":"GET","path":"/api/v1/articles/{articleId}"},"patchLibraryState":{"method":"PATCH","path":"/api/v1/articles/{articleId}/library-state"}} as const
