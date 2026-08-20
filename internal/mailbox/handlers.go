package mailbox

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/jackc/pgx/v5"
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

type Mailbox struct {
	ID             string `json:"id"`
	OrganizationID string `json:"organization_id"`
	DomainID       string `json:"domain_id"`
	UserID         string `json:"user_id,omitempty"`
	UserEmail      string `json:"user_email,omitempty"`
	Address        string `json:"address"`
	Status         string `json:"status"`
	QuotaBytes     int64  `json:"quota_bytes"`
	UsedBytes      int64  `json:"used_bytes"`
	CreatedAt      string `json:"created_at"`
}

// List returns the tenant's mailboxes (org-scoped admins: their org only).
func (h *Handlers) List(w http.ResponseWriter, r *http.Request) {
	id := auth.IdentityFrom(r.Context())
	query := `
		SELECT m.id, m.organization_id, m.domain_id,
		       COALESCE(m.user_id::text, ''), COALESCE(u.email::text, ''),
		       m.address, m.status, m.quota_bytes, m.used_bytes, m.created_at::text
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
			&m.Address, &m.Status, &m.QuotaBytes, &m.UsedBytes, &m.CreatedAt); err != nil {
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
	httpx.JSON(w, http.StatusCreated, map[string]string{"id": mailboxID})
}
