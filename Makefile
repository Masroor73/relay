-include .env.local
export

.PHONY: dev test migrate seed

## dev: starts local Postgres and runs the ingestion service
dev:
	docker compose up -d
	@echo "Waiting for Postgres to be healthy..."
	@until docker compose ps postgres | grep -q "healthy"; do sleep 1; done
	cd apps/ingestion && go run ./cmd/server

## test: runs the Go test suite
test:
	cd apps/ingestion && go test ./...

## migrate: applies database migrations against DATABASE_URL
migrate:
	cd apps/ingestion && go run ./cmd/migrate

## seed: inserts sample events and orders for manual testing
seed:
	cd apps/ingestion && go run ./cmd/seed