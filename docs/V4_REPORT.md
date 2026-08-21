# V4 Report — Production Mail Security, DNS, DKIM, Deliverability, Queue, Multi-tenant

Date: 2026-08-21. Environment: controlled/local only (no public internet).

## 1. Executive Summary

All ten V4 critical gates pass in the controlled environment. Message-ID is
canonicalized platform-wide and the legacy migration is crash-safe (the V3
duplicate incident cannot recur). A real DNS verification subsystem checks
MX/SPF/DKIM/DMARC/PTR/MTA-STS/TLS-RPT. Outbound mail is genuinely
DKIM-signed by the mail core and the signature verifies cryptographically.
Rspamd and ClamAV are deployed and wired into inbound (milter) and the
control-plane acceptance path; EICAR is rejected at SMTP, spam is filed to
Junk. Quarantine, Queue Center and Deliverability are real, data-backed
admin surfaces. An Organization → Project layer scopes domains, API keys and
SMTP credentials, with tenant isolation enforced and tested. A production
profile and startup config validation exist. Unified Mail from V3 is intact.

Not done, stated honestly: public-internet delivery is unverified (no public
IP, PTR or port 25); IP pools/reputation, admin MFA and Vault/KMS are
deferred with reasons.

## 2. Data Integrity

- `mailaddr.NormalizeMessageID` — one canonical form (strip angle brackets,
  trim, lowercase the domain only; local part case preserved per RFC 5322).
  Adopted at every JMAP boundary, MIME render, threading site, migration and
  webmail lookup. The two dead webmail queries from V3 (bracket mismatch)
  are fixed. Migration 00019 canonicalized existing rows.
- `cmd/migrate-mail` rewritten: import identity is the canonical Message-ID
  per folder; the duplicate guard pages the **whole** folder (the V3 bug was
  a 100-message window); the stamp runs under `context.WithoutCancel` so a
  shutdown between import and stamp cannot split them.
- Crash test at unit level (`cmd/migrate-mail/main_test.go`): import
  succeeds → stamp "crashes" → restart → **exactly one logical copy**. Plus
  page-beyond-100 and repeated-run (lost-stamps) idempotency.
- Delivery worker, Sent-copy and quarantine release all pre-check
  Message-ID via `mailservice.HasMessage` (folder-scoped, so a legitimate
  Sent+Inbox self-mail pair is never mistaken for a duplicate).

## 3. DNS

`internal/dnscheck` behind a `Resolver` interface (fixture-testable, §123).
Records: ownership TXT, MX, SPF, DKIM, DMARC, PTR (forward-confirmed),
MTA-STS, TLS-RPT. Six-state model — verified/missing/invalid/warning/
pending/dns_error — and a transient DNS failure is **never** reported as
missing. SPF parser understands v=spf1/ip4/ip6/a/mx/include/all and refuses
to suggest a second SPF record. Snapshot stored per domain (migration
00020), `Recheck DNS` endpoint is synchronous with a 9s budget. dns-mode
domains are provisioned into the mail core only after ownership+MX verify.
Matrices for SPF/DKIM/DMARC (correct/missing/malformed/multiple/wrong) in
`dnscheck_test.go`.

## 4. DKIM

- Generation: on domain provisioning the mail core generates an RSA key
  (`EnsureDKIMKey`); the control plane records selector + public key
  (migration 00021). Selector is date-versioned (`s202608-<rand>`).
- Storage: the **private key never leaves the mail core** — not in
  PostgreSQL, logs, audit or any API response. Enforced by client-boundary
  redaction (`internal/mailcore/stalwart.go` `redactBody`) and proven by
  `redaction_test.go` (reproduces the real leak found live: Stalwart's
  `fieldAlreadyExists` echoing a private-key PEM).
- Signing: outbound mail carries a valid `DKIM-Signature`. Verified
  cryptographically with `cmd/dkimverify` against the sink's captured raw
  message — PASS for both the initial and the rotated key.
- Rotation: active → previous with `retire_after` (30d); the DKIM tab
  generates/rotates and shows copy-ready TXT records.

## 5. Rspamd

Deployed (`deploy/rspamd`, `make scanners-up`), milter into Stalwart at the
DATA stage, Bayes/stats in Redis db 2. Score thresholds: ≥15 reject, ≥6
Junk. Control-plane mail also scanned via `/checkv2` (`internal/scanner`),
verdict stored as `messages.security_scan` and an `email.scanned` trace
event. GTUBE and an externally-spoofed internal-domain message are filed to
Junk with Authentication-Results; verdict/symbols recorded.

## 6. ClamAV

Deployed with persistent signature DB; INSTREAM scan via rspamd's antivirus
module (inbound) and a direct `internal/scanner` clamd client (fallback +
health). EICAR (standard safe artifact) → **rejected at SMTP 554** inbound;
via the control plane → quarantined with reason `malware`. Health
distinguishes alive from ready (clamd answers only once signatures load).

## 7. Quarantine

States pending/pending_scan/released/deleted (migration 00022); reasons
spam/malware/policy. Release reconstructs the canonical message and imports
one copy (crash-safe re-release guard), keeps the original verdict, is
audited. Delete requires confirmation and is audited. UI is a dedicated page
with a detail drawer showing metadata + signals only — no HTML body render,
no remote resources, nothing executed (§54).

## 8. Queue

`internal/queueops` over Stalwart's queue API (verified live: list returns
ids, per-id detail carries recipient status/retry_num/next_retry and the
verbatim remote reply; `PATCH {id}` = retry now, `DELETE {id}` = cancel).
Remote replies are sanitized (control chars stripped, length capped — no log
injection). Queue Center UI: summary tiles + table + detail drawer +
retry/cancel, both audited. Queue is platform-wide: org-scoped admins are
refused.

## 9. Deliverability

`internal/deliverability` from real `message_events`/`message_recipients`
(indexes in migration 00023). **Accepted vs delivered-local vs relayed are
kept strictly distinct** — "relayed" means handed to a remote queue, never
claimed as user delivery (§69); the definitions ship in the API and render
as tooltips. Provider breakdown by recipient MX (Google/Microsoft/Yahoo/
Other), time series, top failure reasons, live queue snapshot, 24h/7d/30d
ranges.

## 10. Organizations / Projects

Project layer (migration 00024): Organization → Project → Domain. Existing
installs get a Default project and all existing domains/keys/creds attach to
it automatically (§81 — no manual data fixing). Domains, API keys and SMTP
credentials carry project scope; SMTP creds inherit the mailbox domain's
project. UI: expandable org rows with per-org project create/list; domain
create accepts a project.

## 11. Tenant Isolation

E2E (`scripts/e2e.sh`, now 79 checks) seeds a second organization/tenant and
asserts, in addition to the V3 message/attachment/user checks:
org B cannot list org A domains, cannot read org A DNS (404), cannot read or
rotate org A DKIM (404), and an org-scoped admin cannot open the platform
queue (403). A regular USER is denied `/quarantine`, `/admin/queue`,
`/admin/deliverability`, `/system/infrastructure` (403).

## 12. TLS

Development keeps self-signed Stalwart TLS (`STALWART_INSECURE_TLS=true`).
Production: `docker-compose.prod.yml` (standard ports 25/587/465/993, mounted
cert+key, no published admin/HTTP port, no dev-default secrets) and
`APP_ENV=production` startup validation that **refuses to start** with a
self-signed fallback, default passwords, insecure cookies, a localhost/http
app URL, or `sslmode=disable` (`config_test.go`). This is the §196 gate: no
silent insecure fallback.

## 13. Security

- **Open relay**: unauthenticated external→external denied at RCPT (550) —
  Gate 8 PASS.
- **Spoofing**: external mail claiming an internal From scores 15+ → Junk
  with Authentication-Results (§171).
- **Secrets**: DKIM private key, master password, webhook secrets absent
  from browser responses, logs and audit — verified live and by unit tests.
- **HTML mail / Reading Pane**: unchanged from V3 (plain-text render); the
  quarantine preview is metadata-only. Full HTML sanitizer + CSP remains a
  tracked item for the inbound-HTML phase.

## 14. Infrastructure

Health V3 (`/system/infrastructure`) now covers postgres, redis, nats,
minio, stalwart, **rspamd, clamav**, worker — all reporting `ok` at report
time. Alive vs ready honored for clamav.

## 15. Frontend

New pages: Queue Center, Deliverability, Quarantine, Domain detail with
tabs (Overview/DNS/DKIM/Deliverability/Security). Reworked admin sidebar
(§136 grouping). Domain list is now click-through with a Project column;
Organizations page gained Projects. All use the existing Silver/Graphite kit
(Drawer/Tabs/Badge/ConfirmDialog/tables). `tsc`, `eslint`, `next build` all
clean; flagship pages screenshot-verified.

## 16. Database

Migrations 00019–00024: canonical Message-ID + unmigrated index; domain DNS
records + verification token; DKIM keys; security_scan + pending_scan;
deliverability indexes; projects + default-project backfill. All applied and
reversible.

## 17. Tests

- `go build ./...`, `go vet ./...` — clean. `gofmt` clean on all
  session-touched files (pre-existing gofmt-version drift on untouched files
  left alone).
- Go unit tests: new suites for mailaddr, dnscheck, mailcore (queue +
  redaction), security engine (scanner policy), config validation, and the
  migrate-mail crash/idempotency tests — all pass.
- E2E: **79 passed, 0 failed** (60 baseline + 19 new V4 checks).
- `tsc` / `eslint` / `next build` — clean.

## 18. Critical Gates

| # | Gate | Result | Evidence |
|---|------|--------|----------|
| 1 | Data integrity: crash/retry → 1 copy | PASS | `migrate-mail/main_test.go` |
| 2 | DKIM signature cryptographically valid | PASS | `cmd/dkimverify` on sink capture |
| 3 | DNS MX/SPF/DKIM/DMARC classified | PASS | `dnscheck_test.go` matrices + live recheck |
| 4 | Inbound spam → Junk/Quarantine | PASS | GTUBE/spoof → Junk (live) |
| 5 | EICAR → blocked | PASS | SMTP 554 inbound; quarantine via control plane |
| 6 | 451 deferred → Queue → retry | PASS | Queue Center live (defer-*@sink.test) |
| 7 | Multi-tenant: A cannot reach B | PASS | E2E org-isolation checks |
| 8 | Open relay denied | PASS | 550 at RCPT (live) |
| 9 | Production TLS no silent fallback | PASS | `config.validateProduction` + test |
| 10 | No secret leak | PASS | redaction tests + live grep |

## 19. Failure & Recovery

- Rspamd down → fail-open (Stalwart's built-in filter still runs); health
  shows unavailable. ClamAV down → fail-closed as defer (451 inbound;
  pending_scan control-plane). Malware → reject/quarantine. (ADR-004.)
- Stalwart down → webmail 503 MAIL_SERVICE_UNAVAILABLE, control plane and
  acceptance keep working; provisioning + DKIM generation retry on recovery
  (unchanged from V3, still holds).
- Docker/stack restart mid-session was recovered without any manual DB edit.

## 20. Public Internet Verification

**Not performed.** No public IP, no inbound port 25, no PTR, no real DNS.
Every inbound/outbound/DNS/DKIM result above is from the controlled
environment (local SMTP sink, local submission, fixture/system resolver).
The local sink is not evidence of Gmail/Microsoft delivery.

## 21. Remaining Gaps

DKIM previous-key pruning after retire_after; per-domain Security tab feed;
webhooks for the new delivery/security event types; MTA-STS/TLS-RPT policy
hosting (diagnosis only today); full inbound-HTML sanitizer + CSP; mailcheck
v2 subcommands.

## 22. Production Blockers

Public IP + PTR + port 25 + real DNS + external delivery tests; admin MFA
(foundation not built — use a vetted TOTP library); Vault/KMS for secrets
(DKIM keys currently live only in the mail core's store; app passwords still
reach Stalwart in plaintext — TECH_DEBT #11); real TLS certificates; HA and
backup/restore validation; Stalwart pinned at v0.13.4.

## 23. Manual Setup

```
docker compose up -d              # + make scanners-up for rspamd/clamav
make migrate-up                   # applies 00019–00024
make migrate-mail                 # legacy backfill (idempotent, crash-safe)
# restart api + worker
make sink-up                      # controlled delivery target for tests
```
New env vars (in `.env`/`.env.example`): PUBLIC_APP_URL, MAIL_HOSTNAME,
OUTBOUND_IP, DNS_RESOLVER_ADDR, RSPAMD_URL, RSPAMD_PASSWORD, CLAMAV_ADDR,
APP_ENV. Production: `docker compose -f docker-compose.yml -f
docker-compose.prod.yml up -d` with real secrets + TLS cert/key paths.

## 24. Recommended V5

Deploy on a real Ubuntu host with a public IP and domain; set PTR; publish
MX/SPF/DKIM/DMARC; run the first real Gmail/Microsoft delivery tests and
wire bounce/complaint feedback. Then: admin MFA, Vault/KMS secret backend,
IP pool manager + reputation from real telemetry, webhook coverage for the
new events, and the inbound-HTML sanitizer.
