# Shared contract fixture coverage

Run `./scripts/contract.sh` on Bash or `.\scripts\contract.ps1` on PowerShell.
The command is offline and requires no provider credentials.

| Boundary | Positive fixtures | Negative/forward-compatibility case |
| --- | --- | --- |
| API response families | health, profile, ranking configuration, source/source list, credential status, refresh, feed, article detail, library state, API error | unknown ranking signal rejected; unknown reason code accepted |
| Optional values | profile with absent signals; metadata/full-content articles with nullable fields | metadata-only full content rejected |
| Refresh | partial success containing successful and failed source outcomes | deterministic error summary |
| Ranking | bounded weights and demographic caps | out-of-range weight rejected |
| Sources | feed configuration and approved scraper policy | secret adapter field, mismatched adapter, and unapproved enabled scraper rejected |
| Ingestion | RSS 2.0, Atom, fictional official API, approved scraper HTML | manifest enforces no network, no credentials, and explicit permission |
| Persistence | migration inventory, relationships, ranking, refresh, and content permissions | invalid rows rejected by SQLite constraints |

The API manifest is the frontend handoff: generated Vitest consumers compile
every response fixture against generated TypeScript models. The integration Go
tests are the backend handoff: they decode the same JSON plus ingestion shapes
and assert domain semantics. Implementers may copy fixtures into lane-local
tests, but must not alter shared fixtures to accommodate implementation behavior.

OpenAPI generation hashes the parsed document canonically. Formatting-only
changes retain the generated digest; semantic changes require regeneration.
