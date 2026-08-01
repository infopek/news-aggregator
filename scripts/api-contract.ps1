$ErrorActionPreference = "Stop"

npm --prefix scripts exec -- redocly lint --config scripts/redocly.yaml api/openapi.yaml
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
node scripts/generate-api-types.mjs --check
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
go test ./internal/httpapi/...
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
npm --prefix web run typecheck
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
npm --prefix web run test
exit $LASTEXITCODE
