# Project Status

Updated: 2026-08-20 (night, stage 2). States: VERIFIED (tested by
command/E2E), IN PROGRESS, NOT STARTED, BLOCKED.

Automated gate: `scripts/e2e.sh` — **60 checks, all passing** (on Windows:
`scripts/e2e.ps1` runs the same suite in a docker:cli container attached to
the compose network) — plus Go unit tests, `gofmt`/`go vet`, `tsc`,
`eslint`, `next build`, and restart persistence checks (compose services and
Stalwart).

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

**Mail core (Stalwart) — stage 2, VERIFIED**
- Stalwart v0.13.4 pinned in compose (config `deploy/stalwart/config.toml`,
  persistent `stalwartdata` volume, healthcheck); admin secret via env only
- `internal/mailcore`: `Provider` abstraction + `Stalwart` implementation
  (management REST API, live-verified against v0.13.4 incl. its
  200-with-error-body responses) + `Disabled` fallback
  (`MAIL_CORE_PROVIDER=none`)
- Provisioning lifecycle on domains/mailboxes (`pending → provisioning →
  active | failed | skipped` + stored error + retry endpoints + audit);
  wired into domain create, mailbox create, user-with-mailbox create
- SMTP credentials are pushed to Stalwart as labelled app passwords
  (create registers before commit; revoke removes remotely first; mailbox
  disable clears account secrets and revokes local creds)
- ADR-001 PoC criteria 1–3, 6 verified: provisioning via API ✓, authenticated
  submission on 587 ✓, local delivery + retrieval (JMAP) ✓, compose container
  with persistent storage ✓ (`go run ./cmd/mailcheck` automates the check);
  criteria 4–5 (outbound DKIM relay, bounce signals) need the internet phase
- Self-mail fix: `mailbox_messages` uniqueness now `(mailbox, message,
  folder)` (migration 00017); A→A lands in Sent *and* Inbox; folder moves
  merge duplicates
- Observability: Message Trace (admin search + event timeline), sender
  delivery timeline in the Reading Pane, `GET /system/infrastructure`
  (cached; postgres/redis/nats/minio/stalwart/worker heartbeat), Admin →
  Infrastructure page, dashboard "Needs attention" from real signals
- Settings → Mail clients shows IMAP/SMTP connection parameters

## IN PROGRESS

- (nothing mid-flight)

**Unified mail store — stage 3, VERIFIED (ADR-003)**
- Stalwart is now the authoritative store for Business Mail. Webmail reads
  and writes mailboxes over JMAP with master-user authentication; the
  PostgreSQL `mailbox_messages` write path is retired for Business Mail
- **Gate 1** — one message, three interfaces: a webmail send is readable in
  webmail, JMAP and IMAP under the same store id and RFC Message-ID;
  read/star set in webmail appear as `\Seen \Flagged` over IMAP
- **Gate 2** — inbound SMTP from outside (port 25, unauthenticated) is
  accepted, classified by the mail core (no SPF/DKIM/DMARC ⇒ Junk) and
  visible identically in webmail and IMAP; unknown recipients are refused
  at RCPT with 550 (never accepted-then-dropped)
- **Gate 3** — outbound through the mail core's queue against a controlled
  SMTP target (`cmd/smtpsink`, compose profile `test`, alias `sink.test`):
  250 delivered, 451 queued with `retry_num` advancing and the verbatim
  remote reply retained, 550 bounced with a DSN quoting the reply
- **Gate 4** — recovery without manual intervention: with Stalwart stopped,
  webmail degrades to 503 `MAIL_SERVICE_UNAVAILABLE`, sends are still
  accepted durably and provisioning jobs stay pending; on restart the mail
  is delivered and the jobs complete (observed: 3 attempts → done)
- Async provisioning: `provisioning_jobs` + worker with backoff replaced the
  synchronous in-handler calls
- Drafts live in the store's Drafts folder (visible to mail clients); the
  store has no in-place update, so each save writes a new copy and destroys
  the old one — the API returns the current id
- Legacy backfill: `cmd/migrate-mail` moved all 27 pre-V3 copies into the
  store, idempotent by Message-ID (proven by re-running with the stamps
  cleared: 27 skipped, 0 duplicates), plus `-dedupe` reconciliation
- Secret redaction tests (`internal/mailservice/redaction_test.go`) caught a
  real leak: an echoed `Authorization` header in an upstream error body put
  the master credential into error strings and logs. Fixed by redacting
  upstream bodies at the client boundary

## NOT STARTED (deliberately deferred, with reasons)

- DKIM signing — the mail core logs `DKIM signer not found`; outbound mail
  is unsigned. Needs key provisioning + storage design (P1)
- DNS verification (MX/SPF/DKIM/DMARC checks) — needs the internet phase for
  real records; `dns`-mode domains stay pending by design
- Rspamd / ClamAV — inbound classification currently comes from Stalwart's
  built-in filter plus our deterministic risk engine. Their failure policy
  (fail-open / fail-closed / defer) must be fixed in an ADR before wiring
- Queue Center and Deliverability UI — the data exists (mail-core queue API,
  message events); the admin surfaces are not built
- Public-internet delivery — never exercised: no public IP, no port 25
  ingress, no PTR. Everything above is controlled-environment only
- Rspamd/ClamAV integration — after mail core bridge; risk engine already
  exposes the Verdict seam they plug into
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
