# Typed server state

`queryKeys` is the sole key factory. Collection prefixes (`sources`, `feed`) may be invalidated without touching unrelated profile, ranking, article, or refresh entries. Feed keys normalize source IDs so equivalent filters share a cache entry.

Mutations reconcile returned authoritative objects, then invalidate only their detail/collection dependencies. Failure also invalidates those dependencies because a response can be lost after the server commits. Credential mutations cache only the returned configured status indirectly through source refetches; request secrets are never cached, logged, serialized, or readable.

`ServerStateClient.query` cancels and supersedes an older request for the same key. `RefreshPoller` is disposable and stops for every terminal status, missing results, timeout, external cancellation, or an obsolete poll generation.
