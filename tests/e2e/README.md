# VERIFY-002 daily-use acceptance

This suite drives the built Vue application through a separate real Go process and a temporary SQLite database. The Go acceptance executable uses the production repositories, services, adapters, HTTP handlers, and embedded frontend. Its only test seam maps fictional public fixture hosts to an in-process HTTP server through the production fetcher's resolver/dialer interfaces; production SSRF behavior is unchanged and no backend API is mocked.

Run from the repository root:

```sh
node tests/e2e/verify-daily-workflow.mjs
```

Prerequisites are the repository's pinned Node dependencies (`npm --prefix web ci`), Go, and Chrome/Chromium/Edge. Set `NEWS_AGGREGATOR_BROWSER` when the browser is not installed in a standard location.

The run starts from an empty user-data directory, performs setup and a mixed-outcome refresh, verifies ranked reading and library actions, stops and relaunches the Go process against the same database, and audits accessibility, keyboard navigation, narrow layout, browser storage, and browser outbound requests. Replay artifacts are written to `tests/e2e/evidence/`; temporary database and binaries are deleted.

The fixture hosts are fictional and permitted:

- `metadata.fixture.test` supplies metadata-only RSS.
- `full.fixture.test` supplies explicitly permitted full-content RSS.
- `failure.fixture.test` returns a deterministic 503 to prove partial refresh handling.
- `publisher.fixture.test` is retained only as the canonical publisher link and is never contacted by the browser proof.
