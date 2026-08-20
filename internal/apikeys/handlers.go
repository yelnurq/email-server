package apikeys

import (
	"log/slog"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/yelnurq/email-server/internal/audit"
	"github.com/yelnurq/email-server/internal/auth"
	"github.com/yelnurq/email-server/internal/httpx"
)

// Handlers manages API keys through the admin API (session-authenticated).
type Handlers struct {
	Pool  *pgxpool.Pool
	Audit *audit.Logger
	Log   *slog.Logger
}

type keyView struct {
	ID             string   `json:"id"`
	OrganizationID string   `json:"organization_id"`
	Name           string   `json:"name"`
	Prefix         string   `json:"prefix"`
	Scopes         []string `json:"scopes"`
	Status         string   `json:"status"`
	ExpiresAt      *string  `json:"expires_at,omitempty"`
	LastUsedAt     *string  `json:"last_used_at,omitempty"`
	CreatedAt      string   `json:"created_at"`
}

// List returns the tenant's API keys (never secrets).
func (h *Handlers) List(w http.ResponseWriter, r *http.Request) {
	id := auth.IdentityFrom(r.Context())
	query := `
		SELECT id, organization_id, name, prefix, scopes, status,
		       expires_at::text, last_used_at::text, created_at::text
		FROM api_keys WHERE tenant_id = $1`
	args := []any{id.TenantID}
	if !id.TenantWide() {
		query += ` AND organization_id = $2`
		args = append(args, id.OrganizationID)
	}
	query += ` ORDER BY created_at DESC`
	rows, err := h.Pool.Query(r.Context(), query, args...)
	if err != nil {
		h.Log.Error("api key list failed", slog.String("error", err.Error()))
		httpx.Internal(w, r)
		return
	}
	defer rows.Close()
	out := []keyView{}
	for rows.Next() {
		var k keyView
		if err := rows.Scan(&k.ID, &k.OrganizationID, &k.Name, &k.Prefix, &k.Scopes,
			&k.Status, &k.ExpiresAt, &k.LastUsedAt, &k.CreatedAt); err != nil {
			httpx.Internal(w, r)
			return
		}
		out = append(out, k)
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"api_keys": out})
}

type createRequest struct {
	Name           string   `json:"name"`
	OrganizationID string   `json:"organization_id"`
	Scopes         []string `json:"scopes"`
}

var allowedScopes = map[string]bool{
	"emails.send": true,
	"emails.read": true,
}

// Create issues a new key. The raw secret appears only in this response.
func (h *Handlers) Create(w http.ResponseWriter, r *http.Request) {
	id := auth.IdentityFrom(r.Context())
	var req createRequest
	if err := httpx.Decode(w, r, &req); err != nil {
		httpx.Error(w, r, http.StatusBadRequest, "INVALID_BODY", err.Error())
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" || len(req.Name) > 100 {
		httpx.Error(w, r, http.StatusBadRequest, "INVALID_NAME", "Key name is required (max 100 chars)")
		return
	}
	if len(req.Scopes) == 0 {
		req.Scopes = []string{"emails.send"}
	}
	for _, s := range req.Scopes {
		if !allowedScopes[s] {
			httpx.Error(w, r, http.StatusBadRequest, "INVALID_SCOPE", "Unknown scope: "+s)
			return
		}
	}
	orgID := req.OrganizationID
	if orgID == "" {
		orgID = id.OrganizationID
	}
	if orgID == "" {
		httpx.Error(w, r, http.StatusBadRequest, "MISSING_ORGANIZATION", "organization_id is required")
		return
	}
	if !id.TenantWide() && orgID != id.OrganizationID {
		httpx.Error(w, r, http.StatusForbidden, "FORBIDDEN", "Cannot manage another organization")
		return
	}
	var orgOK bool
	if err := h.Pool.QueryRow(r.Context(),
		`SELECT EXISTS (SELECT 1 FROM organizations WHERE id = $1 AND tenant_id = $2)`,
		orgID, id.TenantID).Scan(&orgOK); err != nil || !orgOK {
		httpx.Error(w, r, http.StatusNotFound, "ORGANIZATION_NOT_FOUND", "Organization not found")
		return
	}

	raw, hash := Generate()
	prefix := raw[:12]
	var keyID string
	if err := h.Pool.QueryRow(r.Context(), `
		INSERT INTO api_keys (tenant_id, organization_id, name, prefix, secret_hash, scopes, created_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7) RETURNING id`,
		id.TenantID, orgID, req.Name, prefix, hash, req.Scopes, id.UserID).Scan(&keyID); err != nil {
		h.Log.Error("api key create failed", slog.String("error", err.Error()))
		httpx.Internal(w, r)
		return
	}
	h.Audit.Record(r.Context(), audit.Entry{
		TenantID: id.TenantID, ActorUserID: id.UserID, Action: "apikey.create",
		ResourceType: "api_key", ResourceID: keyID,
		Detail: map[string]any{"name": req.Name, "scopes": req.Scopes},
	})
	httpx.JSON(w, http.StatusCreated, map[string]any{
		"id":     keyID,
		"name":   req.Name,
		"prefix": prefix,
		"scopes": req.Scopes,
		// Shown exactly once; never retrievable again.
		"secret": raw,
	})
}

// Revoke disables a key permanently.
func (h *Handlers) Revoke(w http.ResponseWriter, r *http.Request) {
	id := auth.IdentityFrom(r.Context())
	keyID := chi.URLParam(r, "id")
	query := `UPDATE api_keys SET status = 'revoked' WHERE id = $1 AND tenant_id = $2`
	args := []any{keyID, id.TenantID}
	if !id.TenantWide() {
		query += ` AND organization_id = $3`
		args = append(args, id.OrganizationID)
	}
	ct, err := h.Pool.Exec(r.Context(), query, args...)
	if err != nil {
		httpx.Internal(w, r)
		return
	}
	if ct.RowsAffected() == 0 {
		httpx.Error(w, r, http.StatusNotFound, "KEY_NOT_FOUND", "API key not found")
		return
	}
	h.Audit.Record(r.Context(), audit.Entry{
		TenantID: id.TenantID, ActorUserID: id.UserID, Action: "apikey.revoke",
		ResourceType: "api_key", ResourceID: keyID,
	})
	httpx.JSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
