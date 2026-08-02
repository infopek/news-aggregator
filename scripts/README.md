# Development commands

Prerequisites: Go 1.24 or newer, Node.js 22.12 or newer, and npm 10 or newer. The build uses pure Go at this stage and does not require a C compiler.

Run from the repository root.

## Bash

```bash
npm --prefix web ci
npm --prefix scripts ci
./scripts/verify.sh
```

## PowerShell

```powershell
npm --prefix web ci
npm --prefix scripts ci
.\scripts\verify.ps1
```

Both scripts format-check Go, run Go static analysis and tests, lint/type-check/test/build the frontend, and build the Go command after Vite writes assets to `internal/webassets/dist`.

## SQLite migration contract

Run `./scripts/migration-contract.sh` or `.\scripts\migration-contract.ps1` to
apply every migration to a temporary SQLite database and verify repeat runs,
schema inventory, relationship/deduplication constraints, interrupted rollback,
unsupported-newer-version rejection, and forbidden schema terms. The harness
uses the Node.js 22 built-in SQLite binding only for contract verification; the
application runtime remains pure Go and does not gain a SQLite dependency here.

## API contract

After editing `api/openapi.yaml`, regenerate the checked-in TypeScript bindings:

```bash
node scripts/generate-api-types.mjs
```

CI-style OpenAPI 3.1 validation, drift, fixture, Go boundary, and frontend checks are available as
`./scripts/api-contract.sh` or `.\scripts\api-contract.ps1`. Generation uses
only Node.js built-ins; Redocly CLI is pinned under `scripts/package-lock.json`
for standards validation. The OpenAPI file intentionally remains JSON-compatible
YAML so generation remains deterministic.

Generated frontend output is ignored by Git. The checked-in `internal/webassets/dist/.gitkeep` makes a clean-checkout Go build valid before the first frontend build.

## Shared cross-layer contract gate

Run exactly one command before backend/frontend lane handoff:

```bash
./scripts/contract.sh
```

On Windows PowerShell use `.\scripts\contract.ps1`. It validates standards-valid
OpenAPI, deterministic generated bindings and formatting invariance, typed
frontend fixture consumers, Go/domain fixture semantics, SQLite migrations and
constraints, and offline ingestion shapes. See
`test/fixtures/CONTRACT_COVERAGE.md` for the fixture inventory and handoff rules.

`web/go.mod` is an intentionally empty nested Go module boundary. It prevents root `go test ./...` and `go vet ./...` commands from traversing Go source files that may exist inside npm dependencies.
