#!/usr/bin/env bash
set -euo pipefail

test -z "$(gofmt -l cmd internal)"
go vet ./...
go test ./...
./scripts/migration-contract.sh
npm --prefix web run lint
npm --prefix web run typecheck
npm --prefix web run test
npm --prefix web run build
go build ./cmd/news-aggregator
