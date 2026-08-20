# ADR-001: Mail core selection (Stalwart vs Postfix+Dovecot)

Date: 2026-08-20
Status: Proposed (decision direction fixed; final acceptance after PoC in the
Linux phase)

## Context

The platform needs a mail core for standards-level protocol handling:
inbound/outbound SMTP on the internet, IMAP/JMAP access, DKIM signing and
verification. Phase 1 runs local-only delivery through our own application
pipeline (API → NATS → worker → mailbox), which is deliberately independent
of the mail core: the control plane, tenant model, delivery orchestrator and
security pipeline are product components and must not be replaced by an MTA's
built-ins.

## Options

### A. Stalwart

Single modern server (Rust): SMTP + IMAP + JMAP, built-in DKIM/SPF/DMARC/ARC,
sieve, REST management API, webhooks, single binary/container, first-class
Docker, S3-compatible blob storage option, active development, AGPL-3.0 with
enterprise licensing.

- (+) One container covers SMTP submission/relay, IMAP and JMAP — smallest
  operational surface for self-hosted deployments.
- (+) Management REST API + webhooks fit our control-plane-driven design
  (provision mailboxes/domains programmatically, receive delivery signals).
- (+) Native DKIM signing and inbound auth evaluation (SPF/DKIM/DMARC/ARC).
- (+) S3 blob backend aligns with our MinIO storage layer.
- (−) Younger ecosystem than Postfix/Dovecot; fewer battle-tested recipes.
- (−) AGPL: fine for self-hosted/SaaS use on our side, but obligations must
  be reviewed before distributing bundled offerings.

### B. Postfix + Dovecot

The classic pair: Postfix (SMTP) + Dovecot (IMAP/LMTP/sieve), plus OpenDKIM/
Rspamd for signing/auth.

- (+) Decades of production hardening, enormous operational knowledge base.
- (+) Very flexible via maps/policies/milters.
- (−) Two+ daemons with separate configs to template and orchestrate per
  tenant/domain; integration is file/DB-map based, not API-first.
- (−) No JMAP; DKIM et al. require additional components (OpenDKIM/Rspamd).
- (−) Higher operational complexity in Docker and for self-hosted bundles.

### C. Hybrid

Postfix edge + Stalwart/Dovecot storage. Rejected for now: double complexity
without a proven need.

## Decision

Prefer **Stalwart** as the mail core, subject to a PoC when the Linux phase
starts. PoC acceptance criteria:

1. Domain + mailbox provisioning via management API from our control plane;
2. SMTP submission (587) authenticated against per-mailbox credentials we
   manage;
3. Local delivery into Stalwart mailboxes + IMAP retrieval;
4. Outbound relay with DKIM signing using keys we provision;
5. Delivery/bounce signals consumable by our pipeline (webhooks/log/API);
6. Runs as a Linux container alongside our compose stack with S3 storage.

Any blocker in 1–6 reopens Option B.

## Boundaries (non-negotiable)

The mail core handles protocol I/O and mailbox storage for
protocol-level access. Our platform keeps: tenant model, RBAC, domains'
lifecycle, message acceptance API, delivery orchestration/events, security
policy decisions, quarantine, webhooks, audit and analytics. Webmail keeps
reading from our own data plane; the mail core is integrated behind the
routing engine so switching cores never rewrites the product.
