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

## Post-MVP milestone: consumer UI/UX overhaul

Status: proposed. This milestone becomes authoritative only after its pull
request is reviewed and merged.

### Goal

Replace the current admin-form presentation with a coherent, quiet,
local-first news-reader experience. Preserve the accepted backend behavior,
privacy and safety boundaries, persistence, ranking correctness, accessibility,
and Windows support. This is an information-architecture and interaction
redesign, not a visual re-theme.

### Navigation and information architecture

- Keep the primary post-onboarding navigation focused on Ranked feed, Library,
  Sources, and Settings.
- Use first-run Setup only for approachable onboarding. Put later
  personalization changes in Settings and source administration on Sources.
- Organize Settings into Personalization, Ranking, and Privacy & local data.
- Use progressive disclosure so advanced and diagnostic controls do not
  dominate normal workflows.

### First-run setup

- Present setup as four clearly separated steps: Interests, optional Location,
  Sources, and Finish.
- Replace comma-separated interest entry with accessible topic chips/tags,
  examples, and custom-topic entry. Assign sensible canonical weights
  internally; do not expose “New interest weight” during onboarding.
- Present location as Country plus one human-facing “City or area” field. A
  user must be able to enter Hungary and Budapest without understanding an
  administrative-region schema.
- Keep location manual, optional, local, and free from browser/IP geolocation,
  inference, remote geocoding, or third-party services.
- Show starter sources as clear selectable rows/cards with useful names and
  descriptions.
- Keep age and gender under a collapsed Additional personalization disclosure.
  Explain that they are optional, user-entered, low-influence signals that only
  match explicit publisher/source audience metadata.
- Finish with one unambiguous primary action.

### Ranking settings

- Remove raw numeric ranking weights from the default interface.
- Offer semantic ranking styles using the existing canonical configuration,
  such as Balanced (recommended), More personalized, and More recent.
- If advanced controls remain, express influence semantically and explain that
  relative values do not need to add up to one. Preserve accepted caps,
  invariants, deterministic local ranking, and compatibility with persisted
  configurations.
- Never infer demographic relevance from article text. Missing, disabled, or
  unmatched optional signals remain neutral.

### Ranked feed and articles

- Give the page a clear header, concise purpose statement, and compact refresh
  action/status. Preserve detailed partial-failure outcomes behind an
  accessible disclosure.
- Group source, read, saved, search, date, and hidden filters into one coherent,
  responsive filter bar. Show Clear filters only when filters are active.
- Keep the feed-filter API and SQLite repository authoritative across process
  restart; browser storage must not become the source of truth.
- Render stories as visually distinct accessible cards with clear title,
  source, human-readable publication time, subordinate excerpt, primary reading
  action, and secondary read/save/hide actions.
- Preserve full-content permission enforcement and the truthful publisher-link
  fallback for metadata-only articles.
- Present ranking explanations as concise human-readable reasons driven by the
  backend reason data. Hide raw contribution floats by default; any diagnostic
  values belong in an advanced disclosure.

### Sources, library, and forms

- Make configured source identity, enabled state, permission mode, and refresh
  status easy to scan.
- Give source URLs, feed/API choices, content permissions, credentials, and
  approved-scraper policy plain-language examples and corrective validation
  messages without weakening enforcement.
- Apply the same card/surface, spacing, status, action, empty-state, and
  responsive patterns to Sources and Library.
- Every non-obvious field needs a useful label and, where necessary, an example
  or helper explaining the decision rather than repeating the label.

### Visual system and responsive behavior

- Define shared tokens and primitives for spacing, content widths, surfaces,
  typography, borders, radii, focus, buttons, fields, chips, banners, cards,
  disclosures, and empty states.
- Use deliberate hierarchy and grouping instead of a large white canvas,
  repeated horizontal rules, or equally weighted controls.
- Keep comfortable interaction targets, visible focus, sufficient contrast,
  semantic grouping, screen-reader-friendly errors/status, and keyboard access.
- Support desktop and approximately 390-pixel narrow viewports without
  horizontal overflow; filters and actions must stack or wrap intentionally.

### Non-negotiable boundaries

- Remain local-first, loopback-only, single-user, and same-origin.
- Add no accounts, sync, external AI/profile processing, automatic or IP
  geolocation, remote geocoding, analytics, tracking, demographic inference,
  background service, or new live-provider/scraping scope.
- Keep SQLite authoritative for profile, source, ranking, library, refresh, and
  filter state. Do not move authoritative personal data into browser storage.
- Keep credentials behind the accepted credential-store abstraction and never
  expose plaintext credential material.
- Preserve content permissions, scraping policy, deterministic ranking,
  explicit audience evidence, demographic caps, and backend-owned explanations.

### Verification and evidence

- Add component coverage for topic chips, simplified location, optional
  demographic disclosure, semantic ranking mapping, filter state, compact
  refresh status, article actions, helper/validation behavior, and explanations
  that hide raw scores by default while mapping production reason codes.
- Add backend/integration tests only where an API/domain compatibility change is
  justified, including migration/persistence and ranking invariants.
- Update the real Go + SQLite + Vue workflow to cover redesigned onboarding,
  source setup, mixed refresh, cards and explanations, library actions, filters,
  full process restart, interruption/recovery, keyboard use, accessibility,
  narrow viewport, loopback-only requests, and browser-storage privacy.
- Produce review evidence for onboarding, Settings, Ranked feed, expanded
  explanation, Sources, Library, and narrow viewport.
- Require an independent reviewer decision on the exact implementation revision;
  implementation work must not claim its own approval.

### Acceptance signals

- A first-time user can enter interests and Budapest, Hungary without knowing
  internal weight or administrative-region schemas.
- Optional age/gender controls are secondary, understandable, and neutral when
  missing or disabled.
- Normal workflows expose neither unexplained raw ranking weights nor long raw
  contribution floats.
- Refresh, filtering, story boundaries, explanations, reading, saving, hiding,
  restoring, source configuration, and error recovery are immediately legible.
- Desktop and narrow layouts look and behave like one deliberate application,
  retain keyboard/accessibility support, and pass the full existing verification
  suite without weakening any accepted privacy, security, persistence, ranking,
  permission, or Windows boundary.
