// Package domains manages mail domains: creation, mail-core provisioning
// and DNS verification. development-mode domains bypass verification (local
// platform only); dns-mode domains prove ownership through the _mailplatform
// TXT record plus a platform MX, checked by internal/dnscheck, and are
// provisioned into the mail core only after that proof (V4 §11-16).
package domains

import (
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/yelnurq/email-server/internal/audit"
	"github.com/yelnurq/email-server/internal/auth"
	"github.com/yelnurq/email-server/internal/dnscheck"
	"github.com/yelnurq/email-server/internal/httpx"
	"github.com/yelnurq/email-server/internal/mailaddr"
	"github.com/yelnurq/email-server/internal/mailcore"
)

type Handlers struct {
	Pool        *pgxpool.Pool
	Audit       *audit.Logger
	Log         *slog.Logger
	Provisioner *mailcore.Provisioner

	// DNS verification (V4 §11): the checker plus the platform identity the
	// expected records are built from (§17 — configuration, not hardcoded).
	DNSChecker   *dnscheck.Checker
	MailHostname string
	OutboundIP   string
}

type Domain struct {
	ID                 string  `json:"id"`
	OrganizationID     string  `json:"organization_id"`
	ProjectID          *string `json:"project_id,omitempty"`
	ProjectName        string  `json:"project_name,omitempty"`
	Name               string  `json:"name"`
	Status             string  `json:"status"`
	VerificationMode   string  `json:"verification_mode"`
	ProvisioningStatus string  `json:"provisioning_status"`
	ProvisioningError  string  `json:"provisioning_error,omitempty"`
	ProvisionedAt      *string `json:"provisioned_at,omitempty"`
	CreatedAt          string  `json:"created_at"`
}

// List returns the tenant's domains (org-scoped admins: their org only).
func (h *Handlers) List(w http.ResponseWriter, r *http.Request) {
	id := auth.IdentityFrom(r.Context())
	query := `
		SELECT d.id, d.organization_id, d.project_id, COALESCE(p.name, ''),
		       d.name, d.status, d.verification_mode,
		       d.provisioning_status, d.provisioning_error, d.provisioned_at::text,
		       d.created_at::text
		FROM domains d
		LEFT JOIN projects p ON p.id = d.project_id
		WHERE d.tenant_id = $1`
	args := []any{id.TenantID}
	if !id.TenantWide() {
		query += ` AND d.organization_id = $2`
		args = append(args, id.OrganizationID)
	}
	query += ` ORDER BY d.created_at`
	rows, err := h.Pool.Query(r.Context(), query, args...)
	if err != nil {
		h.Log.Error("domain list failed", slog.String("error", err.Error()))
		httpx.Internal(w, r)
		return
	}
	defer rows.Close()
	out := []Domain{}
	for rows.Next() {
		var d Domain
		if err := rows.Scan(&d.ID, &d.OrganizationID, &d.ProjectID, &d.ProjectName,
			&d.Name, &d.Status, &d.VerificationMode,
			&d.ProvisioningStatus, &d.ProvisioningError, &d.ProvisionedAt, &d.CreatedAt); err != nil {
			httpx.Internal(w, r)
			return
		}
		out = append(out, d)
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"domains": out})
}

type createRequest struct {
	OrganizationID   string `json:"organization_id"`
	ProjectID        string `json:"project_id"`
	Name             string `json:"name"`
	VerificationMode string `json:"verification_mode"`
}

// Create provisions a domain. development mode becomes verified immediately
// (local-only bypass); dns mode stays pending until real verification exists.
func (h *Handlers) Create(w http.ResponseWriter, r *http.Request) {
	id := auth.IdentityFrom(r.Context())
	var req createRequest
	if err := httpx.Decode(w, r, &req); err != nil {
		httpx.Error(w, r, http.StatusBadRequest, "INVALID_BODY", err.Error())
		return
	}
	name, err := mailaddr.NormalizeDomain(req.Name)
	if err != nil {
		httpx.Error(w, r, http.StatusBadRequest, "INVALID_DOMAIN", "Invalid domain name")
		return
	}
	mode := req.VerificationMode
	if mode == "" {
		mode = "development"
	}
	if mode != "development" && mode != "dns" {
		httpx.Error(w, r, http.StatusBadRequest, "INVALID_MODE", "verification_mode must be development or dns")
		return
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
	// Verify the organization belongs to the caller's tenant.
	var exists bool
	if err := h.Pool.QueryRow(r.Context(),
		`SELECT EXISTS (SELECT 1 FROM organizations WHERE id = $1 AND tenant_id = $2)`,
		orgID, id.TenantID).Scan(&exists); err != nil || !exists {
		httpx.Error(w, r, http.StatusNotFound, "ORGANIZATION_NOT_FOUND", "Organization not found")
		return
	}
	// Domain ownership (§80): the domain attaches to a project of its
	// organization — an explicit one, or the organization's Default project.
	var projectID *string
	if req.ProjectID != "" {
		if err := h.Pool.QueryRow(r.Context(), `
			SELECT id FROM projects
			WHERE id = $1 AND organization_id = $2 AND status = 'active'`,
			req.ProjectID, orgID).Scan(&projectID); err != nil {
			httpx.Error(w, r, http.StatusNotFound, "PROJECT_NOT_FOUND", "Project not found in this organization")
			return
		}
	} else {
		_ = h.Pool.QueryRow(r.Context(), `
			SELECT id FROM projects WHERE organization_id = $1 AND slug = 'default'`,
			orgID).Scan(&projectID)
	}

	status := "pending"
	if mode == "development" {
		status = "verified"
	}
	var d Domain
	err = h.Pool.QueryRow(r.Context(), `
		INSERT INTO domains (tenant_id, organization_id, project_id, name, status, verification_mode, verification_token)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (name) DO NOTHING
		RETURNING id, organization_id, project_id, name, status, verification_mode, created_at::text`,
		id.TenantID, orgID, projectID, name, status, mode, newVerificationToken()).
		Scan(&d.ID, &d.OrganizationID, &d.ProjectID, &d.Name, &d.Status, &d.VerificationMode, &d.CreatedAt)
	if err != nil {
		httpx.Error(w, r, http.StatusConflict, "DOMAIN_EXISTS", "This domain is already registered")
		return
	}
	h.Audit.Record(r.Context(), audit.Entry{
		TenantID: id.TenantID, ActorUserID: id.UserID, Action: "domain.create",
		ResourceType: "domain", ResourceID: d.ID,
		Detail: map[string]any{"name": d.Name, "mode": mode},
	})
	// Mail-core provisioning is asynchronous: a job is enqueued and the
	// worker drives it with retries; the UI polls the status. A dns-mode
	// domain is NOT provisioned until ownership is proven (§16) — the
	// verified transition in RecheckDNS enqueues the job.
	if d.Status == "verified" {
		d.ProvisioningStatus = h.Provisioner.Enqueue(r.Context(), "domain", id.TenantID, d.ID, id.UserID)
	} else {
		d.ProvisioningStatus = "pending"
	}
	httpx.JSON(w, http.StatusCreated, d)
}

// Provision retries pushing a domain into the mail core.
func (h *Handlers) Provision(w http.ResponseWriter, r *http.Request) {
	id := auth.IdentityFrom(r.Context())
	domainID := chi.URLParam(r, "id")

	var orgID string
	err := h.Pool.QueryRow(r.Context(),
		`SELECT organization_id FROM domains WHERE id = $1 AND tenant_id = $2`,
		domainID, id.TenantID).Scan(&orgID)
	if err != nil {
		httpx.Error(w, r, http.StatusNotFound, "DOMAIN_NOT_FOUND", "Domain not found")
		return
	}
	if !id.TenantWide() && orgID != id.OrganizationID {
		httpx.Error(w, r, http.StatusForbidden, "FORBIDDEN", "Cannot manage another organization")
		return
	}
	status := h.Provisioner.Enqueue(r.Context(), "domain", id.TenantID, domainID, id.UserID)
	httpx.JSON(w, http.StatusOK, map[string]string{"provisioning_status": status})
}
