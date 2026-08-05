-include .env.local
export

.PHONY: dev test migrate

## dev: starts local Postgres and runs the ingestion service
dev:
	docker compose up -d
	cd apps/ingestion && go run ./cmd/server

## test: runs the Go test suite
test:
	cd apps/ingestion && go test ./...

## migrate: applies database migrations against DATABASE_URL
migrate:
	cd apps/ingestion && go run ./cmd/migrate