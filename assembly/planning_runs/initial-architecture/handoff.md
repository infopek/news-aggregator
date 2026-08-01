# Initial Architecture Planning Handoff

## Source of truth

- `project_workspace.json`
- `assembly/intake/project_intake.json`
- `assembly/requirements/REQUIREMENTS.md`
- `assembly/generated/project_spec.json`
- `assembly/generated/repo_plan.json`
- `assembly/generated/agent_prompts.json`
- `assembly/generated/slots_db.json`

## Architecture summary

Use one monorepo and one distributable Go process. Keep dependency-free domain and application ports inside the Go codebase, place infrastructure behind adapters, expose a versioned loopback-only HTTP API, and generate frontend bindings from the canonical OpenAPI contract. Embed the compiled Vue PWA into the executable. SQLite is authoritative; Windows Credential Manager owns secret values.

## Parallelization gate

Shared domain ports, OpenAPI shapes, migration ownership, error semantics, and deterministic fixtures must be accepted before the backend/core and frontend implementation lanes run concurrently. Each implementation PR requires an explicit approval from the independent reviewer lane.

## Task Splitter guidance

Decompose shared contracts and repository scaffolding before parallel feature work. Then split work along the backend/core and frontend ownership boundaries in `repo_plan.json`. Keep integration and Windows smoke proof explicit, and do not assign overlapping production-file ownership to concurrent tasks.

## Non-blocking questions

- Choose the first starter feeds and regions before final onboarding acceptance.
- Choose the first official API and approved scraper adapters before provider-specific acceptance.
- Treat ranking weights as configurable defaults and tune them from local usage evidence.

## Next action

Review and merge the planning PR. Then start a fresh Task Splitter context from the merged artifacts.
