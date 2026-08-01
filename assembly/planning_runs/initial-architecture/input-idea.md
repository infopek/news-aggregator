# Accepted Requirements Input

Authoritative merged inputs:

- `project_workspace.json`
- `assembly/intake/project_intake.json`
- `assembly/requirements/REQUIREMENTS.md`
- `assembly/context/handoff.md`

Plan a local-first, single-user Windows 11 news aggregator. A Go executable serves an embedded Vue + TypeScript PWA, persists authoritative data in SQLite, reads optional API secrets from Windows Credential Manager, ingests RSS/Atom plus selected official APIs and narrowly approved scrapers, and ranks locally using explainable weighted signals plus lightweight text similarity. Two implementation lanes may run in parallel only after shared contracts are fixed; a third reviewer lane gates every implementation pull request. Final task decomposition is deferred.
