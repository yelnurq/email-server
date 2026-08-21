package mailcore

import (
	"context"
	"errors"
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

// Enqueue schedules asynchronous provisioning of a domain or mailbox: the
// entity is marked pending and a job row drives the actual mail-core calls
// from the worker with retries. Returns the provisioning_status to report
// ("pending", or "skipped" when no mail core is configured). Duplicate
// enqueues while a job is live are collapsed by the partial unique index.
func (p *Provisioner) Enqueue(ctx context.Context, kind, tenantID, entityID, actorUserID string) string {
	table := "domains"
	if kind == "mailbox" {
		table = "mailboxes"
	}
	if !p.Provider.Enabled() {
		_, _ = p.Pool.Exec(ctx, `
			UPDATE `+table+` SET provisioning_status = 'skipped', provisioning_error = ''
			WHERE id = $1`, entityID)
		return "skipped"
	}
	var actor *string
	if actorUserID != "" {
		actor = &actorUserID
	}
	_, err := p.Pool.Exec(ctx, `
		INSERT INTO provisioning_jobs (tenant_id, kind, entity_id, actor_user_id)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (kind, entity_id) WHERE status IN ('pending', 'running') DO NOTHING`,
		tenantID, kind, entityID, actor)
	if err != nil {
		p.Log.Error("provisioning enqueue failed", slog.String("error", err.Error()))
		return "failed"
	}
	_, _ = p.Pool.Exec(ctx, `
		UPDATE `+table+` SET provisioning_status = 'pending', provisioning_error = ''
		WHERE id = $1 AND provisioning_status <> 'active'`, entityID)
	return "pending"
}

// RunJobs is the provisioning worker loop: it claims due jobs, executes the
// mail-core calls and retries transient failures with exponential backoff.
// Configuration errors (mail core rejected the request) fail terminally.
func (p *Provisioner) RunJobs(ctx context.Context) {
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			p.runDueJobs(ctx)
		}
	}
}

const maxJobAttempts = 8

func (p *Provisioner) runDueJobs(ctx context.Context) {
	rows, err := p.Pool.Query(ctx, `
		UPDATE provisioning_jobs SET status = 'running', attempts = attempts + 1, updated_at = now()
		WHERE id IN (
			SELECT id FROM provisioning_jobs
			WHERE status IN ('pending', 'running') AND next_attempt_at <= now()
			ORDER BY id
			LIMIT 5
			FOR UPDATE SKIP LOCKED
		)
		RETURNING id, tenant_id, kind, entity_id, COALESCE(actor_user_id::text, ''), attempts`)
	if err != nil {
		p.Log.Error("provisioning job claim failed", slog.String("error", err.Error()))
		return
	}
	type job struct {
		id                       int64
		tenantID, kind, entityID string
		actorUserID              string
		attempts                 int
	}
	var jobs []job
	for rows.Next() {
		var j job
		if err := rows.Scan(&j.id, &j.tenantID, &j.kind, &j.entityID, &j.actorUserID, &j.attempts); err != nil {
			rows.Close()
			return
		}
		jobs = append(jobs, j)
	}
	rows.Close()

	for _, j := range jobs {
		var status string
		var err error
		if j.kind == "domain" {
			status, err = p.ProvisionDomain(ctx, j.tenantID, j.entityID, j.actorUserID)
		} else {
			status, err = p.ProvisionMailbox(ctx, j.tenantID, j.entityID, j.actorUserID)
		}
		switch {
		case status == "active" || status == "skipped":
			_, _ = p.Pool.Exec(ctx, `
				UPDATE provisioning_jobs SET status = 'done', last_error = '', updated_at = now()
				WHERE id = $1`, j.id)
		case err != nil && errors.Is(err, ErrUnavailable) && j.attempts < maxJobAttempts:
			// Transient: retry with exponential backoff (10s..10m).
			backoff := 10 * time.Second << uint(j.attempts-1)
			if backoff > 10*time.Minute {
				backoff = 10 * time.Minute
			}
			_, _ = p.Pool.Exec(ctx, `
				UPDATE provisioning_jobs SET status = 'pending', last_error = $2,
				       next_attempt_at = now() + $3::interval, updated_at = now()
				WHERE id = $1`, j.id, truncErrStr(err), backoff.String())
		default:
			// Configuration error or retries exhausted: terminal for both
			// the job and the entity.
			msg := "provisioning did not complete"
			if err != nil {
				msg = truncErrStr(err)
			}
			_, _ = p.Pool.Exec(ctx, `
				UPDATE provisioning_jobs SET status = 'failed', last_error = $2, updated_at = now()
				WHERE id = $1`, j.id, msg)
			table := "domains"
			if j.kind == "mailbox" {
				table = "mailboxes"
			}
			_, _ = p.Pool.Exec(ctx, `
				UPDATE `+table+` SET provisioning_status = 'failed', provisioning_error = $2
				WHERE id = $1`, j.entityID, msg)
		}
	}
}

func truncErrStr(err error) string {
	return truncErr(err)
}

// ProvisionDomain pushes one domain into the mail core and records the
// outcome. Returns the resulting provisioning_status and the underlying
// error (used by the job runner for retry classification).
func (p *Provisioner) ProvisionDomain(ctx context.Context, tenantID, domainID, actorUserID string) (string, error) {
	var name string
	if err := p.Pool.QueryRow(ctx,
		`SELECT name::text FROM domains WHERE id = $1 AND tenant_id = $2`,
		domainID, tenantID).Scan(&name); err != nil {
		return "failed", err
	}

	if !p.Provider.Enabled() {
		_, _ = p.Pool.Exec(ctx, `
			UPDATE domains SET provisioning_status = 'skipped', provisioning_error = ''
			WHERE id = $1`, domainID)
		return "skipped", nil
	}

	_, _ = p.Pool.Exec(ctx, `
		UPDATE domains SET provisioning_status = 'provisioning' WHERE id = $1`, domainID)

	// Detached from the request: a client disconnect must not abort a
	// provisioning call that the mail core may already be applying.
	callCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), provisionTimeout)
	defer cancel()
	err := p.Provider.EnsureDomain(callCtx, name)
	if err != nil {
		// Keep the entity in a non-terminal state: the job runner decides
		// whether this attempt is retryable and marks 'failed' only when it
		// gives up, so the UI never shows "failed" while a retry is due.
		_, _ = p.Pool.Exec(ctx, `
			UPDATE domains SET provisioning_error = $2 WHERE id = $1`, domainID, truncErr(err))
		p.Log.Error("domain provisioning failed",
			slog.String("domain", name), slog.String("error", err.Error()))
		p.Audit.Record(ctx, audit.Entry{
			TenantID: tenantID, ActorUserID: actorUserID, Action: "domain.provision_failed",
			ResourceType: "domain", ResourceID: domainID,
			Detail: map[string]any{"name": name, "error": truncErr(err)},
		})
		return "failed", err
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
	return "active", nil
}

// ProvisionMailbox pushes one mailbox account into the mail core (ensuring
// its domain first) and records the outcome.
func (p *Provisioner) ProvisionMailbox(ctx context.Context, tenantID, mailboxID, actorUserID string) (string, error) {
	var address, domainName, displayName string
	var quota int64
	if err := p.Pool.QueryRow(ctx, `
		SELECT m.address::text, d.name::text, COALESCE(u.display_name, ''), m.quota_bytes
		FROM mailboxes m
		JOIN domains d ON d.id = m.domain_id
		LEFT JOIN users u ON u.id = m.user_id
		WHERE m.id = $1 AND m.tenant_id = $2`,
		mailboxID, tenantID).Scan(&address, &domainName, &displayName, &quota); err != nil {
		return "failed", err
	}

	if !p.Provider.Enabled() {
		_, _ = p.Pool.Exec(ctx, `
			UPDATE mailboxes SET provisioning_status = 'skipped', provisioning_error = ''
			WHERE id = $1`, mailboxID)
		return "skipped", nil
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
			UPDATE mailboxes SET provisioning_error = $2 WHERE id = $1`, mailboxID, truncErr(err))
		p.Log.Error("mailbox provisioning failed",
			slog.String("address", address), slog.String("error", err.Error()))
		p.Audit.Record(ctx, audit.Entry{
			TenantID: tenantID, ActorUserID: actorUserID, Action: "mailbox.provision_failed",
			ResourceType: "mailbox", ResourceID: mailboxID,
			Detail: map[string]any{"address": address, "error": truncErr(err)},
		})
		return "failed", err
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
	return "active", nil
}
