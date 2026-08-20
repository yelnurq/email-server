# Mail Platform — Architecture Overview

Status: Phase 1 (Windows Local Development), Milestone 1 (Foundation).
This document describes the current state and the target shape it grows into.
Requirements source: Email & Communication Platform v3.0 spec.

## 1. Goal

A corporate mail platform with Webmail and Admin UI. Phase 1 delivers a fully
local Webmail MVP on Windows (dev domain `company.test`, no internet/DNS),
built Linux-ready from day one:

```text
WINDOWS DEVELOPMENT → LOCAL MVP → LINUX MIGRATION → REAL DNS → INTERNET SMTP → PRODUCTION
```

## 2. System shape

Modular monolith in Go + single Next.js app + Linux containers for infrastructure.

```text
┌─────────────────────────────┐
│  apps/web  (Next.js)        │  /auth/*  /mail/*  /admin/*
└──────────────┬──────────────┘
               │ HTTP /api/v1
┌──────────────▼──────────────┐
│  cmd/api   (Go, chi)        │  auth · control plane · message acceptance
└───┬─────────┬───────────┬───┘
    │         │           │ publish mail.accepted
┌───▼───┐ ┌───▼───┐ ┌─────▼─────────┐
│Postgres│ │ Redis │ │ NATS JetStream│
└───▲───┘ └───────┘ └─────┬─────────┘
    │                     │ durable consumer
    │             ┌───────▼───────┐        ┌─────────┐
    └─────────────┤  cmd/worker   ├───────►│  MinIO  │
                  │ routing +     │        │ raw MIME│
                  │ local delivery│        │ attach. │
                  └───────────────┘        └─────────┘
```

Mail is NOT a CRUD insert into an inbox table. Sending goes through a real
pipeline: API accepts + persists → publishes `mail.accepted` → worker routes →
delivers into recipient mailboxes. The routing engine later gains an
Internet SMTP branch without rewriting Webmail.

## 3. Planes

- **Control plane**: tenants, organizations, users, RBAC, domains, settings, policies.
- **Data/mail plane**: mailboxes, messages, MIME, attachments, queue, routing, delivery, events.

Admin UI unavailability must never corrupt or lose accepted messages.

## 4. Backend modules (Go modular monolith)

```text
internal/
├── auth/          # sessions, password hashing, RBAC (Milestone 2)
├── tenant/        # tenancy root (Milestone 2)
├── organization/  # organizations (Milestone 2)
├── users/         # users (Milestone 2)
├── domains/       # dev-mode domains, future verification states (Milestone 3)
├── mailbox/       # mailboxes, folders (Milestone 3)
├── messages/      # message model, acceptance (Milestone 4)
├── delivery/      # routing + local delivery worker logic (Milestone 4)
├── storage/       # S3-compatible object storage abstraction (Milestone 6)
├── audit/         # audit logging
├── config/        # env-based configuration (exists)
└── server/        # HTTP server, middleware, health (exists)
```

Module boundaries are package boundaries today; any module must be extractable
into its own service later without a full rewrite.

## 5. Data stores

| Store      | Role                                                            |
|------------|-----------------------------------------------------------------|
| PostgreSQL | source of truth: metadata, relationships, mailbox/message state |
| MinIO      | raw MIME, attachments, large objects (S3-compatible, swappable) |
| Redis      | sessions, rate limits, short-lived cache, locks only            |
| NATS JS    | event/job backbone (`mail.accepted` → … → `mail.delivered_local`) |

Message model separates `Message` (immutable content identity) from
`MailboxMessage` (per-mailbox state: folder, read, starred) — one message can
appear in many mailboxes. No `message.inbox_id`.

## 6. Multi-tenancy

```text
Tenant → Organization → { Projects, Domains, Users → Mailboxes }
```

Every query is tenant-scoped; isolation is enforced in the backend, never the
frontend. Cross-tenant access by ID guessing must be impossible.

## 7. Linux-readiness rules

- No Windows-only APIs, registry, services, IIS, `C:\` paths, CRLF-dependent parsing.
- Paths via `path/filepath` / Node `path`; config via env vars (`.env` locally).
- Infrastructure runs as Linux containers; `docker-compose.yml` stays compatible
  with plain Linux Docker Compose (no Docker-Desktop-specific features).
- LF endings enforced by `.gitattributes`.

## 8. Mail core

Stalwart vs Postfix+Dovecot is decided in `docs/adr/ADR-001-mail-core.md`
(pending, Milestone 8). Until then, local delivery is application-level through
the pipeline above, designed so the mail core slots in behind the routing engine.

## 9. Observability (Phase 1 scope)

Structured JSON/text logs via `log/slog`: request ID, tenant/user context
(no sensitive data), message ID, component, UTC timestamps. No secrets, no
mail bodies in logs. Full OTel/Prometheus/Grafana stack is out of scope for now.

## 10. Health

- `GET /health/live` — process is up.
- `GET /health/ready` — Postgres, Redis, NATS, MinIO reachable; 503 otherwise.
