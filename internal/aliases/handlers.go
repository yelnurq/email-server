// Package aliases manages mail aliases: alternate addresses that deliver to
// one or more existing mailboxes. Targets are mailboxes only (no nesting),
// which makes routing loops structurally impossible.
package aliases

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
	"github.com/yelnurq/email-server/internal/mailaddr"
	"github.com/yelnurq/email-server/internal/mailbox"
)

type Handlers struct {
	Pool  *pgxpool.Pool
	Audit *audit.Logger
	Log   *slog.Logger
}

type Alias struct {
	ID             string   `json:"id"`
	OrganizationID string   `json:"organization_id"`
	Address        string   `json:"address"`
	Status         string   `json:"status"`
	Targets        []string `json:"targets"` // target mailbox addresses
	CreatedAt      string   `json:"created_at"`
}

// List returns the tenant's aliases with their target addresses.
func (h *Handlers) List(w http.ResponseWriter, r *http.Request) {
	id := auth.IdentityFrom(r.Context())
	query := `
		SELECT a.id, a.organization_id, a.address::text, a.status, a.created_at::text,
		       COALESCE(array_agg(m.address::text ORDER BY m.address) FILTER (WHERE m.id IS NOT NULL), '{}')
		FROM mailbox_aliases a
		LEFT JOIN mailbox_alias_targets t ON t.alias_id = a.id
		LEFT JOIN mailboxes m ON m.id = t.mailbox_id
		WHERE a.tenant_id = $1`
	args := []any{id.TenantID}
	if !id.TenantWide() {
		query += ` AND a.organization_id = $2`
		args = append(args, id.OrganizationID)
	}
	query += ` GROUP BY a.id ORDER BY a.created_at`
	rows, err := h.Pool.Query(r.Context(), query, args...)
	if err != nil {
		h.Log.Error("alias list failed", slog.String("error", err.Error()))
		httpx.Internal(w, r)
		return
	}
	defer rows.Close()
	out := []Alias{}
	for rows.Next() {
		var a Alias
		if err := rows.Scan(&a.ID, &a.OrganizationID, &a.Address, &a.Status, &a.CreatedAt, &a.Targets); err != nil {
			httpx.Internal(w, r)
			return
		}
		out = append(out, a)
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"aliases": out})
}

type createRequest struct {
	DomainID         string   `json:"domain_id"`
	LocalPart        string   `json:"local_part"`
	TargetMailboxIDs []string `json:"target_mailbox_ids"`
}

// Create provisions an alias with at least one target mailbox. All resources
// must belong to the caller's tenant (and organization for org-scoped admins).
func (h *Handlers) Create(w http.ResponseWriter, r *http.Request) {
	id := auth.IdentityFrom(r.Context())
	var req createRequest
	if err := httpx.Decode(w, r, &req); err != nil {
		httpx.Error(w, r, http.StatusBadRequest, "INVALID_BODY", err.Error())
		return
	}
	if len(req.TargetMailboxIDs) == 0 || len(req.TargetMailboxIDs) > 50 {
		httpx.Error(w, r, http.StatusBadRequest, "INVALID_TARGETS", "Provide 1-50 target mailboxes")
		return
	}
	local, err := mailaddr.NormalizeLocalPart(req.LocalPart)
	if err != nil {
		httpx.Error(w, r, http.StatusBadRequest, "INVALID_LOCAL_PART", "Invalid alias local part")
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

	address := mailaddr.Join(local, domainName)
	taken, err := mailbox.AddressInUse(r.Context(), tx, address)
	if err != nil {
		httpx.Internal(w, r)
		return
	}
	if taken {
		httpx.Error(w, r, http.StatusConflict, "ADDRESS_TAKEN", "This address is already in use")
		return
	}

	var aliasID string
	if err := tx.QueryRow(r.Context(), `
		INSERT INTO mailbox_aliases (tenant_id, organization_id, domain_id, local_part, address)
		VALUES ($1, $2, $3, $4, $5) RETURNING id`,
		id.TenantID, domainOrgID, req.DomainID, local, address).Scan(&aliasID); err != nil {
		httpx.Internal(w, r)
		return
	}
	for _, mbID := range req.TargetMailboxIDs {
		ct, err := tx.Exec(r.Context(), `
			INSERT INTO mailbox_alias_targets (alias_id, mailbox_id)
			SELECT $1, id FROM mailboxes WHERE id = $2 AND tenant_id = $3
			ON CONFLICT DO NOTHING`,
			aliasID, mbID, id.TenantID)
		if err != nil {
			httpx.Internal(w, r)
			return
		}
		if ct.RowsAffected() == 0 {
			httpx.Error(w, r, http.StatusNotFound, "MAILBOX_NOT_FOUND", "Target mailbox not found")
			return
		}
	}
	if err := tx.Commit(r.Context()); err != nil {
		httpx.Internal(w, r)
		return
	}
	h.Audit.Record(r.Context(), audit.Entry{
		TenantID: id.TenantID, ActorUserID: id.UserID, Action: "alias.create",
		ResourceType: "alias", ResourceID: aliasID,
		Detail: map[string]any{"address": address, "targets": len(req.TargetMailboxIDs)},
	})
	httpx.JSON(w, http.StatusCreated, map[string]string{"id": aliasID, "address": address})
}

type patchRequest struct {
	Status *string `json:"status"`
}

// Patch toggles alias status (active/inactive).
func (h *Handlers) Patch(w http.ResponseWriter, r *http.Request) {
	id := auth.IdentityFrom(r.Context())
	aliasID := chi.URLParam(r, "id")
	var req patchRequest
	if err := httpx.Decode(w, r, &req); err != nil {
		httpx.Error(w, r, http.StatusBadRequest, "INVALID_BODY", err.Error())
		return
	}
	if req.Status == nil || (*req.Status != "active" && *req.Status != "inactive") {
		httpx.Error(w, r, http.StatusBadRequest, "INVALID_STATUS", "status must be active or inactive")
		return
	}
	query := `UPDATE mailbox_aliases SET status = $1 WHERE id = $2 AND tenant_id = $3`
	args := []any{*req.Status, aliasID, id.TenantID}
	if !id.TenantWide() {
		query += ` AND organization_id = $4`
		args = append(args, id.OrganizationID)
	}
	ct, err := h.Pool.Exec(r.Context(), query, args...)
	if err != nil {
		httpx.Internal(w, r)
		return
	}
	if ct.RowsAffected() == 0 {
		httpx.Error(w, r, http.StatusNotFound, "ALIAS_NOT_FOUND", "Alias not found")
		return
	}
	h.Audit.Record(r.Context(), audit.Entry{
		TenantID: id.TenantID, ActorUserID: id.UserID, Action: "alias.status",
		ResourceType: "alias", ResourceID: aliasID, Detail: map[string]any{"status": *req.Status},
	})
	httpx.JSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// Delete removes an alias.
func (h *Handlers) Delete(w http.ResponseWriter, r *http.Request) {
	id := auth.IdentityFrom(r.Context())
	aliasID := chi.URLParam(r, "id")
	query := `DELETE FROM mailbox_aliases WHERE id = $1 AND tenant_id = $2`
	args := []any{aliasID, id.TenantID}
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
		httpx.Error(w, r, http.StatusNotFound, "ALIAS_NOT_FOUND", "Alias not found")
		return
	}
	h.Audit.Record(r.Context(), audit.Entry{
		TenantID: id.TenantID, ActorUserID: id.UserID, Action: "alias.delete",
		ResourceType: "alias", ResourceID: aliasID,
	})
	httpx.JSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
