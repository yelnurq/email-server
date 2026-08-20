// Package groups manages mail groups: one address fanning out to member
// mailboxes. Members are mailboxes only (no nested groups/aliases), so
// routing loops cannot occur.
package groups

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

type Group struct {
	ID             string   `json:"id"`
	OrganizationID string   `json:"organization_id"`
	Address        string   `json:"address"`
	Name           string   `json:"name"`
	Status         string   `json:"status"`
	InternalOnly   bool     `json:"internal_only"`
	Members        []string `json:"members"` // member mailbox addresses
	CreatedAt      string   `json:"created_at"`
}

// List returns the tenant's groups with member addresses.
func (h *Handlers) List(w http.ResponseWriter, r *http.Request) {
	id := auth.IdentityFrom(r.Context())
	query := `
		SELECT g.id, g.organization_id, g.address::text, g.name, g.status, g.internal_only,
		       g.created_at::text,
		       COALESCE(array_agg(m.address::text ORDER BY m.address) FILTER (WHERE m.id IS NOT NULL), '{}')
		FROM mail_groups g
		LEFT JOIN mail_group_members gm ON gm.group_id = g.id
		LEFT JOIN mailboxes m ON m.id = gm.mailbox_id
		WHERE g.tenant_id = $1`
	args := []any{id.TenantID}
	if !id.TenantWide() {
		query += ` AND g.organization_id = $2`
		args = append(args, id.OrganizationID)
	}
	query += ` GROUP BY g.id ORDER BY g.created_at`
	rows, err := h.Pool.Query(r.Context(), query, args...)
	if err != nil {
		h.Log.Error("group list failed", slog.String("error", err.Error()))
		httpx.Internal(w, r)
		return
	}
	defer rows.Close()
	out := []Group{}
	for rows.Next() {
		var g Group
		if err := rows.Scan(&g.ID, &g.OrganizationID, &g.Address, &g.Name, &g.Status,
			&g.InternalOnly, &g.CreatedAt, &g.Members); err != nil {
			httpx.Internal(w, r)
			return
		}
		out = append(out, g)
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"groups": out})
}

type createRequest struct {
	DomainID         string   `json:"domain_id"`
	LocalPart        string   `json:"local_part"`
	Name             string   `json:"name"`
	InternalOnly     bool     `json:"internal_only"`
	MemberMailboxIDs []string `json:"member_mailbox_ids"`
}

// Create provisions a group with initial members.
func (h *Handlers) Create(w http.ResponseWriter, r *http.Request) {
	id := auth.IdentityFrom(r.Context())
	var req createRequest
	if err := httpx.Decode(w, r, &req); err != nil {
		httpx.Error(w, r, http.StatusBadRequest, "INVALID_BODY", err.Error())
		return
	}
	if len(req.MemberMailboxIDs) == 0 || len(req.MemberMailboxIDs) > 200 {
		httpx.Error(w, r, http.StatusBadRequest, "INVALID_MEMBERS", "Provide 1-200 member mailboxes")
		return
	}
	local, err := mailaddr.NormalizeLocalPart(req.LocalPart)
	if err != nil {
		httpx.Error(w, r, http.StatusBadRequest, "INVALID_LOCAL_PART", "Invalid group local part")
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

	var groupID string
	if err := tx.QueryRow(r.Context(), `
		INSERT INTO mail_groups (tenant_id, organization_id, domain_id, local_part, address, name, internal_only)
		VALUES ($1, $2, $3, $4, $5, $6, $7) RETURNING id`,
		id.TenantID, domainOrgID, req.DomainID, local, address, req.Name, req.InternalOnly).Scan(&groupID); err != nil {
		httpx.Internal(w, r)
		return
	}
	for _, mbID := range req.MemberMailboxIDs {
		ct, err := tx.Exec(r.Context(), `
			INSERT INTO mail_group_members (group_id, mailbox_id)
			SELECT $1, id FROM mailboxes WHERE id = $2 AND tenant_id = $3
			ON CONFLICT DO NOTHING`,
			groupID, mbID, id.TenantID)
		if err != nil {
			httpx.Internal(w, r)
			return
		}
		if ct.RowsAffected() == 0 {
			httpx.Error(w, r, http.StatusNotFound, "MAILBOX_NOT_FOUND", "Member mailbox not found")
			return
		}
	}
	if err := tx.Commit(r.Context()); err != nil {
		httpx.Internal(w, r)
		return
	}
	h.Audit.Record(r.Context(), audit.Entry{
		TenantID: id.TenantID, ActorUserID: id.UserID, Action: "group.create",
		ResourceType: "group", ResourceID: groupID,
		Detail: map[string]any{"address": address, "members": len(req.MemberMailboxIDs)},
	})
	httpx.JSON(w, http.StatusCreated, map[string]string{"id": groupID, "address": address})
}

type membersRequest struct {
	Add    []string `json:"add"`
	Remove []string `json:"remove"`
}

// UpdateMembers adds/removes group members.
func (h *Handlers) UpdateMembers(w http.ResponseWriter, r *http.Request) {
	id := auth.IdentityFrom(r.Context())
	groupID := chi.URLParam(r, "id")
	var req membersRequest
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

	ownQuery := `SELECT organization_id FROM mail_groups WHERE id = $1 AND tenant_id = $2`
	var orgID string
	if err := tx.QueryRow(r.Context(), ownQuery, groupID, id.TenantID).Scan(&orgID); err != nil {
		httpx.Error(w, r, http.StatusNotFound, "GROUP_NOT_FOUND", "Group not found")
		return
	}
	if !id.TenantWide() && orgID != id.OrganizationID {
		httpx.Error(w, r, http.StatusForbidden, "FORBIDDEN", "Cannot manage another organization")
		return
	}

	for _, mbID := range req.Add {
		if _, err := tx.Exec(r.Context(), `
			INSERT INTO mail_group_members (group_id, mailbox_id)
			SELECT $1, id FROM mailboxes WHERE id = $2 AND tenant_id = $3
			ON CONFLICT DO NOTHING`, groupID, mbID, id.TenantID); err != nil {
			httpx.Internal(w, r)
			return
		}
	}
	for _, mbID := range req.Remove {
		if _, err := tx.Exec(r.Context(),
			`DELETE FROM mail_group_members WHERE group_id = $1 AND mailbox_id = $2`,
			groupID, mbID); err != nil {
			httpx.Internal(w, r)
			return
		}
	}
	if err := tx.Commit(r.Context()); err != nil {
		httpx.Internal(w, r)
		return
	}
	h.Audit.Record(r.Context(), audit.Entry{
		TenantID: id.TenantID, ActorUserID: id.UserID, Action: "group.members",
		ResourceType: "group", ResourceID: groupID,
		Detail: map[string]any{"added": len(req.Add), "removed": len(req.Remove)},
	})
	httpx.JSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// Delete removes a group.
func (h *Handlers) Delete(w http.ResponseWriter, r *http.Request) {
	id := auth.IdentityFrom(r.Context())
	groupID := chi.URLParam(r, "id")
	query := `DELETE FROM mail_groups WHERE id = $1 AND tenant_id = $2`
	args := []any{groupID, id.TenantID}
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
		httpx.Error(w, r, http.StatusNotFound, "GROUP_NOT_FOUND", "Group not found")
		return
	}
	h.Audit.Record(r.Context(), audit.Entry{
		TenantID: id.TenantID, ActorUserID: id.UserID, Action: "group.delete",
		ResourceType: "group", ResourceID: groupID,
	})
	httpx.JSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
