# Mail Platform — development commands.
# Works on Linux/macOS and on Windows via `make` in Git Bash (if installed).
# Every target is a thin wrapper over cross-platform commands documented in
# README.md, so make itself is optional on Windows.

.PHONY: up down logs ps api worker migrate-up migrate-down migrate-status web web-prod \
        migrate-mail migrate-mail-dry mail-dedupe sink-build sink-up fmt vet lint test build

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

# Production build served locally. Pages respond in tens of milliseconds
# instead of the seconds `next dev` spends compiling routes on demand — use
# this for demos and for judging real performance.
web-prod:
	cd apps/web && npm run build && npm start

## Mail core
# Backfill pre-V3 mailbox contents into the mail store (idempotent).
migrate-mail:
	go run ./cmd/migrate-mail

migrate-mail-dry:
	go run ./cmd/migrate-mail -dry-run

# Reconcile: remove duplicate Message-IDs left by an interrupted import.
mail-dedupe:
	go run ./cmd/migrate-mail -dedupe -dry-run

# Controlled SMTP delivery target for outbound tests (see docs/RUNBOOK.md).
sink-build:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o bin/smtpsink-linux ./cmd/smtpsink

sink-up: sink-build
	docker compose --profile test up -d smtpsink

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
