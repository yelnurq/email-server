package mailbox

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/yelnurq/email-server/internal/audit"
	"github.com/yelnurq/email-server/internal/auth"
	"github.com/yelnurq/email-server/internal/httpx"
	"github.com/yelnurq/email-server/internal/mailcore"
)

type Handlers struct {
	Pool        *pgxpool.Pool
	Audit       *audit.Logger
	Log         *slog.Logger
	Provider    mailcore.Provider
	Provisioner *mailcore.Provisioner
}

type Mailbox struct {
	ID                 string `json:"id"`
	OrganizationID     string `json:"organization_id"`
	DomainID           string `json:"domain_id"`
	UserID             string `json:"user_id,omitempty"`
	UserEmail          string `json:"user_email,omitempty"`
	Address            string `json:"address"`
	Status             string `json:"status"`
	QuotaBytes         int64  `json:"quota_bytes"`
	UsedBytes          int64  `json:"used_bytes"`
	ProvisioningStatus string `json:"provisioning_status"`
	ProvisioningError  string `json:"provisioning_error,omitempty"`
	CreatedAt          string `json:"created_at"`
}

// List returns the tenant's mailboxes (org-scoped admins: their org only).
func (h *Handlers) List(w http.ResponseWriter, r *http.Request) {
	id := auth.IdentityFrom(r.Context())
	query := `
		SELECT m.id, m.organization_id, m.domain_id,
		       COALESCE(m.user_id::text, ''), COALESCE(u.email::text, ''),
		       m.address, m.status, m.quota_bytes, m.used_bytes,
		       m.provisioning_status, m.provisioning_error, m.created_at::text
		FROM mailboxes m
		LEFT JOIN users u ON u.id = m.user_id
		WHERE m.tenant_id = $1`
	args := []any{id.TenantID}
	if !id.TenantWide() {
		query += ` AND m.organization_id = $2`
		args = append(args, id.OrganizationID)
	}
	query += ` ORDER BY m.created_at`
	rows, err := h.Pool.Query(r.Context(), query, args...)
	if err != nil {
		h.Log.Error("mailbox list failed", slog.String("error", err.Error()))
		httpx.Internal(w, r)
		return
	}
	defer rows.Close()
	out := []Mailbox{}
	for rows.Next() {
		var m Mailbox
		if err := rows.Scan(&m.ID, &m.OrganizationID, &m.DomainID, &m.UserID, &m.UserEmail,
			&m.Address, &m.Status, &m.QuotaBytes, &m.UsedBytes,
			&m.ProvisioningStatus, &m.ProvisioningError, &m.CreatedAt); err != nil {
			httpx.Internal(w, r)
			return
		}
		out = append(out, m)
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"mailboxes": out})
}

type createRequest struct {
	DomainID  string `json:"domain_id"`
	UserID    string `json:"user_id"`
	LocalPart string `json:"local_part"`
}

// Create provisions a mailbox (with system folders) on a verified domain of
// the caller's tenant.
func (h *Handlers) Create(w http.ResponseWriter, r *http.Request) {
	id := auth.IdentityFrom(r.Context())
	var req createRequest
	if err := httpx.Decode(w, r, &req); err != nil {
		httpx.Error(w, r, http.StatusBadRequest, "INVALID_BODY", err.Error())
		return
	}

	tx, err := h.Pool.Begin(r.Context())
	if err != nil {
		httpx.Internal(w, r)
		return
	}
	defer tx.Rollback(r.Context())

	var domainName, domainOrgID, domainStatus string
	err = tx.QueryRow(r.Context(), `
		SELECT name, organization_id, status FROM domains WHERE id = $1 AND tenant_id = $2`,
		req.DomainID, id.TenantID).Scan(&domainName, &domainOrgID, &domainStatus)
	if errors.Is(err, pgx.ErrNoRows) {
		httpx.Error(w, r, http.StatusNotFound, "DOMAIN_NOT_FOUND", "Domain not found")
		return
	}
	if err != nil {
		httpx.Internal(w, r)
		return
	}
	if !id.TenantWide() && domainOrgID != id.OrganizationID {
		httpx.Error(w, r, http.StatusForbidden, "FORBIDDEN", "Cannot manage another organization")
		return
	}
	if domainStatus != "verified" {
		httpx.Error(w, r, http.StatusConflict, "DOMAIN_NOT_VERIFIED", "Domain is not verified")
		return
	}

	if req.UserID != "" {
		var userOK bool
		if err := tx.QueryRow(r.Context(),
			`SELECT EXISTS (SELECT 1 FROM users WHERE id = $1 AND tenant_id = $2)`,
			req.UserID, id.TenantID).Scan(&userOK); err != nil || !userOK {
			httpx.Error(w, r, http.StatusNotFound, "USER_NOT_FOUND", "User not found")
			return
		}
	}

	mailboxID, err := Provision(r.Context(), tx, id.TenantID, domainOrgID, req.DomainID, domainName, req.UserID, req.LocalPart)
	if errors.Is(err, ErrAddressTaken) {
		httpx.Error(w, r, http.StatusConflict, "ADDRESS_TAKEN", "This address is already in use")
		return
	}
	if err != nil {
		httpx.Error(w, r, http.StatusBadRequest, "INVALID_LOCAL_PART", "Invalid mailbox local part")
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		httpx.Internal(w, r)
		return
	}
	h.Audit.Record(r.Context(), audit.Entry{
		TenantID: id.TenantID, ActorUserID: id.UserID, Action: "mailbox.create",
		ResourceType: "mailbox", ResourceID: mailboxID,
	})
	// Provision the account in the mail core; a failure keeps the local
	// mailbox with provisioning_status=failed (retryable).
	provStatus := h.Provisioner.ProvisionMailbox(r.Context(), id.TenantID, mailboxID, id.UserID)
	httpx.JSON(w, http.StatusCreated, map[string]string{
		"id": mailboxID, "provisioning_status": provStatus,
	})
}

// lookup loads org/address/status for scope checks; writes the HTTP error
// itself when the mailbox is missing or belongs to another organization.
func (h *Handlers) lookup(w http.ResponseWriter, r *http.Request, mailboxID string) (orgID, address, status string, ok bool) {
	id := auth.IdentityFrom(r.Context())
	err := h.Pool.QueryRow(r.Context(), `
		SELECT organization_id, address::text, status FROM mailboxes
		WHERE id = $1 AND tenant_id = $2`, mailboxID, id.TenantID).
		Scan(&orgID, &address, &status)
	if errors.Is(err, pgx.ErrNoRows) {
		httpx.Error(w, r, http.StatusNotFound, "MAILBOX_NOT_FOUND", "Mailbox not found")
		return "", "", "", false
	}
	if err != nil {
		httpx.Internal(w, r)
		return "", "", "", false
	}
	if !id.TenantWide() && orgID != id.OrganizationID {
		httpx.Error(w, r, http.StatusForbidden, "FORBIDDEN", "Cannot manage another organization")
		return "", "", "", false
	}
	return orgID, address, status, true
}

type patchRequest struct {
	Status     *string `json:"status"` // active | disabled
	QuotaBytes *int64  `json:"quota_bytes"`
}

// Patch enables/disables a mailbox or changes its quota, propagating the
// change to the mail core. Disabling clears the account's mail-client
// passwords in the mail core and revokes the mailbox's SMTP credentials
// locally, so protocol access stops immediately.
func (h *Handlers) Patch(w http.ResponseWriter, r *http.Request) {
	id := auth.IdentityFrom(r.Context())
	mailboxID := chi.URLParam(r, "id")
	var req patchRequest
	if err := httpx.Decode(w, r, &req); err != nil {
		httpx.Error(w, r, http.StatusBadRequest, "INVALID_BODY", err.Error())
		return
	}
	_, address, _, ok := h.lookup(w, r, mailboxID)
	if !ok {
		return
	}

	if req.QuotaBytes != nil {
		if *req.QuotaBytes < 0 {
			httpx.Error(w, r, http.StatusBadRequest, "INVALID_QUOTA", "quota_bytes must be non-negative")
			return
		}
		if h.Provider.Enabled() {
			if err := h.Provider.SetAccountQuota(r.Context(), address, *req.QuotaBytes); err != nil {
				h.Log.Error("mail core quota update failed", slog.String("error", err.Error()))
				httpx.Error(w, r, http.StatusBadGateway, "MAIL_CORE_UNAVAILABLE",
					"The mail service did not accept the quota change; try again")
				return
			}
		}
		if _, err := h.Pool.Exec(r.Context(),
			`UPDATE mailboxes SET quota_bytes = $1, updated_at = now() WHERE id = $2`,
			*req.QuotaBytes, mailboxID); err != nil {
			httpx.Internal(w, r)
			return
		}
		h.Audit.Record(r.Context(), audit.Entry{
			TenantID: id.TenantID, ActorUserID: id.UserID, Action: "mailbox.quota_change",
			ResourceType: "mailbox", ResourceID: mailboxID,
			Detail: map[string]any{"address": address, "quota_bytes": *req.QuotaBytes},
		})
	}

	if req.Status != nil {
		switch *req.Status {
		case "disabled":
			if h.Provider.Enabled() {
				if err := h.Provider.DisableAccount(r.Context(), address); err != nil {
					h.Log.Error("mail core disable failed", slog.String("error", err.Error()))
					httpx.Error(w, r, http.StatusBadGateway, "MAIL_CORE_UNAVAILABLE",
						"The mail service did not confirm the change; the mailbox was not disabled")
					return
				}
			}
			if _, err := h.Pool.Exec(r.Context(), `
				UPDATE smtp_credentials SET status = 'revoked'
				WHERE mailbox_id = $1 AND status = 'active'`, mailboxID); err != nil {
				httpx.Internal(w, r)
				return
			}
			if _, err := h.Pool.Exec(r.Context(),
				`UPDATE mailboxes SET status = 'disabled', updated_at = now() WHERE id = $1`,
				mailboxID); err != nil {
				httpx.Internal(w, r)
				return
			}
			h.Audit.Record(r.Context(), audit.Entry{
				TenantID: id.TenantID, ActorUserID: id.UserID, Action: "mailbox.disable",
				ResourceType: "mailbox", ResourceID: mailboxID,
				Detail: map[string]any{"address": address},
			})
		case "active":
			if _, err := h.Pool.Exec(r.Context(),
				`UPDATE mailboxes SET status = 'active', updated_at = now() WHERE id = $1`,
				mailboxID); err != nil {
				httpx.Internal(w, r)
				return
			}
			// Re-ensure the account so quota/description are current. Mail
			// client passwords are NOT restored: issue new SMTP credentials.
			h.Provisioner.ProvisionMailbox(r.Context(), id.TenantID, mailboxID, id.UserID)
			h.Audit.Record(r.Context(), audit.Entry{
				TenantID: id.TenantID, ActorUserID: id.UserID, Action: "mailbox.enable",
				ResourceType: "mailbox", ResourceID: mailboxID,
				Detail: map[string]any{"address": address},
			})
		default:
			httpx.Error(w, r, http.StatusBadRequest, "INVALID_STATUS", "status must be active or disabled")
			return
		}
	}
	httpx.JSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// Provision retries pushing a mailbox account into the mail core.
func (h *Handlers) Provision(w http.ResponseWriter, r *http.Request) {
	id := auth.IdentityFrom(r.Context())
	mailboxID := chi.URLParam(r, "id")
	if _, _, _, ok := h.lookup(w, r, mailboxID); !ok {
		return
	}
	status := h.Provisioner.ProvisionMailbox(r.Context(), id.TenantID, mailboxID, id.UserID)
	httpx.JSON(w, http.StatusOK, map[string]string{"provisioning_status": status})
}
