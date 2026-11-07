# Nexus developer targets. Run `make bootstrap` once per machine.
SHELL := /bin/bash

.PHONY: bootstrap migrate run test cover test-flutter build zip

bootstrap:
	bash scripts/bootstrap.sh

migrate:
	bash scripts/migrate.sh

run:
	cd backend && go run ./cmd/api

test:
	cd backend && go test ./... -count=1

cover:
	cd backend && go test ./internal/... -count=1 -coverprofile=coverage.out && go tool cover -func=coverage.out | tail -1

test-flutter:
	cd mobile && flutter test

build:
	cd backend && go build -o bin/api ./cmd/api
