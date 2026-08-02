#!/usr/bin/env bash
set -euo pipefail

npm --prefix web ci
node scripts/generate-api-types.mjs --check
npm --prefix web run lint
npm --prefix web run typecheck
npm --prefix web run test
npm --prefix web run build
go build ./cmd/news-aggregator
