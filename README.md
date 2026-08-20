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

On first start with an empty database the API bootstraps the initial admin
from `BOOTSTRAP_ADMIN_EMAIL` / `BOOTSTRAP_ADMIN_PASSWORD` (see `.env`).
Sign in at http://localhost:3000, then: Admin → add a development domain
(e.g. `company.test`) → create users with mailboxes → users exchange mail.

## End-to-end tests

With infra + API + worker running:

```bash
bash scripts/e2e.sh
```

19 checks: full send/receive/reply journey, Sent copies, mailbox and
cross-tenant isolation, RBAC, session revocation.

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

## API (v1)

All endpoints under `/api/v1`, unified error envelope
`{"error":{"code","message","request_id"}}`.

| Area | Endpoints |
|---|---|
| Auth | `POST /auth/login`, `POST /auth/logout`, `GET /me` |
| Admin | `GET/POST /organizations`, `GET/POST /domains`, `GET/POST /users`, `GET/POST /mailboxes` |
| Webmail | `GET /mail/summary`, `GET /mail/messages?folder=&q=&limit=&offset=`, `GET/PATCH/DELETE /mail/messages/{id}`, `POST /mail/send` |
| Drafts | `POST /mail/drafts`, `PUT /mail/drafts/{id}`, `POST /mail/drafts/{id}/send` |

Sessions: HttpOnly cookie for the browser, or `Authorization: Bearer <token>`
from the login response.

## Quality checks

```bash
gofmt -l .            # formatting (empty output = clean)
go vet ./...
go test ./...         # on Windows set GOTMPDIR=./bin/gotmp first
                      # (Application Control blocks test exes in %TEMP%)
go build ./...

cd apps/web
npx tsc --noEmit      # typecheck
npm run lint
npm run build
```

## Security notes

- Real `.env` is gitignored; never commit secrets.
- Logs never contain passwords, session secrets, keys or mail bodies.
