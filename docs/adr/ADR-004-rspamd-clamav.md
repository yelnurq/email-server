# ADR-004: Rspamd + ClamAV integration and failure policy

Date: 2026-08-21 (stage V4)
Status: Accepted

## Context

V3 left inbound classification to Stalwart's built-in filter and a
deterministic risk engine on the control-plane acceptance path. V4 (§38-47)
requires real external engines — Rspamd for spam, ClamAV for malware — with
an explicitly chosen failure policy (§108 of the original spec: never pick
fail-open/fail-closed implicitly).

## Decision

### Topology

```text
inbound SMTP (25)
  → Stalwart (SPF/DKIM/DMARC evaluation, its filter stays on)
  → milter (session.milter.rspamd, DATA stage)
  → Rspamd proxy worker (self-scan)
      ├─ score < 6   → no action        → normal placement
      ├─ 6 ≤ score < 15 → add header    → Junk (X-Spam / X-Spam-Status)
      ├─ score ≥ 15  → reject           → SMTP 554 5.7.1
      └─ antivirus module → ClamAV (clamd:3310, INSTREAM per MIME part)
           ├─ detected      → force reject 554 (whatever the score)
           └─ clamd down    → force soft reject 451
```

- One integration point: Stalwart's milter client — a mechanism both sides
  support natively. No spam engine is duplicated in Go (§39).
- Rspamd's Bayes/statistics live in the platform Redis (db 2).
- Greylisting is disabled (hostile to transactional mail).
- The milter is configured through Stalwart's settings API under
  `session.milter.rspamd.*`. v0.13.4 quirks, verified live: the settings
  API `insert` requires the `assert_empty` field; milter settings are read
  under `session.milter.*` (not `session.data.milter.*`) and take effect
  after a server restart, not a mere `/api/reload`.

### Failure policy — inbound (SMTP edge)

| Failure | Behavior | Rationale |
|---|---|---|
| Rspamd unreachable | **fail-open**: Stalwart delivers using its built-in filter only (milter `tempfail_on_error` off) | Mail flow must survive a scanner outage; the built-in filter still runs SPF/DKIM/DMARC/DNSBL. An outage is surfaced through health (§114), not by bouncing legitimate mail. |
| ClamAV unreachable | **fail-closed as defer**: force `soft reject` (451) via `CLAM_VIRUS_FAIL` | An unscanned attachment must never be treated as safe (§47). 451 keeps the mail retryable on the sender's side; nothing is lost, nothing unscanned is delivered. |
| Malware detected | **reject 554 at SMTP** | Refusing at the protocol edge is the strongest honest signal: the sender knows, nothing malicious is stored, and no accept-then-drop occurs (§66). The platform-side quarantine (with admin release) applies to control-plane-accepted mail — webmail/API attachments — where the platform itself is the acceptance point. |
| Spam ≥ reject threshold | reject 554 | Same §66 reasoning; the threshold (15) is conservative. |

### Failure policy — outbound (control plane → submission)

Outbound platform mail is authored by authenticated users and passes the
platform's own outbound-abuse checks; it is not routed through Rspamd for
content scoring. Attachments uploaded through the platform are scanned by
ClamAV on the acceptance path; if clamd is down the message is held as
`pending_scan` (quarantine state, §48) rather than sent unscanned —
release happens automatically when scanning succeeds on retry.

### Control-plane signals (§41)

For mail accepted by the platform (webmail, Email API, broadcast) the
security engine additionally queries Rspamd over HTTP (`/checkv2`) and
records score, action and symbols into the message's security metadata;
these power Security Center detail views. The HTTP path shares the same
engine and configuration as the milter path, so verdicts agree.

## Consequences

- Inbound spam/malware policy is enforced before storage; the mail store
  holds no known-malicious content.
- The EICAR flow (§46) is provable end-to-end: SMTP → milter → Rspamd →
  ClamAV → 554, verified live.
- A deliberate, documented deviation from §192's literal "EICAR →
  Quarantine": inbound malware is rejected at SMTP instead of stored into a
  quarantine mailbox. Storing malware weakens the security posture and
  §66 forbids accept-and-drop; the quarantine flow with admin release
  operates on the control-plane acceptance path where the platform is the
  MX of record for the message.
- Stalwart's built-in filter remains active as defense-in-depth and as the
  fail-open fallback; double classification is acceptable (its verdict and
  Rspamd's headers are both visible in the stored message).
