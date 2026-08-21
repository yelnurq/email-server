// Package domains manages mail domains. In Phase 1 only development-mode
// domains are provisioned (verification bypassed by explicit design); the
// dns verification path ships with the internet SMTP phase.
package domains

import (
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/yelnurq/email-server/internal/audit"
	"github.com/yelnurq/email-server/internal/auth"
	"github.com/yelnurq/email-server/internal/httpx"
	"github.com/yelnurq/email-server/internal/mailaddr"
	"github.com/yelnurq/email-server/internal/mailcore"
)

type Handlers struct {
	Pool        *pgxpool.Pool
	Audit       *audit.Logger
	Log         *slog.Logger
	Provisioner *mailcore.Provisioner
}

type Domain struct {
	ID                 string  `json:"id"`
	OrganizationID     string  `json:"organization_id"`
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
		SELECT id, organization_id, name, status, verification_mode,
		       provisioning_status, provisioning_error, provisioned_at::text,
		       created_at::text
		FROM domains WHERE tenant_id = $1`
	args := []any{id.TenantID}
	if !id.TenantWide() {
		query += ` AND organization_id = $2`
		args = append(args, id.OrganizationID)
	}
	query += ` ORDER BY created_at`
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
		if err := rows.Scan(&d.ID, &d.OrganizationID, &d.Name, &d.Status, &d.VerificationMode,
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

	status := "pending"
	if mode == "development" {
		status = "verified"
	}
	var d Domain
	err = h.Pool.QueryRow(r.Context(), `
		INSERT INTO domains (tenant_id, organization_id, name, status, verification_mode)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (name) DO NOTHING
		RETURNING id, organization_id, name, status, verification_mode, created_at::text`,
		id.TenantID, orgID, name, status, mode).
		Scan(&d.ID, &d.OrganizationID, &d.Name, &d.Status, &d.VerificationMode, &d.CreatedAt)
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
	// worker drives it with retries. The UI polls the status.
	d.ProvisioningStatus = h.Provisioner.Enqueue(r.Context(), "domain", id.TenantID, d.ID, id.UserID)
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
