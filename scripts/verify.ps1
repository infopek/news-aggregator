$ErrorActionPreference = "Stop"

$unformatted = gofmt -l cmd internal
if ($unformatted) {
    throw "Go files require formatting: $unformatted"
}

go vet ./...
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
go test ./...
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }

.\scripts\migration-contract.ps1
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
npm --prefix web run lint
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
npm --prefix web run typecheck
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
npm --prefix web run test
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
npm --prefix web run build
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
node scripts/verify-pwa-output.mjs
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
go build ./cmd/news-aggregator
exit $LASTEXITCODE
