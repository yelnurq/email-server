# Project Status

Updated: 2026-08-20 (evening). States: VERIFIED (tested by command/E2E),
IN PROGRESS, NOT STARTED, BLOCKED.

Automated gate: `scripts/e2e.sh` — **37 checks, all passing** — plus Go unit
tests, `gofmt`/`go vet`, `tsc`, `eslint`, `next build`, and a
`docker compose restart` persistence check.

## VERIFIED

**Foundation**
- Docker Compose (PostgreSQL 16, Redis 7, NATS 2.10 JetStream, MinIO), all
  healthy; health endpoints; structured logging; goose migrations (11 applied)
- Persistence across `docker compose restart` + API/worker restarts

**Identity & RBAC**
- Tenants/organizations/users, Argon2id credentials, opaque sessions
  (cookie + bearer, revocation, disabled users), login rate limiting,
  bootstrap admin, password change (revokes other sessions)
- Roles/permissions with platform/tenant/organization scopes; org-scoped
  admins restricted to their org; member blocked from admin APIs

**Directory**
- Development domains (verified-by-design bypass; dns mode stays pending)
- Mailbox provisioning with system folders; quotas with usage accounting
- Aliases (multi-target) and mail groups (fanout, internal-only sender
  policy); shared address namespace across mailboxes/aliases/groups

**Mail pipeline**
- Accept → PostgreSQL (+Sent copy, events, transactional outbox) → NATS
  JetStream → worker → routing (mailbox/alias/group) → folder delivery
- Idempotent delivery, retries with backoff, poison-message termination
- Threading via in_reply_to/References; drafts (create/edit/send)

**Webmail UI** — login, folders with bulk actions/pagination, message view
(thread, reply/reply-all/forward, star/unread/move/trash), compose with
autosave drafts and attachments (progress), FTS search, settings page,
responsive layout, toasts, loading/empty/error states

**Attachments** — MinIO via storage abstraction, 25MB cap, MIME sniffing,
safe filenames, checksums, authorized streaming download; quota-counted

**Search** — PostgreSQL FTS (tsvector + GIN, websearch syntax) + sender match

**Email API (Journey 3)** — API keys (hashed, scoped, revocable, secret
shown once), POST /emails (202 + stable id), batch, Idempotency-Key
(replay/conflict/in-progress semantics), status + events endpoints; OpenAPI
spec in docs/api/openapi.yaml

**Webhooks (Journey 5)** — subscriptions per event type, HMAC-SHA256 signed
deliveries (v1=hmac(secret, ts.body)), exponential backoff 5s..6h, terminal
failure after 8 attempts, delivery log, manual retry; signature verified
independently in E2E

**Security Center** — deterministic risk engine (sender blocks + content
markers; bands 0-40/41-60/61+), spam-folder routing, quarantine (hold →
release-to-inbox / delete / block sender), sender block CRUD, audit trail
API + UI; email.quarantined events

**SMTP credentials** — mailbox-scoped, hashed at rest, shown once,
revocable, audited (listener ships with mail-core phase)

**Admin Portal** — dashboard, organizations, domains, users, mailboxes,
aliases, groups, API keys, SMTP, webhooks (with delivery log), security,
audit — all RBAC-guarded

## IN PROGRESS

- (nothing mid-flight)

## NOT STARTED (deliberately deferred, with reasons)

- Rspamd/ClamAV integration — after mail core (ADR-001); risk engine already
  exposes the Verdict seam they plug into
- Mail core PoC (Stalwart) — Linux phase; ADR-001 fixes criteria
- Forgot/reset password flow — design noted in SECURITY.md
- Sending streams / reputation scoring — needs real SMTP telemetry (spec
  says don't build before internet mail)
- ClickHouse analytics, Prometheus metrics — no mass events locally yet
- API-wide rate limiting beyond login; security headers pass
- shadcn/ui adoption (hand-rolled kit in place)

## BLOCKED (external)

- Linux deployment — needs a Linux host and user command
- Real domain / DNS / MX / SPF / DKIM / DMARC — needs domain + DNS access
- Internet SMTP send/receive — needs public IP, port 25, rDNS
