# ADR-000: Foundation architecture for Phase 1

Date: 2026-08-20
Status: Accepted

## Context

We are starting the Mail Platform (Email & Communication Platform v3.0) from an
empty repository. Phase 1 targets a local Webmail MVP developed on Windows 11,
with a hard requirement of being Linux-ready without architectural rework.
The full spec mandates the stack (Go/chi/pgx, Next.js/TS/Tailwind, PostgreSQL,
Redis, NATS JetStream, MinIO) — this ADR records how we assemble it, not
whether to use it.

## Decision

1. **Modular monolith in Go**, single module `github.com/yelnurq/email-server`,
   entrypoints `cmd/api`, `cmd/worker`, `cmd/migrate`; domain modules under
   `internal/`. Microservices are deferred; boundaries are package-level.
2. **Single Next.js application** in `apps/web` serving `/auth/*`, `/mail/*`,
   `/admin/*`. No separate admin frontend.
3. **Infrastructure as Linux containers** via one `docker-compose.yml`
   (PostgreSQL 16, Redis 7, NATS 2.10 JetStream, MinIO), kept compatible with
   plain Linux Docker Compose. Host ports 5433 (Postgres) and 6380 (Redis) are
   used because the dev machine runs native PostgreSQL/Memurai on the defaults;
   in-container ports stay standard.
4. **Migrations with `pressly/goose`** embedded via `embed.FS` and executed by
   `cmd/migrate` (`go run ./cmd/migrate up`). Chosen over golang-migrate to
   avoid a separate platform-specific CLI binary — one cross-platform Go
   command works identically on Windows and Linux.
5. **Configuration only via environment variables** (documented in
   `.env.example`); no config files with secrets in git.
6. **Logging with stdlib `log/slog`** (JSON or text), request IDs via chi
   middleware. No third-party logging framework.
7. **Local delivery is application-level through NATS JetStream** until the
   mail-core ADR (ADR-001, Milestone 8) selects Stalwart or Postfix+Dovecot.

## Consequences

- One `go build` / `go test` covers the whole backend; simple CI later.
- Compose file moves to Linux unchanged except `.env` values.
- Non-default dev ports (5433/6380) must be documented in README and default
  `.env.example`; on a clean Linux host they can be switched back if desired.
- Extracting a module into a service later requires defining its transport
  (events already flow through NATS), not rewriting storage.

## Alternatives considered

- **golang-migrate CLI**: solid, but adds a separate binary to install on every
  dev/prod machine; goose-as-library keeps the toolchain "Go + Docker + Node" only.
- **Separate admin frontend**: unnecessary complexity for Phase 1 (spec §11).
- **Running infra natively on Windows** (existing PostgreSQL 16 + Memurai):
  rejected as primary path — diverges from the future Linux environment; kept
  as a documented fallback only.
