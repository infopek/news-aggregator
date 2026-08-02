$ErrorActionPreference = "Stop"

npm --prefix scripts exec -- redocly lint --config scripts/redocly.yaml api/openapi.yaml
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
node scripts/generate-api-types.mjs --check
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
node scripts/verify-openapi-format.mjs
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
git diff --exit-code -- web/src/api/generated
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
go test ./internal/domain/... ./internal/httpapi/... ./tests/integration/...
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
node scripts/verify-migrations.mjs
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
npm --prefix web run typecheck
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
npm --prefix web run test
if ($LASTEXITCODE -ne 0) { exit $LASTEXITCODE }
Write-Output "RESULT OK shared_contract=true network=unused credentials=absent"
