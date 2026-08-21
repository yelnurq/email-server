# Operational runbook

Symptom-first procedures for the mail platform. Every check below reads a
real signal; none of them require editing the database by hand.

Quick orientation:

- Control plane health: `GET /api/v1/system/infrastructure` (admin) —
  postgres, redis, nats, minio, stalwart, delivery worker.
- Message lifecycle: **Admin → Message Trace** (search by Message-ID,
  sender, recipient or subject).
- Mail-core queue: `GET /api/queue/messages` on the Stalwart management API
  (backend-only credentials).
- One-shot diagnosis: `go run ./cmd/mailcheck …` (SMTP submission + JMAP
  retrieval against a real mailbox).

---

## Mail not arriving (internal sender → internal recipient)

1. **Was it accepted?** Admin → Message Trace, search the subject or
   sender. No row ⇒ the send never reached the control plane; check the
   API logs for the request id the client reported.
2. **What did routing decide?** Open the trace. The timeline shows
   `email.accepted` then one of:
   - `email.delivered_local` — the mail store accepted it; continue at 4.
   - `email.quarantined` — held by policy; Admin → Security → Quarantine.
   - `email.failed` — the recipient row carries the reason (`no such
     mailbox`, `quota exceeded`, group policy).
3. **Stuck at accepted only?** The delivery worker is not consuming. Check
   `worker` in Admin → Infrastructure (it reports the last heartbeat) and
   the worker process logs.
4. **Delivered but not visible?** The message is in the mail store but the
   user is looking at the wrong folder: unauthenticated or unsigned senders
   land in **Junk**. Check Junk/Spam, and confirm with
   `go run ./cmd/mailcheck …` that the mailbox is readable at all.

## Outbound mail deferred

1. Read the mail-core queue: `GET /api/queue/messages`, then
   `GET /api/queue/messages/{id}`. Each recipient shows `status`
   (`temp_fail`), `retry_num`, `next_retry` and the **verbatim remote SMTP
   reply** — that reply is the answer to "why".
2. `4xx` replies are retried on the configured schedule; nothing to do
   unless the reply indicates a policy problem on our side (rate limits,
   missing PTR/SPF/DKIM for the sending domain).
3. If the queue keeps growing, check that outbound TCP/25 is permitted from
   the host and that DNS resolution works inside the Stalwart container:
   `docker exec mailplatform-stalwart bash -c 'exec 3<>/dev/tcp/<mx-host>/25'`.

## Message bounced

The sender receives a DSN from `MAILER-DAEMON@<domain>` quoting the remote
reply, and the Trace records the failure. A `5xx` reply is terminal by
design — do not re-queue it; fix the address or the sending reputation.

## Mail core (Stalwart) unavailable

Symptoms: Admin → Infrastructure shows `stalwart: unavailable`; webmail
mailbox reads return `MAIL_SERVICE_UNAVAILABLE` (503) and the UI shows a
service notice.

1. `docker compose ps` / `docker logs mailplatform-stalwart`.
2. `docker compose up -d stalwart` (data lives in the `stalwartdata`
   volume and survives recreation).
3. **Recovery is automatic**: accepted mail stays durable in PostgreSQL +
   JetStream and is delivered when the store returns; provisioning jobs
   retry with backoff. Verify with Admin → Infrastructure and by re-checking
   Message Trace — no manual intervention is expected.
4. While it is down, the control plane, Admin Portal and the Email API
   acceptance path keep working.

## Queue growing

1. Compare `queued` volume against delivery attempts in the Stalwart logs.
2. Identify whether one destination domain dominates (a single broken
   remote defers everything addressed to it).
3. Confirm the delivery worker is alive (Admin → Infrastructure) — a stale
   worker heartbeat means acceptance continues while local delivery stops.

## Provisioning failed

Admin → Domains / Mailboxes shows `Failed` with the stored error.

1. Use **Retry** in the UI, or `POST /api/v1/domains/{id}/provision` /
   `POST /api/v1/mailboxes/{id}/provision`. Both enqueue a job; the API
   returns immediately and the row moves `pending → provisioning → active`.
2. Inspect job history:
   `SELECT kind, status, attempts, last_error FROM provisioning_jobs ORDER BY id DESC;`
3. Transient errors (mail core unreachable) retry automatically with
   backoff; configuration errors fail terminally and need the underlying
   problem fixed first.

## DKIM missing / failing

Every provisioned domain gets an RSA signing key automatically (Admin →
Domains → open a domain → **DKIM**). Outbound mail is signed by the mail
core with the active selector.

1. **Verify the key is published**: the DKIM tab shows the selector, host
   (`<selector>._domainkey.<domain>`) and TXT value with a Copy button.
   Publish that record, then run Admin → Domains → DNS → **Recheck DNS**;
   the DKIM row turns Verified when the published key matches.
2. **Prove signing works** without the public internet:
   `make sink-up`, send to `ok-…@sink.test`, then
   `docker exec mailplatform-smtpsink tail -1 /data/sink.jsonl` — the
   `raw` field contains the `DKIM-Signature:` header. Verify it
   cryptographically:
   `go run ./cmd/dkimverify -domain <domain> -selector <selector> -pubkey <p-value>`
   (feed the raw message on stdin). Exit 0 = valid signature.
3. **Rotation**: DKIM tab → Rotate. The new key becomes active immediately;
   the previous key moves to `previous` with a `retire_after` date — keep
   its DNS record until then so in-flight mail still verifies.
4. If the mail core still logs `DKIM signer not found`, the settings did
   not reload: the signature id must be `rsa-<domain>` / `ed25519-<domain>`
   (the ids Stalwart's default rule looks up). `EnsureDKIMKey` sets this and
   calls `/api/reload`.

The DKIM **private key never leaves the mail core** — it is not stored in
PostgreSQL, logs, audit entries or any API response (only the public key
is). This is enforced by `internal/mailcore` redaction and covered by
`internal/mailcore/redaction_test.go`.

## Rspamd / ClamAV unavailable

Inbound mail is scanned by Rspamd (milter, DATA stage) with ClamAV behind
it; the control-plane acceptance path also scans through Rspamd's HTTP API.
Failure policy is fixed in `docs/adr/ADR-004-rspamd-clamav.md`:

- **Rspamd down** → fail-open: Stalwart's built-in filter still runs; mail
  flows. Health shows `rspamd: unavailable` (Admin → Infrastructure).
- **ClamAV down** → fail-closed as a deferral: inbound mail with
  attachments gets a 451 (retryable); control-plane mail with attachments
  is held `pending_scan` and released automatically when scanning recovers.
  Unscanned attachments are never delivered.
- **Malware detected** → rejected at SMTP (554) for inbound; quarantined
  (reason `malware`) for control-plane mail.

Restart a scanner with `docker compose up -d rspamd` / `clamav`. ClamAV's
first start downloads its signature database (minutes); its healthcheck
reports ready only when `clamd` accepts connections, so
`clamav: unavailable` right after start is normal (alive vs ready).

## Legacy mail migration

`go run ./cmd/migrate-mail -dry-run` reports what is pending;
`go run ./cmd/migrate-mail` performs it. Safe to re-run: rows already
migrated are stamped, and each mailbox's existing Message-IDs are checked
before importing, so repeats never duplicate mail.

If duplicates ever appear (an import interrupted before its row was
stamped): `go run ./cmd/migrate-mail -dedupe -dry-run` lists them and
`-dedupe` removes the extra copies, keeping the oldest per folder.

## Delivery testing without the public internet

`docker compose --profile test up -d smtpsink` starts a controlled SMTP
target reachable as `sink.test`. The recipient's local part selects the
response:

| Recipient            | Remote reply | Expected platform behaviour |
|----------------------|--------------|-----------------------------|
| `ok-…@sink.test`     | 250          | delivered, appears in the sink log |
| `defer-…@sink.test`  | 451          | queued, `retry_num` increases |
| `bounce-…@sink.test` | 550          | DSN to the sender, no retry |
| `slow-…@sink.test`   | 250 (delayed)| exercises timeouts |

Inspect what arrived: `docker exec mailplatform-smtpsink cat /data/sink.jsonl`.
