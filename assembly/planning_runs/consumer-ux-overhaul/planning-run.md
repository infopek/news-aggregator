# Consumer UX Overhaul Planning Run

## Run slug

`consumer-ux-overhaul`

## Goal

Update the accepted planning package for the merged post-MVP UX requirements.
Define screen architecture, ownership, compatibility boundaries, verification,
and reviewer expectations without implementing product code or generating the
task backlog.

## Required outputs

- `project_spec.json`
- `repo_plan.json`
- `agent_prompts.json`
- `slots_db.json`

## Planning decisions

- Keep the existing Go/SQLite/OpenAPI/Vue architecture and local-only boundary.
- Treat the redesign primarily as frontend information architecture and shared
  component work; require a reviewed contract handoff before any justified API
  or domain compatibility change.
- Preserve existing persisted profiles, ranking configurations, sources,
  filters, library state, and permissions.
- Map semantic ranking controls deterministically onto the existing accepted
  bounded configuration; do not create a second ranking model in the frontend.
- Require rendered desktop and narrow evidence in addition to automated tests.
- Defer stable task identifiers and assignments to the Task Splitter after this
  planning PR is reviewed and merged.

## Prohibited outputs

- Product implementation
- Task batches or canonical task backlog
- Task claims, assignments, or completion records
- Reviewer approval of this planning run
