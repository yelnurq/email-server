// Package trace implements the admin Message Trace tool: search accepted
// messages and reconstruct one message's lifecycle from its recorded events,
// recipients and delivery outcomes. Everything shown is read from real
// message_events / message_recipients rows — nothing is synthesized.
package trace

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/yelnurq/email-server/internal/auth"
	"github.com/yelnurq/email-server/internal/httpx"
)

type Handlers struct {
	Pool *pgxpool.Pool
	Log  *slog.Logger
}

type listItem struct {
	PublicID    string `json:"message_id"`
	From        string `json:"from"`
	FromDisplay string `json:"from_display"`
	Subject     string `json:"subject"`
	Status      string `json:"status"`
	Recipients  int    `json:"recipients"`
	SizeBytes   int64  `json:"size_bytes"`
	CreatedAt   string `json:"created_at"`
}

// List searches messages by public id, sender, recipient or subject.
// GET /admin/messages?q=&status=&limit=&offset=
func (h *Handlers) List(w http.ResponseWriter, r *http.Request) {
	id := auth.IdentityFrom(r.Context())
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	if offset < 0 {
		offset = 0
	}
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	status := strings.TrimSpace(r.URL.Query().Get("status"))

	query := `
		SELECT m.public_id, m.from_address::text, m.from_display, m.subject, m.status,
		       (SELECT count(*) FROM message_recipients mr WHERE mr.message_id = m.id),
		       m.size_bytes, m.created_at::text, count(*) OVER() AS total
		FROM messages m
		WHERE m.tenant_id = $1 AND m.status <> 'draft'`
	args := []any{id.TenantID}
	if !id.TenantWide() {
		args = append(args, id.OrganizationID)
		query += ` AND m.organization_id = $` + strconv.Itoa(len(args))
	}
	if q != "" {
		args = append(args, q, "%"+q+"%")
		n, l := strconv.Itoa(len(args)-1), strconv.Itoa(len(args))
		query += ` AND (m.public_id = $` + n + `
			OR m.from_address::text ILIKE $` + l + `
			OR m.subject ILIKE $` + l + `
			OR EXISTS (SELECT 1 FROM message_recipients mr
			           WHERE mr.message_id = m.id AND mr.address::text ILIKE $` + l + `))`
	}
	if status != "" {
		args = append(args, status)
		query += ` AND m.status = $` + strconv.Itoa(len(args))
	}
	query += ` ORDER BY m.created_at DESC LIMIT ` + strconv.Itoa(limit) + ` OFFSET ` + strconv.Itoa(offset)

	rows, err := h.Pool.Query(r.Context(), query, args...)
	if err != nil {
		h.Log.Error("trace list failed", slog.String("error", err.Error()))
		httpx.Internal(w, r)
		return
	}
	defer rows.Close()
	items, total := []listItem{}, 0
	for rows.Next() {
		var it listItem
		if err := rows.Scan(&it.PublicID, &it.From, &it.FromDisplay, &it.Subject, &it.Status,
			&it.Recipients, &it.SizeBytes, &it.CreatedAt, &total); err != nil {
			httpx.Internal(w, r)
			return
		}
		items = append(items, it)
	}
	httpx.JSON(w, http.StatusOK, map[string]any{
		"messages": items, "total": total, "limit": limit, "offset": offset,
	})
}

type recipientTrace struct {
	Kind    string `json:"kind"`
	Address string `json:"address"`
	Status  string `json:"status"`
	Error   string `json:"error,omitempty"`
	Mailbox string `json:"mailbox,omitempty"`
}

type eventTrace struct {
	Type      string          `json:"type"`
	Detail    json.RawMessage `json:"detail"`
	CreatedAt string          `json:"created_at"`
}

// Get reconstructs one message's trace.
// GET /admin/messages/{publicID}/trace
func (h *Handlers) Get(w http.ResponseWriter, r *http.Request) {
	id := auth.IdentityFrom(r.Context())
	publicID := chi.URLParam(r, "publicID")

	var msgID, from, fromDisplay, subject, status, createdAt string
	var sentAt *string
	var size int64
	var hasAttachments bool
	query := `
		SELECT m.id, m.from_address::text, m.from_display, m.subject, m.status,
		       m.size_bytes, m.has_attachments, m.created_at::text, m.sent_at::text
		FROM messages m
		WHERE m.public_id = $1 AND m.tenant_id = $2`
	args := []any{publicID, id.TenantID}
	if !id.TenantWide() {
		query += ` AND m.organization_id = $3`
		args = append(args, id.OrganizationID)
	}
	err := h.Pool.QueryRow(r.Context(), query, args...).
		Scan(&msgID, &from, &fromDisplay, &subject, &status, &size, &hasAttachments, &createdAt, &sentAt)
	if errors.Is(err, pgx.ErrNoRows) {
		httpx.Error(w, r, http.StatusNotFound, "MESSAGE_NOT_FOUND", "Message not found")
		return
	}
	if err != nil {
		httpx.Internal(w, r)
		return
	}

	recipients := []recipientTrace{}
	recRows, err := h.Pool.Query(r.Context(), `
		SELECT mr.kind, mr.address::text, mr.status, mr.error,
		       COALESCE(mb.address::text, '')
		FROM message_recipients mr
		LEFT JOIN mailboxes mb ON mb.id = mr.mailbox_id
		WHERE mr.message_id = $1 ORDER BY mr.created_at`, msgID)
	if err != nil {
		httpx.Internal(w, r)
		return
	}
	defer recRows.Close()
	for recRows.Next() {
		var rt recipientTrace
		if err := recRows.Scan(&rt.Kind, &rt.Address, &rt.Status, &rt.Error, &rt.Mailbox); err != nil {
			httpx.Internal(w, r)
			return
		}
		recipients = append(recipients, rt)
	}

	events := []eventTrace{}
	evRows, err := h.Pool.Query(r.Context(), `
		SELECT type, detail, created_at::text FROM message_events
		WHERE message_id = $1 ORDER BY created_at, id`, msgID)
	if err != nil {
		httpx.Internal(w, r)
		return
	}
	defer evRows.Close()
	for evRows.Next() {
		var et eventTrace
		if err := evRows.Scan(&et.Type, &et.Detail, &et.CreatedAt); err != nil {
			httpx.Internal(w, r)
			return
		}
		events = append(events, et)
	}

	httpx.JSON(w, http.StatusOK, map[string]any{
		"message_id":      publicID,
		"from":            from,
		"from_display":    fromDisplay,
		"subject":         subject,
		"status":          status,
		"size_bytes":      size,
		"has_attachments": hasAttachments,
		"created_at":      createdAt,
		"sent_at":         sentAt,
		"recipients":      recipients,
		"events":          events,
	})
}
