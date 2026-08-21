// Package webmail serves the authenticated user's mailbox from the
// authoritative mail store (ADR-003): folders, listings, messages, flags
// and drafts all speak JMAP through internal/mailservice, so webmail, IMAP
// and JMAP clients always see the same state. Sending still enters the
// control-plane acceptance pipeline (validation, policy, events).
package webmail

import (
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/yelnurq/email-server/internal/auth"
	"github.com/yelnurq/email-server/internal/httpx"
	"github.com/yelnurq/email-server/internal/mailaddr"
	"github.com/yelnurq/email-server/internal/mailservice"
	"github.com/yelnurq/email-server/internal/messages"
)

// Handlers is the webmail HTTP surface.
type Handlers struct {
	Pool *pgxpool.Pool
	Mail *mailservice.Service
	// Accept is the control-plane acceptance service (shared with the Email
	// API); delivery into the store is the worker's job.
	Accept *messages.Service
	Log    *slog.Logger
}

func (h *Handlers) senderMailbox(w http.ResponseWriter, r *http.Request) (*auth.Identity, *messages.SenderMailbox, bool) {
	id := auth.IdentityFrom(r.Context())
	mb, err := h.Accept.MailboxForUser(r.Context(), id.TenantID, id.UserID)
	if errors.Is(err, messages.ErrNoMailbox) {
		httpx.Error(w, r, http.StatusNotFound, "NO_MAILBOX", "You have no active mailbox")
		return nil, nil, false
	}
	if err != nil {
		h.Log.Error("mailbox lookup failed", slog.String("error", err.Error()))
		httpx.Internal(w, r)
		return nil, nil, false
	}
	return id, mb, true
}

// storeError maps mail-store failures to stable API errors.
func (h *Handlers) storeError(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, mailservice.ErrUnavailable) {
		httpx.Error(w, r, http.StatusServiceUnavailable, "MAIL_SERVICE_UNAVAILABLE",
			"The mail service is temporarily unavailable")
		return
	}
	h.Log.Error("mail store operation failed", slog.String("error", err.Error()))
	httpx.Internal(w, r)
}

// Summary returns the caller's mailbox and folder counters.
func (h *Handlers) Summary(w http.ResponseWriter, r *http.Request) {
	_, mb, ok := h.senderMailbox(w, r)
	if !ok {
		return
	}
	folders, err := h.Mail.Summary(r.Context(), mb.Address)
	if err != nil {
		h.storeError(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]any{
		"mailbox": map[string]string{"id": mb.ID, "address": mb.Address},
		"folders": folders,
	})
}

// List returns one folder page (?folder=inbox&q=&limit=&offset=).
func (h *Handlers) List(w http.ResponseWriter, r *http.Request) {
	_, mb, ok := h.senderMailbox(w, r)
	if !ok {
		return
	}
	folderType := r.URL.Query().Get("folder")
	if folderType == "" {
		folderType = "inbox"
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	if offset < 0 {
		offset = 0
	}
	items, total, err := h.Mail.List(r.Context(), mb.Address, mailservice.ListQuery{
		FolderType:     folderType,
		Text:           strings.TrimSpace(r.URL.Query().Get("q")),
		UnreadOnly:     r.URL.Query().Get("unread") == "1",
		StarredOnly:    r.URL.Query().Get("starred") == "1",
		HasAttachments: r.URL.Query().Get("attachments") == "1",
		Limit:          limit,
		Offset:         offset,
	})
	if err != nil {
		h.storeError(w, r, err)
		return
	}
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	httpx.JSON(w, http.StatusOK, map[string]any{
		"messages": items, "total": total, "limit": limit, "offset": offset,
	})
}

// Get returns one message with bodies, attachments and thread context.
func (h *Handlers) Get(w http.ResponseWriter, r *http.Request) {
	_, mb, ok := h.senderMailbox(w, r)
	if !ok {
		return
	}
	msg, err := h.Mail.GetMessage(r.Context(), mb.Address, chi.URLParam(r, "id"))
	if err != nil {
		h.storeError(w, r, err)
		return
	}
	if msg == nil {
		httpx.Error(w, r, http.StatusNotFound, "MESSAGE_NOT_FOUND", "Message not found")
		return
	}
	// Bcc recipients are only visible to the sender (JMAP stores them only
	// in the sender's copy, but filter defensively anyway).
	if !strings.EqualFold(msg.From, mb.Address) {
		filtered := msg.Recipients[:0]
		for _, rc := range msg.Recipients {
			if rc.Kind != "bcc" {
				filtered = append(filtered, rc)
			}
		}
		msg.Recipients = filtered
	}
	httpx.JSON(w, http.StatusOK, msg)
}

type patchRequest struct {
	IsRead     *bool   `json:"is_read"`
	IsStarred  *bool   `json:"is_starred"`
	FolderType *string `json:"folder"`
}

// Patch updates message state (read/star/move) in the mail store.
func (h *Handlers) Patch(w http.ResponseWriter, r *http.Request) {
	_, mb, ok := h.senderMailbox(w, r)
	if !ok {
		return
	}
	mmID := chi.URLParam(r, "id")
	var req patchRequest
	if err := httpx.Decode(w, r, &req); err != nil {
		httpx.Error(w, r, http.StatusBadRequest, "INVALID_BODY", err.Error())
		return
	}
	if req.IsRead != nil || req.IsStarred != nil {
		if err := h.Mail.SetFlags(r.Context(), mb.Address, mmID, req.IsRead, req.IsStarred); err != nil {
			h.storeError(w, r, err)
			return
		}
	}
	if req.FolderType != nil {
		if err := h.Mail.Move(r.Context(), mb.Address, mmID, *req.FolderType); err != nil {
			if errors.Is(err, mailservice.ErrUnavailable) {
				h.storeError(w, r, err)
				return
			}
			httpx.Error(w, r, http.StatusBadRequest, "INVALID_FOLDER", "Unknown target folder")
			return
		}
	}
	httpx.JSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// Delete moves a message to Trash, or removes it permanently when it is
// already in Trash (or is a draft).
func (h *Handlers) Delete(w http.ResponseWriter, r *http.Request) {
	_, mb, ok := h.senderMailbox(w, r)
	if !ok {
		return
	}
	mmID := chi.URLParam(r, "id")
	msg, err := h.Mail.GetMessage(r.Context(), mb.Address, mmID)
	if err != nil {
		h.storeError(w, r, err)
		return
	}
	if msg == nil {
		httpx.Error(w, r, http.StatusNotFound, "MESSAGE_NOT_FOUND", "Message not found")
		return
	}
	if msg.Folder == "trash" || msg.Folder == "drafts" {
		err = h.Mail.Destroy(r.Context(), mb.Address, mmID)
	} else {
		err = h.Mail.Move(r.Context(), mb.Address, mmID, "trash")
	}
	if err != nil {
		h.storeError(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// Blob streams one attachment of a stored message from the caller's own
// mailbox (?name=&type= carry the display metadata).
func (h *Handlers) Blob(w http.ResponseWriter, r *http.Request) {
	_, mb, ok := h.senderMailbox(w, r)
	if !ok {
		return
	}
	blobID := chi.URLParam(r, "blobID")
	name := r.URL.Query().Get("name")
	ctype := r.URL.Query().Get("type")
	info, size, err := h.Mail.OpenBlob(r.Context(), mb.Address, blobID, name, ctype)
	if err != nil {
		h.storeError(w, r, err)
		return
	}
	defer info.Close()
	if ctype == "" {
		ctype = "application/octet-stream"
	}
	w.Header().Set("Content-Type", ctype)
	if name != "" {
		w.Header().Set("Content-Disposition", `attachment; filename="`+strings.ReplaceAll(name, `"`, "")+`"`)
	}
	if size > 0 {
		w.Header().Set("Content-Length", strconv.FormatInt(size, 10))
	}
	_, _ = io.Copy(w, info)
}

type sendRequest struct {
	To            []string `json:"to"`
	Cc            []string `json:"cc"`
	Bcc           []string `json:"bcc"`
	Subject       string   `json:"subject"`
	Text          string   `json:"text"`
	InReplyTo     string   `json:"in_reply_to"`
	AttachmentIDs []string `json:"attachment_ids"`
}

// Send accepts a message for delivery (202 + public id). in_reply_to may be
// either a control-plane public id (msg_…) or a store message id — the
// acceptance pipeline resolves threading by public id, so a store id is
// mapped through its RFC Message-ID first.
func (h *Handlers) Send(w http.ResponseWriter, r *http.Request) {
	id, mb, ok := h.senderMailbox(w, r)
	if !ok {
		return
	}
	var req sendRequest
	if err := httpx.Decode(w, r, &req); err != nil {
		httpx.Error(w, r, http.StatusBadRequest, "INVALID_BODY", err.Error())
		return
	}
	inReplyTo := h.resolveReplyTarget(r, mb.Address, req.InReplyTo)
	publicID, err := h.Accept.Accept(r.Context(), id.TenantID, mb, id.DisplayName, messages.SendInput{
		To: req.To, Cc: req.Cc, Bcc: req.Bcc,
		Subject: req.Subject, Text: req.Text, InReplyTo: inReplyTo,
		AttachmentIDs: req.AttachmentIDs, SenderUserID: id.UserID,
	})
	var vErr *messages.ValidationError
	if errors.As(err, &vErr) {
		httpx.Error(w, r, http.StatusBadRequest, "INVALID_MESSAGE", vErr.Msg)
		return
	}
	if err != nil {
		h.Log.Error("message accept failed", slog.String("error", err.Error()))
		httpx.Internal(w, r)
		return
	}
	httpx.JSON(w, http.StatusAccepted, map[string]string{"message_id": publicID})
}

// resolveReplyTarget maps a store message id to the control-plane public id
// via the RFC Message-ID so threading headers survive the bridge. Public
// ids (msg_…) pass through untouched; unknown ids drop threading rather
// than failing the send.
func (h *Handlers) resolveReplyTarget(r *http.Request, account, replyTo string) string {
	if replyTo == "" || strings.HasPrefix(replyTo, "msg_") {
		return replyTo
	}
	msg, err := h.Mail.GetMessage(r.Context(), account, replyTo)
	if err != nil || msg == nil || msg.MessageID == "" {
		return ""
	}
	// Both sides are canonical (bare, domain lowercased): the store id is
	// normalized at the JMAP boundary, the column by migration 00019.
	var publicID string
	err = h.Pool.QueryRow(r.Context(),
		`SELECT public_id FROM messages WHERE rfc_message_id = $1`,
		mailaddr.NormalizeMessageID(msg.MessageID)).Scan(&publicID)
	if errors.Is(err, pgx.ErrNoRows) || err != nil {
		return ""
	}
	return publicID
}

// Events returns the delivery timeline for a message the caller sent. The
// store message is joined to the control-plane trace by RFC Message-ID.
func (h *Handlers) Events(w http.ResponseWriter, r *http.Request) {
	_, mb, ok := h.senderMailbox(w, r)
	if !ok {
		return
	}
	msg, err := h.Mail.GetMessage(r.Context(), mb.Address, chi.URLParam(r, "id"))
	if err != nil {
		h.storeError(w, r, err)
		return
	}
	if msg == nil {
		httpx.Error(w, r, http.StatusNotFound, "MESSAGE_NOT_FOUND", "Message not found")
		return
	}
	empty := map[string]any{"status": "", "recipients": []any{}, "events": []any{}}
	if !strings.EqualFold(msg.From, mb.Address) || msg.MessageID == "" {
		httpx.JSON(w, http.StatusOK, empty)
		return
	}
	var msgID, status string
	err = h.Pool.QueryRow(r.Context(),
		`SELECT id, status FROM messages WHERE rfc_message_id = $1`,
		mailaddr.NormalizeMessageID(msg.MessageID)).Scan(&msgID, &status)
	if errors.Is(err, pgx.ErrNoRows) {
		httpx.JSON(w, http.StatusOK, empty)
		return
	}
	if err != nil {
		httpx.Internal(w, r)
		return
	}

	type recipientStatus struct {
		Address string `json:"address"`
		Status  string `json:"status"`
		Error   string `json:"error,omitempty"`
	}
	recipients := []recipientStatus{}
	recRows, err := h.Pool.Query(r.Context(), `
		SELECT address::text, status, error FROM message_recipients
		WHERE message_id = $1 ORDER BY created_at`, msgID)
	if err != nil {
		httpx.Internal(w, r)
		return
	}
	defer recRows.Close()
	for recRows.Next() {
		var rs recipientStatus
		if err := recRows.Scan(&rs.Address, &rs.Status, &rs.Error); err != nil {
			httpx.Internal(w, r)
			return
		}
		recipients = append(recipients, rs)
	}

	type eventView struct {
		Type      string `json:"type"`
		CreatedAt string `json:"created_at"`
	}
	eventsOut := []eventView{}
	evRows, err := h.Pool.Query(r.Context(), `
		SELECT type, created_at::text FROM message_events
		WHERE message_id = $1 ORDER BY created_at, id`, msgID)
	if err != nil {
		httpx.Internal(w, r)
		return
	}
	defer evRows.Close()
	for evRows.Next() {
		var ev eventView
		if err := evRows.Scan(&ev.Type, &ev.CreatedAt); err != nil {
			httpx.Internal(w, r)
			return
		}
		eventsOut = append(eventsOut, ev)
	}
	httpx.JSON(w, http.StatusOK, map[string]any{
		"status": status, "recipients": recipients, "events": eventsOut,
	})
}
