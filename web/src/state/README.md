# Typed server state

`queryKeys` is the sole key factory. Collection prefixes (`sources`, `feed`) may be invalidated without touching unrelated profile, ranking, article, or refresh entries. Feed keys normalize source IDs so equivalent filters share a cache entry.

Mutations reconcile returned authoritative objects, then invalidate only their dependencies. Failure also invalidates those dependencies because a response can be lost after the server commits.

| Mutation | Invalidates | Authoritative response retained |
| --- | --- | --- |
| profile | profile, all feeds | profile |
| ranking | ranking, all feeds | ranking |
| source create | source list, all feeds | created source detail |
| source update | source detail, source list, all feeds | updated source detail |
| source delete | deleted source, source list, all feeds | no response |
| credential write/delete | source detail, source list | returned status in mutation result only |
| article library | article detail, article-library state, all feeds | article-library state |

Credential request values are never cached, logged, serialized, or readable. Credential status remains available only as the mutation result; no credential query key or read path exists.

`ServerStateClient.query` cancels and supersedes an older request for the same key. `RefreshPoller` is disposable and stops for every terminal status, missing results, timeout, external cancellation, or an obsolete poll generation.
