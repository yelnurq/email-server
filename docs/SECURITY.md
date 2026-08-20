# Security Notes

Living document for security-relevant decisions. Updated 2026-08-20.

## Authentication

- Passwords: Argon2id (m=64MiB, t=3, p=4), PHC-encoded so parameters can be
  upgraded with rehash-on-login later. Constant-time comparison. A dummy hash
  is verified for unknown users to equalize login timing.
- Login rate limiting: Redis fixed window, 10 attempts / 5 min per IP and per
  email. Fails open on Redis outage (availability choice; revisit for prod).
- Password policy: minimum 10 characters (admin-created users and bootstrap).
- Password reset: NOT implemented yet. Planned design: single-use, short-TTL
  token stored hashed, delivered out-of-band; never a bare link with the raw
  token logged. Do not ship a guessable/loggable reset.

## Sessions

- Opaque 256-bit random tokens; only SHA-256 hashes stored in `sessions`.
- Delivered as HttpOnly SameSite=Lax cookie (localhost:3000 and :8080 are
  same-site) and accepted as `Authorization: Bearer` for API clients.
- 7-day TTL, revocation on logout (`revoked_at`), disabled users are rejected
  at resolve time. Revoked-session rejection covered by E2E.
- COOKIE_SECURE=false only for local HTTP development; must be true when TLS
  arrives.

## Tenant boundary

- Tenant context always derives from the authenticated session — never from
  request input. Every query on tenant-owned resources filters by
  `tenant_id` from the identity.
- Org-scoped admins are additionally restricted to their organization.
- Webmail access goes through the caller's own mailbox (user_id match);
  mailbox_message ids from another mailbox return 404.
- E2E enforces: cross-tenant message read, user list, domain list all fail.

## RBAC

- Roles→permissions resolved server-side per request; frontend permissions
  are UX-only. Assignable roles are whitelisted (`super_admin` and
  `tenant_owner` cannot be granted via the users API).

## Mail content

- Compose accepts plain text only in Phase 1; bodies are rendered with
  `whitespace-pre-wrap`, never `dangerouslySetInnerHTML`. When HTML email
  arrives (Email API / inbound SMTP), bodies MUST be sanitized before
  rendering (sanitizer + CSP; remote images off by default).
- Bcc recipients are only returned to the sender.
- Logs carry message ids and metadata only — never bodies, subjects are also
  excluded from logs.

## Secrets

- `.env` is gitignored; `.env.example` holds placeholders only.
- Audit log entries never contain secrets or tokens.
- No secrets in the frontend bundle; the only public env var is the API URL.

## API keys & SMTP credentials

- API keys: `mpk_` bearer tokens; only SHA-256 hashes stored, secret shown
  once at creation; scoped (emails.send / emails.read), revocable, expiry
  supported, last-used stamped. Non-assignable roles cannot be granted.
- SMTP credentials: mailbox-scoped, hashed at rest, shown once, revocable.
- Webhook signing secrets are stored in clear (needed to sign each
  delivery); encrypt-at-rest is a tracked hardening item.

## Webhooks

- Every delivery is signed: `X-Mailplatform-Signature: v1=` HMAC-SHA256 over
  `<unix-ts>.<body>`; receivers must verify with constant-time comparison
  (helper in internal/webhooks). Timestamp header enables replay windows.

## Quarantine & risk baseline

- Deterministic engine only (sender blocks, explicit content markers);
  bands: 0-40 allow, 41-60 spam folder, 61+ quarantine. Quarantined mail
  never reaches the mailbox until an analyst releases it; release/delete/
  block-sender are audited. No AI/ML verdict source.

## Attachments

- 25 MiB cap, content-type sniffing (never filename-trusted), path-stripped
  filenames, SHA-256 checksums, tenant-keyed storage paths. Downloads are
  streamed through the API with ownership checks (uploader or holder of a
  mailbox copy); cross-tenant ids return 404 (E2E-enforced). AV scanning
  (ClamAV) plugs in at the mail-core phase.

## Known gaps (tracked)

- API-wide rate limiting beyond login is not implemented yet.
- Security headers (CSP, X-Frame-Options...) not yet set on the API/frontend.
- Webhook secrets and future DKIM keys need encrypt-at-rest before
  production.
