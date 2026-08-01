# Development commands

Prerequisites: Go 1.24 or newer, Node.js 22 or newer, and npm 10 or newer. The build uses pure Go at this stage and does not require a C compiler.

Run from the repository root.

## Bash

```bash
npm --prefix web ci
./scripts/verify.sh
```

## PowerShell

```powershell
npm --prefix web ci
.\scripts\verify.ps1
```

Both scripts format-check Go, run Go static analysis and tests, lint/type-check/test/build the frontend, and build the Go command after Vite writes assets to `internal/webassets/dist`.

Generated frontend output is ignored by Git. The checked-in `internal/webassets/dist/.gitkeep` makes a clean-checkout Go build valid before the first frontend build.

`web/go.mod` is an intentionally empty nested Go module boundary. It prevents root `go test ./...` and `go vet ./...` commands from traversing Go source files that may exist inside npm dependencies.
