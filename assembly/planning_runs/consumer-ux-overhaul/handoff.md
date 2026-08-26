# Consumer UX Overhaul Planning Handoff

## Architecture

Retain the existing embedded Vue SPA, loopback Go host, OpenAPI boundary, and
SQLite authority. The overhaul adds no service or runtime dependency. Frontend
semantic controls adapt to existing API representations wherever cleanly
possible. Any contract change must be separately justified by a durable UX need,
remain backward compatible or migrated, regenerate bindings, and land before
dependent frontend work.

## Frontend structure

Plan shared foundations before screen rewrites:

1. Design tokens and accessible primitives for layout widths, spacing,
   typography, surfaces, controls, chips, banners, cards, disclosures, actions,
   empty states, and visible focus.
2. Four-step onboarding plus reorganized Settings with shared personalization
   controls and deterministic semantic ranking mappings.
3. Ranked-feed header, compact refresh state, responsive filter bar, article
   cards, reader actions, and concise backend-driven explanations.
4. Sources and Library alignment using the same components, copy standards,
   state hierarchy, responsive behavior, and accessibility rules.
5. Real-process verification and visual evidence across the complete workflow.

Avoid concurrent tasks that edit the same global style/component foundation.
Screen tasks may proceed in parallel only after shared primitives and semantic
control contracts are accepted.

## Compatibility and authority

- Preserve existing saved profile/ranking values and map them predictably into
  semantic controls, including a documented custom/advanced state when a value
  does not exactly match a preset.
- Keep feed-filter API/SQLite state authoritative across process restarts.
- Do not place interests, location, sources, credentials, ranking settings,
  article content, or library state in browser storage.
- Keep explanations sourced from backend contributions/reason codes; raw scores
  are advanced-only if retained.
- Maintain explicit demographic evidence, disabled neutrality, caps, content
  permission, credential, scraper-policy, and loopback guarantees.

## Verification

Every visible implementation task requires component tests and rendered
desktop/narrow evidence. The assembled workflow must run against the real Go
process and SQLite database, cover restart and API interruption, exercise
keyboard interactions, run accessibility checks, and audit browser storage and
outbound requests.

## Task Splitter guidance

Create small dependency-ordered tasks rather than one broad frontend rewrite.
Separate shared design foundations, onboarding/settings, feed/reader/refresh,
Sources/Library consistency, any genuinely required compatibility contract, and
final real-process verification. Give the independent reviewer a final holistic
visual/behavior acceptance task after all screen work is merged.

## Next action

Review and merge the planning PR. Then start a fresh Task Splitter context from
the merged artifacts and create a separate task-decomposition PR.
