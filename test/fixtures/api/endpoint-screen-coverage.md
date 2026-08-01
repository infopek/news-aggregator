# Endpoint-to-screen coverage

This matrix is authoritative for the MVP API surface. “Host” denotes process
readiness rather than a user-facing screen. No postponed feature is exposed.

| Operation | Method and path | MVP consumer | Application boundary |
|---|---|---|---|
| `getHealth` | `GET /api/v1/health` | Host and global status | HTTP host readiness |
| `getProfile`, `putProfile` | `GET/PUT /api/v1/profile` | First-run; Profile and ranking settings | `application.ProfileService` |
| `getRankingConfig`, `putRankingConfig` | `GET/PUT /api/v1/ranking-config` | First-run; Profile and ranking settings | `application.ProfileService` |
| `listStarterSources` | `GET /api/v1/starter-sources` | First-run; Sources and refresh | `httpapi.SourceQueries` pending CONTRACT-006 implementation |
| `listSources`, `createSource` | `GET/POST /api/v1/sources` | First-run; Sources and refresh | `application.SourceService` |
| `getSource`, `updateSource`, `deleteSource` | `GET/PATCH/DELETE /api/v1/sources/{sourceId}` | Sources and refresh | read via `httpapi.SourceQueries`; writes via `application.SourceService` |
| `putSourceCredential`, `deleteSourceCredential` | `PUT/DELETE /api/v1/sources/{sourceId}/credential` | Sources and refresh | `application.SourceService` |
| `startRefresh`, `getRefresh` | `POST /api/v1/refresh`; `GET /api/v1/refresh/{refreshId}` | Ranked feed; Sources and refresh | `application.RefreshService` |
| `getFeed` | `GET /api/v1/feed` | Ranked feed; Personal library | `application.FeedService` |
| `getArticle` | `GET /api/v1/articles/{articleId}` | Article reader | `application.FeedService` |
| `patchLibraryState` | `PATCH /api/v1/articles/{articleId}/library-state` | Ranked feed; Article reader; Personal library | `application.FeedService` |

All resource identifiers are carried in the generated request `path` member;
feed filters are carried in `query`; JSON mutation payloads are carried in
`body`. Credential operations intentionally have no read counterpart.
