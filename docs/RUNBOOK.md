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

Not implemented yet — the mail core logs `DKIM signer not found` for
outbound mail and no key is provisioned. Outbound mail is therefore
unsigned, which foreign providers may treat as suspicious. Tracked as a
gap; see PROJECT_STATUS.

## Rspamd / ClamAV unavailable

Not deployed yet. Inbound classification currently comes from Stalwart's
built-in spam filter (SPF/DKIM/DMARC/IPREV evaluation), plus the platform's
own deterministic risk engine on the acceptance path. When the external
scanners are introduced, their failure policy must be written into an ADR
first (fail-open vs fail-closed vs defer) rather than chosen implicitly.

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
