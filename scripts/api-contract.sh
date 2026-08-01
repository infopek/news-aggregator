#!/usr/bin/env bash
set -euo pipefail

node scripts/generate-api-types.mjs --check
go test ./internal/httpapi/...
npm --prefix web run typecheck
npm --prefix web run test
