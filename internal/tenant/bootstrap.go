// Package tenant owns tenant lifecycle. For Phase 1 it provides the initial
// bootstrap: creating the first tenant, organization and admin user on an
// empty database so the platform is administrable without manual SQL.
package tenant

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/yelnurq/email-server/internal/auth"
)

// BootstrapConfig describes the initial admin account, typically sourced from
// BOOTSTRAP_ADMIN_EMAIL / BOOTSTRAP_ADMIN_PASSWORD environment variables.
type BootstrapConfig struct {
	AdminEmail    string
	AdminPassword string
	TenantName    string
	OrgName       string
}

// Bootstrap creates the initial tenant, organization and admin user if and
// only if no users exist yet. It is safe to run on every startup; an advisory
// lock prevents concurrent API instances from double-bootstrapping.
func Bootstrap(ctx context.Context, pool *pgxpool.Pool, log *slog.Logger, cfg BootstrapConfig) error {
	if cfg.AdminEmail == "" || cfg.AdminPassword == "" {
		return nil // bootstrap not configured; nothing to do
	}
	if len(cfg.AdminPassword) < 10 {
		return fmt.Errorf("BOOTSTRAP_ADMIN_PASSWORD must be at least 10 characters")
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext('mailplatform_bootstrap'))`); err != nil {
		return err
	}

	var userCount int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM users`).Scan(&userCount); err != nil {
		return err
	}
	if userCount > 0 {
		return nil
	}

	tenantName := cfg.TenantName
	if tenantName == "" {
		tenantName = "Default Tenant"
	}
	orgName := cfg.OrgName
	if orgName == "" {
		orgName = "Default Organization"
	}

	var tenantID string
	if err := tx.QueryRow(ctx, `
		INSERT INTO tenants (name, slug) VALUES ($1, $2) RETURNING id`,
		tenantName, slugify(tenantName)).Scan(&tenantID); err != nil {
		return err
	}
	var orgID string
	if err := tx.QueryRow(ctx, `
		INSERT INTO organizations (tenant_id, name, slug) VALUES ($1, $2, $3) RETURNING id`,
		tenantID, orgName, slugify(orgName)).Scan(&orgID); err != nil {
		return err
	}

	hash, err := auth.HashPassword(cfg.AdminPassword)
	if err != nil {
		return err
	}
	var userID string
	if err := tx.QueryRow(ctx, `
		INSERT INTO users (tenant_id, organization_id, email, display_name)
		VALUES ($1, $2, $3, $4) RETURNING id`,
		tenantID, orgID, strings.ToLower(cfg.AdminEmail), "Administrator").Scan(&userID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO user_credentials (user_id, password_hash) VALUES ($1, $2)`,
		userID, hash); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO user_roles (user_id, role_code, scope_type, scope_id)
		VALUES ($1, 'super_admin', 'platform', NULL)`,
		userID); err != nil {
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return err
	}
	log.Info("bootstrap admin created",
		slog.String("email", cfg.AdminEmail),
		slog.String("tenant_id", tenantID),
		slog.String("organization_id", orgID))
	return nil
}

func slugify(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	prevDash := false
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z' || r >= '0' && r <= '9':
			b.WriteRune(r)
			prevDash = false
		default:
			if !prevDash && b.Len() > 0 {
				b.WriteByte('-')
				prevDash = true
			}
		}
	}
	return strings.Trim(b.String(), "-")
}
