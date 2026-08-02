# SQLite migration contract

Files named `NNNN_description.sql` are immutable, forward-only migrations and
are applied in lexical order. The migration runner owns this protocol:

1. Enable `PRAGMA foreign_keys = ON` for every connection.
2. Create `schema_migrations(version INTEGER PRIMARY KEY, name TEXT NOT NULL,
   applied_at_ms INTEGER NOT NULL)` before inspecting versions.
3. Reject a database whose recorded version is newer than the highest bundled
   migration.
4. For each pending file, begin an immediate transaction, execute the complete
   file, insert its numeric version and filename into `schema_migrations`, then
   commit. Roll back the whole migration on any error.
5. If no migration is pending, return the current version without changing the
   database.

Times are UTC Unix milliseconds. Booleans are integers constrained to `0` or
`1`. Optional user-supplied signals are represented by separate present and
enabled flags; the schema does not contain inferred demographics. Credential
references are opaque identifiers for the platform credential store. Secret
values must never be passed to or persisted by SQLite.

Article full content may only be stored when `content_permission` explicitly
allows it. Raw fetched payloads and disposable browser state are not stored.
Individual ranking scores and contributions are constrained to the unit range;
the ranking service must enforce cross-row contribution totals atomically before
persistence because SQLite `CHECK` constraints cannot inspect sibling rows.
Future schema changes must add a new numbered migration rather than modifying a
migration that has shipped.

Configured source deletion is a tombstone operation: `deleted_at_ms` removes a
source from normal configuration reads while retaining its row for article and
library provenance. Saving the same source identifier restores it. Credential
references are cleared and the source is disabled when tombstoned; credential
values are never stored here.
