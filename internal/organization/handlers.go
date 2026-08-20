// Package organization manages organizations within a tenant.
package organization

import (
	"log/slog"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/yelnurq/email-server/internal/audit"
	"github.com/yelnurq/email-server/internal/auth"
	"github.com/yelnurq/email-server/internal/httpx"
)

type Handlers struct {
	Pool  *pgxpool.Pool
	Audit *audit.Logger
	Log   *slog.Logger
}

type Organization struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Slug      string `json:"slug"`
	Status    string `json:"status"`
	CreatedAt string `json:"created_at"`
}

// List returns organizations of the caller's tenant (org-scoped admins see
// only their own organization).
func (h *Handlers) List(w http.ResponseWriter, r *http.Request) {
	id := auth.IdentityFrom(r.Context())
	query := `
		SELECT id, name, slug, status, created_at::text
		FROM organizations WHERE tenant_id = $1`
	args := []any{id.TenantID}
	if !id.TenantWide() {
		query += ` AND id = $2`
		args = append(args, id.OrganizationID)
	}
	query += ` ORDER BY created_at`
	rows, err := h.Pool.Query(r.Context(), query, args...)
	if err != nil {
		h.Log.Error("org list failed", slog.String("error", err.Error()))
		httpx.Internal(w, r)
		return
	}
	defer rows.Close()
	orgs := []Organization{}
	for rows.Next() {
		var o Organization
		if err := rows.Scan(&o.ID, &o.Name, &o.Slug, &o.Status, &o.CreatedAt); err != nil {
			httpx.Internal(w, r)
			return
		}
		orgs = append(orgs, o)
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"organizations": orgs})
}

type createRequest struct {
	Name string `json:"name"`
	Slug string `json:"slug"`
}

// Create adds an organization to the caller's tenant. Requires a
// tenant-wide admin role.
func (h *Handlers) Create(w http.ResponseWriter, r *http.Request) {
	id := auth.IdentityFrom(r.Context())
	if !id.TenantWide() {
		httpx.Error(w, r, http.StatusForbidden, "FORBIDDEN", "Only tenant admins can create organizations")
		return
	}
	var req createRequest
	if err := httpx.Decode(w, r, &req); err != nil {
		httpx.Error(w, r, http.StatusBadRequest, "INVALID_BODY", err.Error())
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" || len(req.Name) > 200 {
		httpx.Error(w, r, http.StatusBadRequest, "INVALID_NAME", "Organization name is required (max 200 chars)")
		return
	}
	slug := strings.TrimSpace(req.Slug)
	if slug == "" {
		slug = slugify(req.Name)
	}
	if slug == "" {
		httpx.Error(w, r, http.StatusBadRequest, "INVALID_SLUG", "Slug could not be derived from the name")
		return
	}

	var o Organization
	err := h.Pool.QueryRow(r.Context(), `
		INSERT INTO organizations (tenant_id, name, slug)
		VALUES ($1, $2, $3)
		ON CONFLICT (tenant_id, slug) DO NOTHING
		RETURNING id, name, slug, status, created_at::text`,
		id.TenantID, req.Name, slug).
		Scan(&o.ID, &o.Name, &o.Slug, &o.Status, &o.CreatedAt)
	if err != nil {
		httpx.Error(w, r, http.StatusConflict, "ORGANIZATION_EXISTS", "An organization with this slug already exists")
		return
	}
	h.Audit.Record(r.Context(), audit.Entry{
		TenantID: id.TenantID, ActorUserID: id.UserID, Action: "organization.create",
		ResourceType: "organization", ResourceID: o.ID,
		Detail: map[string]any{"name": o.Name, "slug": o.Slug},
	})
	httpx.JSON(w, http.StatusCreated, o)
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
