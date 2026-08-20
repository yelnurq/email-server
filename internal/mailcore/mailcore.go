// Package mailcore abstracts the mail-core engine (SMTP/IMAP/JMAP server)
// behind a provider interface so product logic never talks to a concrete
// server directly. The control plane (our Go API) owns tenants, domains,
// mailboxes, policies and audit; the mail core owns protocol I/O and
// protocol-level mailbox storage (ADR-001).
//
// Providers: "stalwart" (real integration) and "none" (mail core disabled;
// provisioning is recorded as skipped so local development works without the
// container).
package mailcore

import (
	"context"
	"errors"
	"time"
)

// Account describes a mail-core account to provision. Email doubles as the
// principal name; authentication secrets are app passwords added separately.
type Account struct {
	Email       string
	DisplayName string
	QuotaBytes  int64
}

// Status is one health observation of the mail core.
type Status struct {
	Provider  string    `json:"provider"`
	Available bool      `json:"available"`
	LatencyMS int64     `json:"latency_ms"`
	Version   string    `json:"version,omitempty"`
	Error     string    `json:"error,omitempty"`
	CheckedAt time.Time `json:"checked_at"`
}

// ErrUnavailable wraps transport-level failures so callers can map them to a
// stable MAIL_CORE_UNAVAILABLE error code instead of leaking internals.
var ErrUnavailable = errors.New("mail core unavailable")

// Provider is the mail-core management surface used by the control plane.
// Implementations must be safe for concurrent use.
type Provider interface {
	// Name identifies the provider ("stalwart", "none") for logs and health.
	Name() string
	// Enabled reports whether a real mail core is configured. When false the
	// control plane records provisioning as skipped instead of calling out.
	Enabled() bool
	// Health probes the mail core. It must be cheap (single HTTP call) —
	// callers cache the result.
	Health(ctx context.Context) Status

	// EnsureDomain makes the domain exist in the mail core (idempotent).
	EnsureDomain(ctx context.Context, name string) error
	// DeleteDomain removes the domain from the mail core.
	DeleteDomain(ctx context.Context, name string) error

	// EnsureAccount makes the account exist with the given quota (idempotent;
	// updates display name and quota on an existing account).
	EnsureAccount(ctx context.Context, a Account) error
	// SetAccountQuota updates the account's storage quota.
	SetAccountQuota(ctx context.Context, email string, quotaBytes int64) error
	// DisableAccount blocks the account: authentication secrets are cleared
	// so SMTP/IMAP/JMAP logins stop working. Mail-client passwords must be
	// re-issued after re-enabling.
	DisableAccount(ctx context.Context, email string) error

	// AddAppPassword registers a labelled application password for the
	// account (used for SMTP submission and IMAP logins). The label lets the
	// password be revoked later without knowing its value.
	AddAppPassword(ctx context.Context, email, label, password string) error
	// RemoveAppPassword revokes the labelled application password.
	RemoveAppPassword(ctx context.Context, email, label string) error
}

// Disabled is the "none" provider: every mutation reports success without
// doing anything, Enabled() is false, and health honestly reports that no
// mail core is configured.
type Disabled struct{}

func (Disabled) Name() string  { return "none" }
func (Disabled) Enabled() bool { return false }

func (Disabled) Health(context.Context) Status {
	return Status{Provider: "none", Available: false, Error: "mail core not configured", CheckedAt: time.Now().UTC()}
}

func (Disabled) EnsureDomain(context.Context, string) error           { return nil }
func (Disabled) DeleteDomain(context.Context, string) error           { return nil }
func (Disabled) EnsureAccount(context.Context, Account) error         { return nil }
func (Disabled) SetAccountQuota(context.Context, string, int64) error { return nil }
func (Disabled) DisableAccount(context.Context, string) error         { return nil }
func (Disabled) AddAppPassword(context.Context, string, string, string) error {
	return nil
}
func (Disabled) RemoveAppPassword(context.Context, string, string) error { return nil }
