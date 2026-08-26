# News Aggregator Handoff

## Current lifecycle phase

The original MVP lifecycle is complete and accepted. Post-MVP consumer UI/UX
overhaul requirements are merged through PR #53 at
`d53d71655a20eab8edc2b10d0f3a8b9c48c3b094`.

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

1. Fetch `main` and confirm it includes PR #53 merge
   `d53d71655a20eab8edc2b10d0f3a8b9c48c3b094`.
2. Create a separate post-MVP planning PR by
   updating the planning package and planning-run trace. Do not create executable
   tasks in that planning PR.
3. After the planning PR is reviewed and merged, create the separate task-batch
   and canonical-backlog PR.
4. Begin UI implementation only after that task-decomposition PR is merged and
   a dependency-ready task is claimed.

The detailed accepted/proposed UX direction is recorded in
`assembly/requirements/REQUIREMENTS.md` on PR #53, commit
`559cb12fa1c16f095d089cb4eba2d4027297142d`.

## Accepted direction

- Local-first, single-user Windows 11 PWA.
- Go backend, embedded Vue + TypeScript frontend, and SQLite.
- RSS/Atom, official APIs, and narrowly approved public-page scraping.
- Explainable weighted ranking plus lightweight local text similarity.
- Two parallel implementation agents and one mandatory reviewer agent.

## Recently completed

- PR #50 corrected final MVP acceptance blockers and merged as
  `49de411ac1c2b924e126e70dfc48253f284a025f`.
- PR #51 durably closed VERIFY-004 and all reconstructed lifecycle records;
  every canonical MVP task is `done`.
- PR #52 added the one-command Windows build wrapper and merged as
  `dab194672168bfd70d6d7bab0c346431e88ba1ef`.
- Normal Windows build command:
  `./packaging/windows/build.ps1` from PowerShell, producing
  `build/news-aggregator.exe`.

## Working-tree expectations

- Resume from a clean checkout of `main`.
- Do not continue from the old requirements branch; PR #53 is already merged.
- Keep build-wrapper work separate from the UX-overhaul planning and
  implementation lifecycle.
