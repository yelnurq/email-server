package auditapi

import (
	"net/http"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/yelnurq/email-server/internal/auth"
	"github.com/yelnurq/email-server/internal/httpx"
)

// parseWhen accepts RFC3339 or a plain date and returns it normalized, or ""
// when the input is empty/invalid (invalid input must not become SQL).
func parseWhen(v string) string {
	if v == "" {
		return ""
	}
	for _, layout := range []string{time.RFC3339, "2006-01-02"} {
		if ts, err := time.Parse(layout, v); err == nil {
			return ts.UTC().Format(time.RFC3339)
		}
	}
	return ""
}

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

	// Optional filters for the admin Logs & Audit center. All are ANDed and
	// parameterized; an empty value means "no constraint".
	query := `
		SELECT a.id, COALESCE(u.email::text, ''), a.action, a.resource_type,
		       a.resource_id, a.detail, COALESCE(a.ip, ''), a.created_at::text,
		       count(*) OVER()
		FROM audit_logs a
		LEFT JOIN users u ON u.id = a.actor_user_id
		WHERE a.tenant_id = $1`
	args := []any{id.TenantID}
	next := 2
	add := func(clause, value string) {
		query += " AND " + clause + "$" + strconv.Itoa(next)
		args = append(args, value)
		next++
	}
	if v := r.URL.Query().Get("action"); v != "" {
		// Prefix match so "auth" covers auth.login / auth.login_failed / …
		add("a.action LIKE ", v+"%")
	}
	if v := r.URL.Query().Get("actor"); v != "" {
		add("u.email::text ILIKE ", "%"+v+"%")
	}
	if v := r.URL.Query().Get("resource_type"); v != "" {
		add("a.resource_type = ", v)
	}
	if v := parseWhen(r.URL.Query().Get("from")); v != "" {
		add("a.created_at >= ", v)
	}
	if v := parseWhen(r.URL.Query().Get("to")); v != "" {
		add("a.created_at < ", v)
	}
	if v := r.URL.Query().Get("q"); v != "" {
		query += ` AND (a.action ILIKE $` + strconv.Itoa(next) +
			` OR a.resource_id ILIKE $` + strconv.Itoa(next) +
			` OR a.detail::text ILIKE $` + strconv.Itoa(next) +
			` OR u.email::text ILIKE $` + strconv.Itoa(next) + `)`
		args = append(args, "%"+v+"%")
		next++
	}
	query += ` ORDER BY a.id DESC LIMIT $` + strconv.Itoa(next) + ` OFFSET $` + strconv.Itoa(next+1)
	args = append(args, limit, offset)

	rows, err := h.Pool.Query(r.Context(), query, args...)
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
