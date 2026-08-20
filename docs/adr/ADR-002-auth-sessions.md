# ADR-002: Authentication and session model

Date: 2026-08-20
Status: Accepted

## Context

Phase 1 needs email+password authentication for webmail and admin, usable
from both a browser (cookies) and API clients (bearer), with revocation and
tenant isolation, on PostgreSQL/Redis we already run.

## Decision

1. **Argon2id** password hashing (m=64MiB, t=3, p=4), PHC-string encoded;
   parameters are read from the stored string, enabling upgrades with
   rehash-on-login. Timing is equalized for unknown users via a dummy hash.
2. **Opaque server-side sessions** stored in PostgreSQL: 256-bit random
   token, SHA-256 hash persisted, 7-day TTL, `revoked_at` for logout/kill.
   Chosen over JWT: instant revocation, no key rotation story, one source of
   truth; session lookup joins the user and loads role→permission grants in
   one round trip.
3. **Dual transport**: HttpOnly SameSite=Lax cookie for the browser, and the
   same token as `Authorization: Bearer` for programmatic clients.
4. **RBAC**: static role→permission tables seeded by migration; grants are
   scoped (platform/tenant/organization). Handlers enforce permissions plus
   tenant scoping in queries; org-scoped admins are limited to their org.
5. **Rate limiting** on login via Redis fixed windows (per IP and email).

## Consequences

- Every authenticated request costs one indexed session lookup (+1 role
  query) — fine at Phase-1 scale, cacheable later.
- Sessions table needs periodic cleanup of expired rows (cron later).
- Password reset and MFA slots into `user_credentials`/flows without schema
  rework.
