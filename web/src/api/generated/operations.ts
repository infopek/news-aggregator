// Generated from api/openapi.yaml (b6c730e56578ce87). Do not edit.
import type * as Models from './models'

export interface ApiOperationMap {
  getHealth: { method: "GET"; path: "/api/v1/health"; request: undefined; response: Models.Health }
  getProfile: { method: "GET"; path: "/api/v1/profile"; request: undefined; response: Models.Profile }
  putProfile: { method: "PUT"; path: "/api/v1/profile"; request: Models.ProfileWrite; response: Models.Profile }
  getRankingConfig: { method: "GET"; path: "/api/v1/ranking-config"; request: undefined; response: Models.RankingConfiguration }
  putRankingConfig: { method: "PUT"; path: "/api/v1/ranking-config"; request: Models.RankingConfigurationWrite; response: Models.RankingConfiguration }
  listStarterSources: { method: "GET"; path: "/api/v1/starter-sources"; request: undefined; response: Models.SourceList }
  listSources: { method: "GET"; path: "/api/v1/sources"; request: undefined; response: Models.SourceList }
  createSource: { method: "POST"; path: "/api/v1/sources"; request: Models.SourceWrite; response: Models.Source }
  getSource: { method: "GET"; path: "/api/v1/sources/{sourceId}"; request: Models.SourceIDRequest; response: Models.Source }
  updateSource: { method: "PATCH"; path: "/api/v1/sources/{sourceId}"; request: Models.SourceWrite; response: Models.Source }
  deleteSource: { method: "DELETE"; path: "/api/v1/sources/{sourceId}"; request: Models.SourceIDRequest; response: undefined }
  putSourceCredential: { method: "PUT"; path: "/api/v1/sources/{sourceId}/credential"; request: Models.CredentialWrite; response: Models.CredentialStatus }
  deleteSourceCredential: { method: "DELETE"; path: "/api/v1/sources/{sourceId}/credential"; request: Models.SourceIDRequest; response: Models.CredentialStatus }
  startRefresh: { method: "POST"; path: "/api/v1/refresh"; request: undefined; response: Models.RefreshRun }
  getRefresh: { method: "GET"; path: "/api/v1/refresh/{refreshId}"; request: Models.RefreshIDRequest; response: Models.RefreshRun }
  getFeed: { method: "GET"; path: "/api/v1/feed"; request: Models.FeedQuery; response: Models.FeedPage }
  getArticle: { method: "GET"; path: "/api/v1/articles/{articleId}"; request: Models.ArticleIDRequest; response: Models.ArticleDetail }
  patchLibraryState: { method: "PATCH"; path: "/api/v1/articles/{articleId}/library-state"; request: Models.LibraryStateWrite; response: Models.LibraryState }
}

export type ApiOperation = keyof ApiOperationMap
export interface ApiClient {
  request<Operation extends ApiOperation>(operation: Operation, request: ApiOperationMap[Operation]["request"]): Promise<ApiOperationMap[Operation]["response"]>
}

export const apiContract = {"getHealth":{"method":"GET","path":"/api/v1/health"},"getProfile":{"method":"GET","path":"/api/v1/profile"},"putProfile":{"method":"PUT","path":"/api/v1/profile"},"getRankingConfig":{"method":"GET","path":"/api/v1/ranking-config"},"putRankingConfig":{"method":"PUT","path":"/api/v1/ranking-config"},"listStarterSources":{"method":"GET","path":"/api/v1/starter-sources"},"listSources":{"method":"GET","path":"/api/v1/sources"},"createSource":{"method":"POST","path":"/api/v1/sources"},"getSource":{"method":"GET","path":"/api/v1/sources/{sourceId}"},"updateSource":{"method":"PATCH","path":"/api/v1/sources/{sourceId}"},"deleteSource":{"method":"DELETE","path":"/api/v1/sources/{sourceId}"},"putSourceCredential":{"method":"PUT","path":"/api/v1/sources/{sourceId}/credential"},"deleteSourceCredential":{"method":"DELETE","path":"/api/v1/sources/{sourceId}/credential"},"startRefresh":{"method":"POST","path":"/api/v1/refresh"},"getRefresh":{"method":"GET","path":"/api/v1/refresh/{refreshId}"},"getFeed":{"method":"GET","path":"/api/v1/feed"},"getArticle":{"method":"GET","path":"/api/v1/articles/{articleId}"},"patchLibraryState":{"method":"PATCH","path":"/api/v1/articles/{articleId}/library-state"}} as const
