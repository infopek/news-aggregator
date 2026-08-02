$ErrorActionPreference = "Stop"

npm --prefix web ci
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
node scripts/generate-api-types.mjs --check
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
