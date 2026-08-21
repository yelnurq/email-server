package organization

import (
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"

	"github.com/yelnurq/email-server/internal/audit"
	"github.com/yelnurq/email-server/internal/auth"
	"github.com/yelnurq/email-server/internal/httpx"
)

// Project is the infrastructure scope inside an organization (V4 §79):
// domains, API keys and SMTP credentials attach to a project. It is not a
// Department (§83) — departments group people, projects group mail
// infrastructure.
type Project struct {
	ID             string `json:"id"`
	OrganizationID string `json:"organization_id"`
	Name           string `json:"name"`
	Slug           string `json:"slug"`
	Status         string `json:"status"`
	Domains        int    `json:"domains"`
	CreatedAt      string `json:"created_at"`
}

// orgScope verifies the target organization belongs to the caller's tenant
// and, for org-scoped admins, to their own organization.
func (h *Handlers) orgScope(r *http.Request, orgID string) (ok bool) {
	id := auth.IdentityFrom(r.Context())
	if orgID == "" {
		return false
	}
	if !id.TenantWide() && orgID != id.OrganizationID {
		return false
	}
	var exists bool
	err := h.Pool.QueryRow(r.Context(),
		`SELECT EXISTS (SELECT 1 FROM organizations WHERE id = $1 AND tenant_id = $2)`,
		orgID, id.TenantID).Scan(&exists)
	return err == nil && exists
}

// ListProjects returns the projects of one organization
// (GET /organizations/{id}/projects).
func (h *Handlers) ListProjects(w http.ResponseWriter, r *http.Request) {
	orgID := chi.URLParam(r, "id")
	if !h.orgScope(r, orgID) {
		httpx.Error(w, r, http.StatusNotFound, "ORGANIZATION_NOT_FOUND", "Organization not found")
		return
	}
	rows, err := h.Pool.Query(r.Context(), `
		SELECT p.id, p.organization_id, p.name, p.slug::text, p.status, p.created_at::text,
		       (SELECT count(*) FROM domains d WHERE d.project_id = p.id)
		FROM projects p
		WHERE p.organization_id = $1
		ORDER BY p.created_at`, orgID)
	if err != nil {
		h.Log.Error("project list failed", slog.String("error", err.Error()))
		httpx.Internal(w, r)
		return
	}
	defer rows.Close()
	out := []Project{}
	for rows.Next() {
		var p Project
		if err := rows.Scan(&p.ID, &p.OrganizationID, &p.Name, &p.Slug, &p.Status, &p.CreatedAt, &p.Domains); err != nil {
			httpx.Internal(w, r)
			return
		}
		out = append(out, p)
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"projects": out})
}

type projectRequest struct {
	Name string `json:"name"`
	Slug string `json:"slug"`
}

// CreateProject adds a project to one organization
// (POST /organizations/{id}/projects).
func (h *Handlers) CreateProject(w http.ResponseWriter, r *http.Request) {
	id := auth.IdentityFrom(r.Context())
	orgID := chi.URLParam(r, "id")
	if !h.orgScope(r, orgID) {
		httpx.Error(w, r, http.StatusNotFound, "ORGANIZATION_NOT_FOUND", "Organization not found")
		return
	}
	var req projectRequest
	if err := httpx.Decode(w, r, &req); err != nil {
		httpx.Error(w, r, http.StatusBadRequest, "INVALID_BODY", err.Error())
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" || len(req.Name) > 200 {
		httpx.Error(w, r, http.StatusBadRequest, "INVALID_NAME", "Project name is required (max 200 chars)")
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

	var p Project
	err := h.Pool.QueryRow(r.Context(), `
		INSERT INTO projects (tenant_id, organization_id, name, slug)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (organization_id, slug) DO NOTHING
		RETURNING id, organization_id, name, slug::text, status, created_at::text`,
		id.TenantID, orgID, req.Name, slug).
		Scan(&p.ID, &p.OrganizationID, &p.Name, &p.Slug, &p.Status, &p.CreatedAt)
	if err != nil {
		httpx.Error(w, r, http.StatusConflict, "PROJECT_EXISTS", "A project with this slug already exists in the organization")
		return
	}
	h.Audit.Record(r.Context(), audit.Entry{
		TenantID: id.TenantID, ActorUserID: id.UserID, Action: "project.create",
		ResourceType: "project", ResourceID: p.ID,
		Detail: map[string]any{"name": p.Name, "slug": p.Slug, "organization_id": orgID},
	})
	httpx.JSON(w, http.StatusCreated, p)
}

type projectPatch struct {
	Name   *string `json:"name"`
	Status *string `json:"status"`
}

// PatchProject renames or archives a project (PATCH /projects/{id}).
func (h *Handlers) PatchProject(w http.ResponseWriter, r *http.Request) {
	id := auth.IdentityFrom(r.Context())
	projectID := chi.URLParam(r, "id")

	var orgID string
	query := `SELECT organization_id FROM projects WHERE id = $1 AND tenant_id = $2`
	if err := h.Pool.QueryRow(r.Context(), query, projectID, id.TenantID).Scan(&orgID); err != nil {
		httpx.Error(w, r, http.StatusNotFound, "PROJECT_NOT_FOUND", "Project not found")
		return
	}
	if !id.TenantWide() && orgID != id.OrganizationID {
		httpx.Error(w, r, http.StatusNotFound, "PROJECT_NOT_FOUND", "Project not found")
		return
	}

	var req projectPatch
	if err := httpx.Decode(w, r, &req); err != nil {
		httpx.Error(w, r, http.StatusBadRequest, "INVALID_BODY", err.Error())
		return
	}
	if req.Status != nil && *req.Status != "active" && *req.Status != "archived" {
		httpx.Error(w, r, http.StatusBadRequest, "INVALID_STATUS", "status must be active or archived")
		return
	}
	if req.Name != nil {
		n := strings.TrimSpace(*req.Name)
		if n == "" || len(n) > 200 {
			httpx.Error(w, r, http.StatusBadRequest, "INVALID_NAME", "Project name is required (max 200 chars)")
			return
		}
		req.Name = &n
	}

	var p Project
	err := h.Pool.QueryRow(r.Context(), `
		UPDATE projects SET
			name = COALESCE($2, name),
			status = COALESCE($3, status),
			updated_at = now()
		WHERE id = $1
		RETURNING id, organization_id, name, slug::text, status, created_at::text`,
		projectID, req.Name, req.Status).
		Scan(&p.ID, &p.OrganizationID, &p.Name, &p.Slug, &p.Status, &p.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) || err != nil {
		httpx.Internal(w, r)
		return
	}
	h.Audit.Record(r.Context(), audit.Entry{
		TenantID: id.TenantID, ActorUserID: id.UserID, Action: "project.update",
		ResourceType: "project", ResourceID: p.ID,
		Detail: map[string]any{"name": p.Name, "status": p.Status},
	})
	httpx.JSON(w, http.StatusOK, p)
}
