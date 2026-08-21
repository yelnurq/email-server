# ADR-003: Unified mail storage — Stalwart as the authoritative mail store

Date: 2026-08-20 (stage V3)
Status: Accepted

## Context

After V2 the platform had two disjoint mail stores:

1. the application data plane (PostgreSQL: `messages`, `mailbox_messages`,
   `folders`) that Webmail read and the internal worker delivered into;
2. the Stalwart mail store, which SMTP submission and IMAP/JMAP clients used.

A message existed in one world or the other, never both. This blocked
Business Mail (ТЗ v3.0): Webmail and mail clients must see the same mailbox.

## Decision

**Stalwart is the authoritative store for Business Mail mailbox state**:
messages, folders, flags (read/star), threads and protocol access (SMTP,
IMAP, JMAP). Webmail becomes one more client of that store.

**PostgreSQL remains the control plane**: users, organizations, departments,
domains, permissions, provisioning state and jobs, audit, security metadata,
API keys/SMTP credentials, webhooks — plus the **transactional Email API
pipeline**, which stays on the existing PostgreSQL/NATS path (it is a
machine-mail product with its own idempotency and event semantics, per ТЗ a
separate product surface from Business Mail).

### How Webmail reaches the store

```text
Next.js → Go API (auth, RBAC) → internal/mailservice → Stalwart JMAP
```

- The backend authenticates to JMAP as `<account>%master` with the master
  secret (`STALWART_MASTER_SECRET`, backend-only; verified live on v0.13.4).
  No per-user protocol secrets are stored by the control plane and Stalwart
  admin/master credentials never reach the browser.
- JMAP `accountId` is read from the **JMAP session** (`GET /jmap/session`)
  and cached for 60s per account. Deriving it from the management API's
  numeric principal id was implemented first and rejected after it produced
  `forbidden`/`invalid JMAP ID` errors on live data: Stalwart's id codec is
  internal and its alphabet is not plain base32 (probing showed the valid
  character set is `a–z0123 7 9`). Only the server is authoritative for ids.
  Note the endpoint: `/.well-known/jmap` returns an *anonymous* session
  (empty `accounts`) for Basic auth on v0.13; `/jmap/session` returns the
  populated one.
- Folder identity uses JMAP mailbox **roles** (inbox, sent, drafts, junk,
  trash), not display names. The UI keeps its own localized labels and keeps
  exposing the legacy type string "spam" for the junk role.

### Sending and delivery — one acceptance point, one store

All product mail (webmail compose, Email API, broadcast) keeps entering
through the existing control-plane acceptance pipeline (validation, policy,
security verdict, `message_events`, transactional outbox → NATS → worker).
What changes is the **delivery target**: the worker no longer inserts rows
into `mailbox_messages`; it renders the canonical RFC822 message once and

- **local recipients** — `Email/import` into the recipient's Stalwart
  mailbox (Inbox, or Junk when the security verdict says spam; quarantine
  still withholds delivery entirely; verified live on v0.13.4);
- **the sender's Sent copy** — `Email/import` into Sent with `$seen`;
- **remote recipients** — authenticated SMTP submission (587) into
  Stalwart's outbound queue, which owns MX lookup, retry and bounces.

Alias/group fanout, quarantine release and the deterministic risk engine
keep working unchanged — they all end in the same Import call. Idempotency
stays where it was: `message_recipients.status` guards redelivery.

`EmailSubmission/set` was evaluated and rejected for now: v0.13.4 rejects
`Identity/set` for the account's own address and auto-provisions no
identities (verified live), so JMAP submission cannot be used until the
version pin moves.

### No dual-write

The `mailbox_messages` insert path is retired for Business Mail. There is
exactly one authoritative mailbox write per delivery, into Stalwart.
PostgreSQL keeps message/recipient/event metadata (the trace plane), which
is not a copy of mailbox state. If a read cache ever becomes necessary it
must be a disposable projection rebuilt from Stalwart state, never a second
authority.

### Attachments

Business Mail attachments are MIME parts in the Stalwart blob store,
uploaded/downloaded through the backend (JMAP blob endpoints). MinIO remains
the store for Email API/legacy attachments and other application artifacts.

### Events and trace

The control plane records message lifecycle events it actually observes
(acceptance, submission result, security verdicts) keyed by RFC Message-ID;
outbound queue state comes from Stalwart's queue management API. The Email
API keeps its existing NATS event pipeline untouched.

### Legacy data

Pre-V3 PostgreSQL messages are migrated per mailbox into Stalwart (RFC822
reconstruction, idempotent by Message-ID). Legacy tables are kept read-only
until the migration is proven; nothing is dropped in the same release.

### Failure behavior

Stalwart down ⇒ Webmail mailbox reads/sends fail fast with
`MAIL_SERVICE_UNAVAILABLE` and a friendly UI status; the control plane,
admin portal and Email API continue to work. Provisioning becomes
asynchronous jobs with retry, so recovery needs no manual intervention.

## Consequences

- One logical message, visible identically to Webmail, JMAP and IMAP.
- Read/unread and star flags are protocol flags (`$seen`, `$flagged`) —
  changes from any client are visible to all others.
- The webmail-specific PostgreSQL delivery worker no longer serves Business
  Mail; the platform's security verdict engine must be re-attached to the
  mail core's pipeline (Stalwart spam filter / milter-style hooks) in a
  later step — captured as an explicit gap.
- Some V2 webmail features backed by SQL (FTS search over bodies) now map
  to JMAP text search; behavior differences are acceptable and documented.
