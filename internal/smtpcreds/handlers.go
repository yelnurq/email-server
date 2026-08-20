// Package smtpcreds manages SMTP submission credentials: scoped to one
// mailbox, hashed at rest, shown exactly once, revocable and audited. The
// SMTP listener that consumes them ships with the mail-core phase.
package smtpcreds

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base32"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
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

type credView struct {
	ID             string  `json:"id"`
	OrganizationID string  `json:"organization_id"`
	MailboxAddress string  `json:"mailbox_address"`
	Username       string  `json:"username"`
	Status         string  `json:"status"`
	LastUsedAt     *string `json:"last_used_at,omitempty"`
	CreatedAt      string  `json:"created_at"`
}

// List returns the tenant's SMTP credentials (never secrets).
func (h *Handlers) List(w http.ResponseWriter, r *http.Request) {
	id := auth.IdentityFrom(r.Context())
	query := `
		SELECT c.id, c.organization_id, m.address::text, c.username::text,
		       c.status, c.last_used_at::text, c.created_at::text
		FROM smtp_credentials c
		JOIN mailboxes m ON m.id = c.mailbox_id
		WHERE c.tenant_id = $1`
	args := []any{id.TenantID}
	if !id.TenantWide() {
		query += ` AND c.organization_id = $2`
		args = append(args, id.OrganizationID)
	}
	query += ` ORDER BY c.created_at DESC`
	rows, err := h.Pool.Query(r.Context(), query, args...)
	if err != nil {
		h.Log.Error("smtp cred list failed", slog.String("error", err.Error()))
		httpx.Internal(w, r)
		return
	}
	defer rows.Close()
	out := []credView{}
	for rows.Next() {
		var v credView
		if err := rows.Scan(&v.ID, &v.OrganizationID, &v.MailboxAddress, &v.Username,
			&v.Status, &v.LastUsedAt, &v.CreatedAt); err != nil {
			httpx.Internal(w, r)
			return
		}
		out = append(out, v)
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"smtp_credentials": out})
}

type createRequest struct {
	MailboxID string `json:"mailbox_id"`
}

// Create issues a credential for a mailbox; the password appears only once.
func (h *Handlers) Create(w http.ResponseWriter, r *http.Request) {
	id := auth.IdentityFrom(r.Context())
	var req createRequest
	if err := httpx.Decode(w, r, &req); err != nil {
		httpx.Error(w, r, http.StatusBadRequest, "INVALID_BODY", err.Error())
		return
	}

	var mailboxOrgID, mailboxAddress string
	err := h.Pool.QueryRow(r.Context(), `
		SELECT organization_id, address::text FROM mailboxes
		WHERE id = $1 AND tenant_id = $2 AND status = 'active'`,
		req.MailboxID, id.TenantID).Scan(&mailboxOrgID, &mailboxAddress)
	if errors.Is(err, pgx.ErrNoRows) {
		httpx.Error(w, r, http.StatusNotFound, "MAILBOX_NOT_FOUND", "Mailbox not found")
		return
	}
	if err != nil {
		httpx.Internal(w, r)
		return
	}
	if !id.TenantWide() && mailboxOrgID != id.OrganizationID {
		httpx.Error(w, r, http.StatusForbidden, "FORBIDDEN", "Cannot manage another organization")
		return
	}

	var userBuf, passBuf [10]byte
	_, _ = rand.Read(userBuf[:])
	_, _ = rand.Read(passBuf[:])
	enc := base32.StdEncoding.WithPadding(base32.NoPadding)
	username := "smtp-" + strings.ToLower(enc.EncodeToString(userBuf[:]))
	password := "smtppw_" + strings.ToLower(enc.EncodeToString(passBuf[:]))
	hash := sha256.Sum256([]byte(password))

	var credID string
	if err := h.Pool.QueryRow(r.Context(), `
		INSERT INTO smtp_credentials (tenant_id, organization_id, mailbox_id, username, secret_hash, created_by)
		VALUES ($1, $2, $3, $4, $5, $6) RETURNING id`,
		id.TenantID, mailboxOrgID, req.MailboxID, username, hash[:], id.UserID).Scan(&credID); err != nil {
		h.Log.Error("smtp cred create failed", slog.String("error", err.Error()))
		httpx.Internal(w, r)
		return
	}
	h.Audit.Record(r.Context(), audit.Entry{
		TenantID: id.TenantID, ActorUserID: id.UserID, Action: "smtpcred.create",
		ResourceType: "smtp_credential", ResourceID: credID,
		Detail: map[string]any{"mailbox": mailboxAddress, "username": username},
	})
	httpx.JSON(w, http.StatusCreated, map[string]string{
		"id":       credID,
		"mailbox":  mailboxAddress,
		"username": username,
		// Shown exactly once; never retrievable again.
		"password": password,
	})
}

// Revoke disables a credential permanently.
func (h *Handlers) Revoke(w http.ResponseWriter, r *http.Request) {
	id := auth.IdentityFrom(r.Context())
	credID := chi.URLParam(r, "id")
	query := `UPDATE smtp_credentials SET status = 'revoked' WHERE id = $1 AND tenant_id = $2`
	args := []any{credID, id.TenantID}
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
		httpx.Error(w, r, http.StatusNotFound, "CREDENTIAL_NOT_FOUND", "SMTP credential not found")
		return
	}
	h.Audit.Record(r.Context(), audit.Entry{
		TenantID: id.TenantID, ActorUserID: id.UserID, Action: "smtpcred.revoke",
		ResourceType: "smtp_credential", ResourceID: credID,
	})
	httpx.JSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
