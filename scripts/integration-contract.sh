#!/usr/bin/env bash
set -euo pipefail

result_file="$(mktemp)"
trap 'rm -f "$result_file"' EXIT
go test -json ./tests/integration | tee "$result_file"
node scripts/assert-go-tests-ran.mjs "$result_file"
