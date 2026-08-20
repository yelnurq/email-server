# Project Status

Updated: 2026-08-20. States: VERIFIED (tested by command/E2E), IN PROGRESS,
NOT STARTED, BLOCKED.

## VERIFIED

- Foundation: Docker Compose (PostgreSQL 16, Redis 7, NATS 2.10 JetStream,
  MinIO), health endpoints, structured logging, migrations (goose), CI-style
  local checks (`gofmt`, `go vet`, `go test`, `tsc`, `eslint`, `next build`)
- Identity: tenants, organizations, users, Argon2id credentials, sessions
  (cookie + bearer, revocation), login/logout/me, login rate limiting,
  bootstrap admin on empty database
- RBAC: roles/permissions baseline, permission middleware, org-scoped vs
  tenant-wide admins, member blocked from admin APIs (E2E)
- Directory: development domains (verified-by-design bypass, dns mode stays
  pending), mailbox provisioning with system folders, user+credential+role+
  mailbox atomic creation, aliases schema (multi-target)
- Local mail pipeline: accept → PostgreSQL (+Sent copy, events, outbox) →
  NATS JetStream → worker → routing (mailboxes + alias targets) → Inbox;
  idempotent delivery, quota accounting, per-recipient status/events,
  retry/poison handling; transactional outbox publisher
- Webmail API: summary/folders, list with pagination+search, message view
  with thread and bcc-hiding, read/star/move/delete, drafts (create/update/
  send), send with threading (in_reply_to)
- Webmail UI: login, folder views (bulk actions, pagination), message view
  (reply/reply-all/forward/star/unread/trash), compose (cc/bcc, autosave
  drafts, forward), responsive sidebar with unread counters, toasts,
  loading/empty/error states
- Admin UI: dashboard, organizations, domains, users (create with mailbox),
  mailboxes; RBAC-guarded
- Audit log: auth events, organization/domain/user/mailbox creation
- E2E suite (`scripts/e2e.sh`): 19 checks — full send/receive/reply journey,
  Sent copies, same-tenant mailbox isolation, cross-tenant isolation
  (message/users/domains), member RBAC, revoked session
- Persistence: `docker compose restart` + API/worker restart keeps users,
  domains, mailboxes, messages, migrations state

## IN PROGRESS

- (nothing mid-flight; next block not started)

## NOT STARTED

- Attachments (MinIO storage, upload/download with authorization) — Journey 4
- Alias admin UI/API endpoints (schema + routing done; create/list endpoints
  and UI pending) — Journey 2 delivery path works at the routing level
- Mail groups
- Email API (`POST /api/v1/emails`, batch, events endpoint) + API keys +
  Idempotency-Key — Journey 3
- Webhooks (endpoints, HMAC signing, retries, delivery log) — Journey 5
- Search beyond ILIKE (PostgreSQL FTS abstraction)
- Admin: aliases/groups/api-keys/smtp/webhooks/security/quarantine/audit pages
- Security pipeline (Rspamd, ClamAV, risk engine, quarantine) — architecture
  reserved, integration deferred until after mail core ADR
- SMTP credentials, sending streams, reputation model
- Mail core PoC (Stalwart) — ADR-001 written, PoC deferred to the
  Linux/Internet SMTP phase
- Password reset flow (architecture noted in SECURITY.md)
- Prometheus metrics, ClickHouse analytics

## BLOCKED (external)

- Linux deployment (needs a Linux host and user command)
- Real domain / DNS / MX / SPF / DKIM / DMARC (needs domain + DNS access)
- Internet SMTP send/receive (needs public IP, port 25, rDNS)
