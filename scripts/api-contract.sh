#!/usr/bin/env bash
set -euo pipefail

npm --prefix scripts exec -- redocly lint --config scripts/redocly.yaml api/openapi.yaml
node scripts/generate-api-types.mjs --check
go test ./internal/httpapi/...
npm --prefix web run typecheck
npm --prefix web run test
