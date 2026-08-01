# News Aggregator Handoff

## Current lifecycle phase

Requirements/bootstrap is merged. The initial architecture planning package is ready for human review.

## Source of truth

- `project_workspace.json`
- `assembly/intake/project_intake.json`
- `assembly/requirements/REQUIREMENTS.md`
- `assembly/generated/project_spec.json`
- `assembly/generated/repo_plan.json`
- `assembly/generated/agent_prompts.json`
- `assembly/generated/slots_db.json`

## Next action

Review and merge the planning PR. After merge, start a fresh Task Splitter context from the merged repository files. Do not create the executable task backlog or implementation work before that approval boundary.

## Accepted direction

- Local-first, single-user Windows 11 PWA.
- Go backend, embedded Vue + TypeScript frontend, and SQLite.
- RSS/Atom, official APIs, and narrowly approved public-page scraping.
- Explainable weighted ranking plus lightweight local text similarity.
- Two parallel implementation agents and one mandatory reviewer agent.
