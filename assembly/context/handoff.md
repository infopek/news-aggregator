# News Aggregator Handoff

## Current lifecycle phase

Requirements/bootstrap and planning are merged. The implementation task backlog is ready for human review.

## Source of truth

- `project_workspace.json`
- `assembly/intake/project_intake.json`
- `assembly/requirements/REQUIREMENTS.md`
- `assembly/generated/project_spec.json`
- `assembly/generated/repo_plan.json`
- `assembly/generated/agent_prompts.json`
- `assembly/generated/slots_db.json`
- `assembly/generated/task_batch_index.json`
- `assembly/generated/task_backlog.json`
- `assembly/generated/collaboration_state.json`

## Next action

Review and merge the task-decomposition PR. After merge, open Dispatch from the merged repository state and claim the first dependency-ready task. Do not treat tasks as accepted or begin implementation before that approval boundary.

## Accepted direction

- Local-first, single-user Windows 11 PWA.
- Go backend, embedded Vue + TypeScript frontend, and SQLite.
- RSS/Atom, official APIs, and narrowly approved public-page scraping.
- Explainable weighted ranking plus lightweight local text similarity.
- Two parallel implementation agents and one mandatory reviewer agent.
