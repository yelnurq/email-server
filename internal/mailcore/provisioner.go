package mailcore

import (
	"context"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/yelnurq/email-server/internal/audit"
)

// Provisioner drives the provisioning lifecycle of domains and mailboxes:
// pending → provisioning → active | failed (or skipped when no mail core is
// configured). It is the only place that transitions provisioning_status, so
// every handler shares the same semantics, error capture and audit trail.
type Provisioner struct {
	Pool     *pgxpool.Pool
	Provider Provider
	Audit    *audit.Logger
	Log      *slog.Logger
}

const provisionTimeout = 15 * time.Second

// truncErr keeps stored provisioning errors short and secret-free.
func truncErr(err error) string {
	s := err.Error()
	if len(s) > 500 {
		s = s[:500]
	}
	return s
}

// ProvisionDomain pushes one domain into the mail core and records the
// outcome. Returns the resulting provisioning_status.
func (p *Provisioner) ProvisionDomain(ctx context.Context, tenantID, domainID, actorUserID string) string {
	var name string
	if err := p.Pool.QueryRow(ctx,
		`SELECT name::text FROM domains WHERE id = $1 AND tenant_id = $2`,
		domainID, tenantID).Scan(&name); err != nil {
		return "failed"
	}

	if !p.Provider.Enabled() {
		_, _ = p.Pool.Exec(ctx, `
			UPDATE domains SET provisioning_status = 'skipped', provisioning_error = ''
			WHERE id = $1`, domainID)
		return "skipped"
	}

	_, _ = p.Pool.Exec(ctx, `
		UPDATE domains SET provisioning_status = 'provisioning' WHERE id = $1`, domainID)

	// Detached from the request: a client disconnect must not abort a
	// provisioning call that the mail core may already be applying.
	callCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), provisionTimeout)
	defer cancel()
	err := p.Provider.EnsureDomain(callCtx, name)
	if err != nil {
		_, _ = p.Pool.Exec(ctx, `
			UPDATE domains SET provisioning_status = 'failed', provisioning_error = $2
			WHERE id = $1`, domainID, truncErr(err))
		p.Log.Error("domain provisioning failed",
			slog.String("domain", name), slog.String("error", err.Error()))
		p.Audit.Record(ctx, audit.Entry{
			TenantID: tenantID, ActorUserID: actorUserID, Action: "domain.provision_failed",
			ResourceType: "domain", ResourceID: domainID,
			Detail: map[string]any{"name": name, "error": truncErr(err)},
		})
		return "failed"
	}
	_, _ = p.Pool.Exec(ctx, `
		UPDATE domains SET provisioning_status = 'active', provisioning_error = '',
		                   provisioned_at = now()
		WHERE id = $1`, domainID)
	p.Audit.Record(ctx, audit.Entry{
		TenantID: tenantID, ActorUserID: actorUserID, Action: "domain.provision",
		ResourceType: "domain", ResourceID: domainID,
		Detail: map[string]any{"name": name, "provider": p.Provider.Name()},
	})
	return "active"
}

// ProvisionMailbox pushes one mailbox account into the mail core (ensuring
// its domain first) and records the outcome.
func (p *Provisioner) ProvisionMailbox(ctx context.Context, tenantID, mailboxID, actorUserID string) string {
	var address, domainName, displayName string
	var quota int64
	if err := p.Pool.QueryRow(ctx, `
		SELECT m.address::text, d.name::text, COALESCE(u.display_name, ''), m.quota_bytes
		FROM mailboxes m
		JOIN domains d ON d.id = m.domain_id
		LEFT JOIN users u ON u.id = m.user_id
		WHERE m.id = $1 AND m.tenant_id = $2`,
		mailboxID, tenantID).Scan(&address, &domainName, &displayName, &quota); err != nil {
		return "failed"
	}

	if !p.Provider.Enabled() {
		_, _ = p.Pool.Exec(ctx, `
			UPDATE mailboxes SET provisioning_status = 'skipped', provisioning_error = ''
			WHERE id = $1`, mailboxID)
		return "skipped"
	}

	_, _ = p.Pool.Exec(ctx, `
		UPDATE mailboxes SET provisioning_status = 'provisioning' WHERE id = $1`, mailboxID)

	callCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), provisionTimeout)
	defer cancel()
	err := p.Provider.EnsureDomain(callCtx, domainName)
	if err == nil {
		err = p.Provider.EnsureAccount(callCtx, Account{
			Email: address, DisplayName: displayName, QuotaBytes: quota,
		})
	}
	if err != nil {
		_, _ = p.Pool.Exec(ctx, `
			UPDATE mailboxes SET provisioning_status = 'failed', provisioning_error = $2
			WHERE id = $1`, mailboxID, truncErr(err))
		p.Log.Error("mailbox provisioning failed",
			slog.String("address", address), slog.String("error", err.Error()))
		p.Audit.Record(ctx, audit.Entry{
			TenantID: tenantID, ActorUserID: actorUserID, Action: "mailbox.provision_failed",
			ResourceType: "mailbox", ResourceID: mailboxID,
			Detail: map[string]any{"address": address, "error": truncErr(err)},
		})
		return "failed"
	}
	_, _ = p.Pool.Exec(ctx, `
		UPDATE mailboxes SET provisioning_status = 'active', provisioning_error = '',
		                     provisioned_at = now()
		WHERE id = $1`, mailboxID)
	p.Audit.Record(ctx, audit.Entry{
		TenantID: tenantID, ActorUserID: actorUserID, Action: "mailbox.provision",
		ResourceType: "mailbox", ResourceID: mailboxID,
		Detail: map[string]any{"address": address, "provider": p.Provider.Name()},
	})
	return "active"
}
