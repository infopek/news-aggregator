#!/usr/bin/env bash
set -euo pipefail

npm --prefix scripts exec -- redocly lint --config scripts/redocly.yaml api/openapi.yaml
node scripts/generate-api-types.mjs --check
node scripts/verify-openapi-format.mjs
git diff --exit-code -- web/src/api/generated
go test ./internal/domain/... ./internal/httpapi/... ./tests/integration/...
node scripts/verify-migrations.mjs
npm --prefix web run typecheck
npm --prefix web run test
echo 'RESULT OK shared_contract=true network=unused credentials=absent'
