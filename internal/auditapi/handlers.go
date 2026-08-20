package auditapi

import (
	"net/http"
	"strconv"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/yelnurq/email-server/internal/auth"
	"github.com/yelnurq/email-server/internal/httpx"
)

// Handlers exposes the tenant audit log (read-only).
type Handlers struct {
	Pool *pgxpool.Pool
}

type entryView struct {
	ID           int64          `json:"id"`
	ActorEmail   string         `json:"actor_email,omitempty"`
	Action       string         `json:"action"`
	ResourceType string         `json:"resource_type,omitempty"`
	ResourceID   string         `json:"resource_id,omitempty"`
	Detail       map[string]any `json:"detail"`
	IP           string         `json:"ip,omitempty"`
	CreatedAt    string         `json:"created_at"`
}

// List returns the tenant's audit trail, newest first.
func (h *Handlers) List(w http.ResponseWriter, r *http.Request) {
	id := auth.IdentityFrom(r.Context())
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	if offset < 0 {
		offset = 0
	}
	rows, err := h.Pool.Query(r.Context(), `
		SELECT a.id, COALESCE(u.email::text, ''), a.action, a.resource_type,
		       a.resource_id, a.detail, COALESCE(a.ip, ''), a.created_at::text,
		       count(*) OVER()
		FROM audit_logs a
		LEFT JOIN users u ON u.id = a.actor_user_id
		WHERE a.tenant_id = $1
		ORDER BY a.id DESC
		LIMIT $2 OFFSET $3`, id.TenantID, limit, offset)
	if err != nil {
		httpx.Internal(w, r)
		return
	}
	defer rows.Close()
	out := []entryView{}
	total := 0
	for rows.Next() {
		var v entryView
		if err := rows.Scan(&v.ID, &v.ActorEmail, &v.Action, &v.ResourceType,
			&v.ResourceID, &v.Detail, &v.IP, &v.CreatedAt, &total); err != nil {
			httpx.Internal(w, r)
			return
		}
		out = append(out, v)
	}
	httpx.JSON(w, http.StatusOK, map[string]any{
		"entries": out, "total": total, "limit": limit, "offset": offset,
	})
}
