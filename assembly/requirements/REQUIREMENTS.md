# News Aggregator Requirements

Status: ready for review. These requirements become authoritative only after this pull request is merged.

## Goal

Provide a private, local-first news aggregator that collects selected sources and ranks the most relevant articles first for one user on Windows 11.

## Target users

- The repository owner as the sole local user.

## MVP

### Must have

- Installable PWA with Windows 11 as the first supported platform.
- One manually launched Windows executable starts the local server and opens the app.
- Small editable starter source catalog plus manual source configuration.
- RSS/Atom ingestion, supported official news APIs, and explicitly approved public-page scrapers.
- Transparent weighted ranking enhanced with lightweight local TF-IDF/BM25-style text similarity.
- Explicit interests, source choices, recency, and reading actions are primary ranking signals.
- Age, gender, location, and city are optional, disableable, explicitly entered, and low-weight.
- Every ranked article includes a concise ranking explanation.
- Full articles appear in-app only when feeds or source permissions provide them; other articles link out.
- Persistent read/unread, saved, hidden/dismissed, and basic filter state.
- SQLite stores authoritative local application data.
- Optional API keys are stored in Windows Credential Manager through a replaceable interface.

### Explicitly postponed

- Accounts, synchronization, and multi-user support.
- Mobile-first and non-Windows packaging.
- Bundled embedding models and separate local model services.
- Automatic source discovery.
- Folders, annotations, exports, and cross-article research.
- Background-service installation and ingestion while the executable is closed.
- Broad or restriction-bypassing scraping.

## Platforms and stack constraints

- Windows 11, installable PWA served on localhost.
- Go backend serving an embedded Vue + TypeScript SPA.
- SQLite persistence.
- Lightweight local text similarity for the MVP, behind a replaceable ranking interface.
- The MVP sends no profile or article data to external AI services.

## Working style and proof

- Prefer small tasks with explicit contracts before parallel implementation.
- Two implementation agents may work concurrently.
- A third independent reviewer agent must approve every implementation PR and may block merging.
- Proof includes relevant unit/component tests, Go/frontend integration checks, critical-path browser tests where applicable, lint/build checks, and a Windows build smoke check.

## Safety boundaries and non-goals

- Scrape only public pages whose terms and robots guidance permit automated access.
- Never bypass paywalls, authentication, access controls, rate limits, or publisher restrictions.
- Do not display extracted full text without feed or source permission.
- Keep profile data and semantic processing local.
- Do not store API credentials in plaintext.
- Do not infer sensitive demographic fields.

## Assumptions

- The sole user controls the Windows machine and local data.
- Manual refresh and refresh-while-running are sufficient for the MVP.
- Starter sources are editable without an application release.
- SQLite is authoritative; browser storage is only for disposable UI preferences or caches.
- Coarse manually selected location is sufficient for regional relevance.

## Open questions

- Which feeds and regions should populate the initial starter catalog?
- Which official news APIs and approved scraper targets should be implemented first?
- What initial configurable weights should the ranking signals use?

## Acceptance signals

- The app builds into a manually launched Windows executable and opens the local PWA.
- The user can configure a profile and sources, refresh news, and receive a ranked feed.
- Ranking reasons are visible and reflect enabled, user-controlled signals.
- Permitted content is readable in-app and other content opens at the publisher.
- Read, saved, hidden, filter, profile, and source state survive restarts.
- API credentials use Windows Credential Manager.
- Layered automated checks and the Windows build smoke check pass.
- The reviewer agent approves the implementation PR.
