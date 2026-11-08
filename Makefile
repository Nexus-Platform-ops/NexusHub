# Nexus developer targets. Run `make bootstrap` once per machine.
SHELL := /bin/bash

.PHONY: bootstrap migrate run test cover gate test-flutter build zip

bootstrap:
	bash scripts/bootstrap.sh

migrate:
	bash scripts/migrate.sh

run:
	cd backend && go run ./cmd/api

test:
	cd backend && go test ./... -count=1

# gate runs the full suite with coverage, then locks the 80% floor
# (internal/covergate). Fails the build when coverage drops below the gate.
gate:
	cd backend && go test ./internal/... -count=1 -coverprofile=coverage.out
	cd backend && GO_COVERAGE_PROFILE=coverage.out go test ./internal/covergate -count=1 -v

cover: gate
	cd backend && go tool cover -func=coverage.out | tail -1

test-flutter:
	cd mobile && flutter test

build:
	cd backend && go build -o bin/api ./cmd/api
