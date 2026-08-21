# Mail Platform

Corporate mail platform: Webmail + Admin UI, built as a Go modular monolith
with a Next.js frontend. Currently in **Phase 1 — Windows local development**
(local Webmail MVP, dev domain `company.test`, no internet mail yet), designed
Linux-ready from day one.

- Architecture: [docs/architecture/ARCHITECTURE.md](docs/architecture/ARCHITECTURE.md)
- Decisions: [docs/adr/](docs/adr/)

## Stack

Go (chi, pgx) · Next.js (TypeScript, Tailwind) · PostgreSQL · Redis ·
NATS JetStream · MinIO · **Stalwart Mail Server** (mail core: SMTP
submission, IMAP, JMAP) — infrastructure runs as Linux containers.

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

# 2. Infrastructure (PostgreSQL, Redis, NATS, MinIO, Stalwart as Linux containers)
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

For local UI testing, `/login` includes two quick-login profiles configured by
`NEXT_PUBLIC_DEMO_ADMIN_*` and `NEXT_PUBLIC_DEMO_USER_*`. These credentials are
development-only and must not be enabled in a production build.

## End-to-end tests

With infra + API + worker running:

```bash
bash scripts/e2e.sh              # Linux/macOS/Git Bash
powershell -File scripts/e2e.ps1 # Windows without bash: runs the same suite
                                 # in a docker:cli container against the host
```

46 checks covering all five user journeys: send/receive/reply, aliases,
groups, attachments (incl. cross-tenant download denial), Email API with
idempotency, webhooks with independent HMAC verification, spam/quarantine
lifecycle, tenant isolation, RBAC and session revocation — plus self-mail
(A → A in both Sent and Inbox), Message Trace, infrastructure health and
mail-core provisioning lifecycle.

All commands are identical in PowerShell, Git Bash and Linux shells.
`make up`, `make api`, `make web` etc. are available where `make` exists
(see [Makefile](Makefile)).

## Ports (local development)

Host ports for PostgreSQL/Redis are shifted because this dev machine runs
native PostgreSQL (5432) and Memurai (6379). Override in `.env`.

| Service           | Host port | In container |
|-------------------|-----------|--------------|
| Go API            | 8080      | —            |
| Next.js           | 3000      | —            |
| PostgreSQL        | **5433**  | 5432         |
| Redis             | **6380**  | 6379         |
| NATS              | 4222      | 4222         |
| NATS monitor      | 8222      | 8222         |
| MinIO S3          | 9000      | 9000         |
| MinIO console     | 9001      | 9001         |
| Stalwart SMTP     | **2525**  | 25           |
| Stalwart submission | **1587** | 587         |
| Stalwart submissions (TLS) | **1465** | 465  |
| Stalwart IMAP     | **1143**  | 143          |
| Stalwart IMAPS    | **1993**  | 993          |
| Stalwart HTTP (mgmt + JMAP) | **8180** | 8080 |

## Mail core (Stalwart)

Stalwart (pinned `v0.13.4`; config in
[deploy/stalwart/config.toml](deploy/stalwart/config.toml), data in the
`stalwartdata` volume) is the protocol engine behind the platform per
[ADR-001](docs/adr/ADR-001-mail-core.md). The Go API is the control plane and
talks to it only through the `internal/mailcore.Provider` abstraction
(implementation: `mailcore.Stalwart`, management REST API, admin credentials
stay in the backend). Set `MAIL_CORE_PROVIDER=none` to develop without the
container — provisioning is then recorded as `skipped`.

What is integrated:

- **Provisioning lifecycle** — creating a domain/mailbox (or retrying via
  `POST /domains/{id}/provision`, `POST /mailboxes/{id}/provision`) pushes a
  principal into Stalwart and tracks `pending → provisioning → active |
  failed` per row; failures keep the error and are retryable, and are audited.
- **SMTP credentials = app passwords** — `POST /smtp-credentials` registers
  the secret as a labelled Stalwart app password, so mail clients log in with
  the mailbox address + that password (587 STARTTLS / 465, IMAP 143/993,
  JMAP). Revoking removes it from Stalwart first; disabling a mailbox clears
  all its protocol passwords.
- **Health** — Stalwart (and the delivery worker via DB heartbeats) appear in
  `GET /api/v1/system/infrastructure` (cached backend checks), Admin →
  Infrastructure and the dashboard.

Smoke-test the whole chain (SMTP submit → Stalwart delivery → JMAP fetch):

```bash
go run ./cmd/mailcheck -smtp localhost:1587 -http http://localhost:8180 \
  -from admin@company.test -from-pass <smtp credential password> \
  -to user1@company.test -to-pass <recipient credential password>
```

**Webmail reads and writes the same store** as IMAP/JMAP clients
([ADR-003](docs/adr/ADR-003-unified-mail-storage.md)): mailbox contents,
folders and flags live in Stalwart, while PostgreSQL keeps the control plane
(users, domains, policies, audit) and the message trace. A message sent from
webmail carries one identity across webmail, JMAP and IMAP, and a star set
in one client shows up in the others.

Operational procedures — mail not arriving, deferred outbound, provisioning
failures, mail core down — are in [docs/RUNBOOK.md](docs/RUNBOOK.md).

Legacy mail from before the unification is backfilled with
`make migrate-mail` (idempotent; `make migrate-mail-dry` previews).

## Health

- `GET http://localhost:8080/health/live` — process is up.
- `GET http://localhost:8080/health/ready` — dependencies (postgres, redis,
  nats, minio) are reachable; `503` with per-check detail otherwise.
- `GET /api/v1/system/infrastructure` (admin) — cached per-component report:
  postgres, redis, nats, minio, stalwart, delivery worker (latency, version,
  last heartbeat).
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
| Auth | `POST /auth/login`, `POST /auth/logout`, `GET /me`, `POST /me/password` |
| Admin | `GET/POST /organizations`, `/domains`, `/users`, `/mailboxes`; aliases, groups (+`/{id}/members`), `/api-keys`, `/smtp-credentials`, `/webhooks` (+deliveries/retry) |
| Security | `GET /quarantine`, `POST /quarantine/{id}/release|delete`, `GET/POST/DELETE /security/blocks`, `GET /audit` |
| Webmail | `GET /mail/summary`, `GET /mail/messages?folder=&q=&limit=&offset=`, `GET/PATCH/DELETE /mail/messages/{id}`, `GET /mail/messages/{id}/events` (sender delivery timeline), `GET /mail/client-config`, `POST /mail/send`, attachments upload/download |
| Operations | `GET /admin/messages?q=&status=` + `GET /admin/messages/{id}/trace` (Message Trace), `GET /system/infrastructure`, `POST /domains/{id}/provision`, `PATCH /mailboxes/{id}`, `POST /mailboxes/{id}/provision` |
| Drafts | `POST /mail/drafts`, `PUT /mail/drafts/{id}`, `POST /mail/drafts/{id}/send` |
| Email API | `POST /emails` (+`Idempotency-Key`), `POST /emails/batch`, `GET /emails/{id}`, `GET /emails/{id}/events` — API-key auth, see [docs/api/openapi.yaml](docs/api/openapi.yaml) |

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
