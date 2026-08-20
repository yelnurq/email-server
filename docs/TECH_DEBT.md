# Technical Debt

Real debt only; feature gaps live in PROJECT_STATUS.md. Updated 2026-08-20.

1. **Message bodies live in PostgreSQL, not MinIO.** Fine for local text-only
   mail; raw MIME + large bodies must move to object storage when the Email
   API/attachments/SMTP land (per architecture). Requires a storage
   abstraction in `internal/storage`.
2. **Search is ILIKE.** Works for small mailboxes; replace with PostgreSQL
   FTS (tsvector column + GIN) behind a search abstraction before large data.
3. **Recipient resolution treats every unknown address as "no such
   mailbox".** Correct until the internet SMTP branch exists; the routing
   engine then needs a local/remote decision point (single place:
   `delivery.resolveTargets`).
4. **Login rate limiter fails open** when Redis is down. Acceptable in dev;
   production should degrade differently (e.g. stricter fallback).
5. **Windows test quirk:** `go test` needs `GOTMPDIR` inside the repo
   because Application Control blocks test binaries in the default temp dir
   (documented in README). Disappears on Linux.
6. **CRLF checkout warnings** from `* text=auto` on Windows are noisy; the
   repo stores LF (correct). Consider `.gitattributes` eol=lf for code files.
7. **Session resolve runs two queries per request** (identity + roles).
   Cache identity per request only; add short-TTL Redis caching if profiling
   ever shows it matters.
8. **No OpenAPI spec yet.** Endpoints are documented in README; generate/
   write OpenAPI once the Email API (public surface) lands.
9. **shadcn/ui not adopted** — UI kit is hand-rolled. Revisit if the spec's
   component needs outgrow it.
10. **Repo lives inside OneDrive.** Known to break `create-next-app` and may
    interfere with file watching; recommend moving the checkout outside
    OneDrive.
11. **App passwords reach Stalwart as `$app$label$<plaintext>`.** Stalwart
    stores the secret as given and its management API returns it on principal
    GET. Our API never exposes it, but production should store a supported
    hash format (e.g. bcrypt) in the secret instead of plaintext.
12. **Mail-core provisioning is synchronous in request handlers** (15s
    timeout). Acceptable at current scale; move to a queued job with
    reconciliation when bulk provisioning or slow mail-core links appear.
13. **Worker liveness is a DB heartbeat row** (10s upsert, 30s threshold).
    One worker only; a fleet needs per-instance names and an aggregate view.
14. **`scripts/e2e.ps1` publishes the webhook port via docker.** If port
    39991 is taken the webhook checks fail; parameterize when it bites.
