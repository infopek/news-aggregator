.PHONY: format lint test web-install web-build build verify

format:
	gofmt -w cmd internal

lint:
	go vet ./...
	npm --prefix web run lint

test:
	go test ./...
	npm --prefix web run test

web-install:
	npm --prefix web ci

web-build:
	npm --prefix web run build

build: web-build
	go build ./cmd/news-aggregator

verify: lint test build
