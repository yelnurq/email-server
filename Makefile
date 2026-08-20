# Mail Platform — development commands.
# Works on Linux/macOS and on Windows via `make` in Git Bash (if installed).
# Every target is a thin wrapper over cross-platform commands documented in
# README.md, so make itself is optional on Windows.

.PHONY: up down logs ps api worker migrate-up migrate-down migrate-status web fmt vet lint test build

## Infrastructure (Docker Compose)
up:
	docker compose up -d

down:
	docker compose down

logs:
	docker compose logs -f

ps:
	docker compose ps

## Backend
api:
	go run ./cmd/api

worker:
	go run ./cmd/worker

migrate-up:
	go run ./cmd/migrate up

migrate-down:
	go run ./cmd/migrate down

migrate-status:
	go run ./cmd/migrate status

## Frontend
web:
	cd apps/web && npm run dev

## Quality
fmt:
	gofmt -w .

vet:
	go vet ./...

lint: vet
	cd apps/web && npx tsc --noEmit && npm run lint

test:
	go test ./...

build:
	go build ./...
	cd apps/web && npm run build
