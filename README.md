# Mail Platform

Corporate mail platform: Webmail + Admin UI, built as a Go modular monolith
with a Next.js frontend. Currently in **Phase 1 — Windows local development**
(local Webmail MVP, dev domain `company.test`, no internet mail yet), designed
Linux-ready from day one.

- Architecture: [docs/architecture/ARCHITECTURE.md](docs/architecture/ARCHITECTURE.md)
- Decisions: [docs/adr/](docs/adr/)

## Stack

Go (chi, pgx) · Next.js (TypeScript, Tailwind) · PostgreSQL · Redis ·
NATS JetStream · MinIO — infrastructure runs as Linux containers.

## Prerequisites

Verified development environment (2026-08-20):

| Tool           | Version                  |
|----------------|--------------------------|
| Windows 11 Pro | 10.0.26200               |
| Git            | 2.55.0                   |
| Go             | 1.26.7                   |
| Node.js / npm  | 24.18.0 / 11.16.0        |
| Docker Desktop | WSL2 backend             |

## Quickstart

```bash
# 1. Configuration
cp .env.example .env            # PowerShell: Copy-Item .env.example .env

# 2. Infrastructure (PostgreSQL, Redis, NATS, MinIO as Linux containers)
docker compose up -d

# 3. Database migrations
go run ./cmd/migrate up

# 4. Backend API  →  http://localhost:8080
go run ./cmd/api

# 5. Worker (separate terminal)
go run ./cmd/worker

# 6. Frontend  →  http://localhost:3000
cd apps/web
npm install
npm run dev
```

All commands are identical in PowerShell, Git Bash and Linux shells.
`make up`, `make api`, `make web` etc. are available where `make` exists
(see [Makefile](Makefile)).

## Ports (local development)

Host ports for PostgreSQL/Redis are shifted because this dev machine runs
native PostgreSQL (5432) and Memurai (6379). Override in `.env`.

| Service        | Host port | In container |
|----------------|-----------|--------------|
| Go API         | 8080      | —            |
| Next.js        | 3000      | —            |
| PostgreSQL     | **5433**  | 5432         |
| Redis          | **6380**  | 6379         |
| NATS           | 4222      | 4222         |
| NATS monitor   | 8222      | 8222         |
| MinIO S3       | 9000      | 9000         |
| MinIO console  | 9001      | 9001         |

## Health

- `GET http://localhost:8080/health/live` — process is up.
- `GET http://localhost:8080/health/ready` — dependencies (postgres, redis,
  nats, minio) are reachable; `503` with per-check detail otherwise.
- The frontend start page (http://localhost:3000) visualizes these checks.

## Project layout

```text
apps/web/          Next.js app (/auth/*, /mail/*, /admin/* — single frontend)
cmd/api/           HTTP API entrypoint
cmd/worker/        background mail-pipeline processor
cmd/migrate/       migration runner (goose, embedded SQL)
internal/          Go modules (config, logging, server; domain modules follow)
migrations/        SQL migrations
docs/              architecture + ADRs
docker-compose.yml local infrastructure (Linux containers)
```

## Quality checks

```bash
gofmt -l .            # formatting (empty output = clean)
go vet ./...
go test ./...
go build ./...

cd apps/web
npx tsc --noEmit      # typecheck
npm run lint
npm run build
```

## Security notes

- Real `.env` is gitignored; never commit secrets.
- Logs never contain passwords, session secrets, keys or mail bodies.
