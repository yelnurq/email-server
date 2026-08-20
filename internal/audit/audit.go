// Package audit records security-relevant actions. Entries never contain
// secrets, passwords, tokens or mail content.
package audit

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Logger writes audit entries to PostgreSQL. A failed audit write is logged
// but does not fail the audited operation (availability over strictness in
// Phase 1; revisit for compliance-grade auditing).
type Logger struct {
	Pool *pgxpool.Pool
	Log  *slog.Logger
}

// Entry describes one audited action.
type Entry struct {
	TenantID     string
	ActorUserID  string
	Action       string // e.g. "auth.login", "user.create"
	ResourceType string
	ResourceID   string
	Detail       map[string]any
	IP           string
}

// Record persists the entry.
func (l *Logger) Record(ctx context.Context, e Entry) {
	detail := e.Detail
	if detail == nil {
		detail = map[string]any{}
	}
	raw, err := json.Marshal(detail)
	if err != nil {
		raw = []byte("{}")
	}
	var tenantID, actorID *string
	if e.TenantID != "" {
		tenantID = &e.TenantID
	}
	if e.ActorUserID != "" {
		actorID = &e.ActorUserID
	}
	_, err = l.Pool.Exec(ctx, `
		INSERT INTO audit_logs (tenant_id, actor_user_id, action, resource_type, resource_id, detail, ip)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		tenantID, actorID, e.Action, e.ResourceType, e.ResourceID, raw, e.IP)
	if err != nil {
		l.Log.Error("audit write failed", slog.String("action", e.Action), slog.String("error", err.Error()))
	}
}
