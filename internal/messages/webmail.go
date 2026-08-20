package messages

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"

	"github.com/yelnurq/email-server/internal/auth"
	"github.com/yelnurq/email-server/internal/httpx"
)

// WebmailHandlers serves the authenticated user's mailbox.
type WebmailHandlers struct {
	Svc *Service
	Log *slog.Logger
}

func (h *WebmailHandlers) senderMailbox(w http.ResponseWriter, r *http.Request) (*auth.Identity, *SenderMailbox, bool) {
	id := auth.IdentityFrom(r.Context())
	mb, err := h.Svc.MailboxForUser(r.Context(), id.TenantID, id.UserID)
	if errors.Is(err, ErrNoMailbox) {
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

// Summary returns the caller's mailbox and folder counters.
func (h *WebmailHandlers) Summary(w http.ResponseWriter, r *http.Request) {
	_, mb, ok := h.senderMailbox(w, r)
	if !ok {
		return
	}
	rows, err := h.Svc.Pool.Query(r.Context(), `
		SELECT f.id, f.name, f.type,
		       count(mm.id) FILTER (WHERE NOT mm.is_read),
		       count(mm.id)
		FROM folders f
		LEFT JOIN mailbox_messages mm ON mm.folder_id = f.id
		WHERE f.mailbox_id = $1
		GROUP BY f.id, f.name, f.type
		ORDER BY array_position(ARRAY['inbox','sent','drafts','spam','trash','custom'], f.type), f.name`,
		mb.ID)
	if err != nil {
		httpx.Internal(w, r)
		return
	}
	defer rows.Close()
	type folder struct {
		ID     string `json:"id"`
		Name   string `json:"name"`
		Type   string `json:"type"`
		Unread int    `json:"unread"`
		Total  int    `json:"total"`
	}
	folders := []folder{}
	for rows.Next() {
		var f folder
		if err := rows.Scan(&f.ID, &f.Name, &f.Type, &f.Unread, &f.Total); err != nil {
			httpx.Internal(w, r)
			return
		}
		folders = append(folders, f)
	}
	httpx.JSON(w, http.StatusOK, map[string]any{
		"mailbox": map[string]string{"id": mb.ID, "address": mb.Address},
		"folders": folders,
	})
}

type listItem struct {
	ID              string `json:"id"` // mailbox_message id
	MessagePublicID string `json:"message_id"`
	From            string `json:"from"`
	FromDisplay     string `json:"from_display"`
	Subject         string `json:"subject"`
	Snippet         string `json:"snippet"`
	Date            string `json:"date"`
	IsRead          bool   `json:"is_read"`
	IsStarred       bool   `json:"is_starred"`
	HasAttachments  bool   `json:"has_attachments"`
}

// List returns messages of one folder (?folder=inbox) with pagination and an
// optional search query over sender/subject/body.
func (h *WebmailHandlers) List(w http.ResponseWriter, r *http.Request) {
	_, mb, ok := h.senderMailbox(w, r)
	if !ok {
		return
	}
	folderType := r.URL.Query().Get("folder")
	if folderType == "" {
		folderType = "inbox"
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	if offset < 0 {
		offset = 0
	}
	q := strings.TrimSpace(r.URL.Query().Get("q"))

	query := `
		SELECT mm.id, m.public_id, m.from_address::text, m.from_display,
		       m.subject, m.snippet, mm.created_at::text, mm.is_read, mm.is_starred,
		       m.has_attachments, count(*) OVER() AS total
		FROM mailbox_messages mm
		JOIN folders f ON f.id = mm.folder_id
		JOIN messages m ON m.id = mm.message_id
		WHERE mm.mailbox_id = $1 AND f.type = $2`
	args := []any{mb.ID, folderType}
	if q != "" {
		query += ` AND (m.subject ILIKE $3 OR m.from_address::text ILIKE $3 OR m.body_text ILIKE $3)`
		args = append(args, "%"+q+"%")
	}
	query += ` ORDER BY mm.created_at DESC LIMIT ` + strconv.Itoa(limit) + ` OFFSET ` + strconv.Itoa(offset)

	rows, err := h.Svc.Pool.Query(r.Context(), query, args...)
	if err != nil {
		h.Log.Error("message list failed", slog.String("error", err.Error()))
		httpx.Internal(w, r)
		return
	}
	defer rows.Close()
	items := []listItem{}
	total := 0
	for rows.Next() {
		var it listItem
		if err := rows.Scan(&it.ID, &it.MessagePublicID, &it.From, &it.FromDisplay,
			&it.Subject, &it.Snippet, &it.Date, &it.IsRead, &it.IsStarred,
			&it.HasAttachments, &total); err != nil {
			httpx.Internal(w, r)
			return
		}
		items = append(items, it)
	}
	httpx.JSON(w, http.StatusOK, map[string]any{
		"messages": items, "total": total, "limit": limit, "offset": offset,
	})
}

type recipientView struct {
	Kind    string `json:"kind"`
	Address string `json:"address"`
}

// Get returns one message with recipients and thread context. Bcc recipients
// are only visible to the sender.
func (h *WebmailHandlers) Get(w http.ResponseWriter, r *http.Request) {
	_, mb, ok := h.senderMailbox(w, r)
	if !ok {
		return
	}
	mmID := chi.URLParam(r, "id")

	var msgID, publicID, threadID, from, fromDisplay, subject, bodyText, bodyHTML, date, folderType string
	var isRead, isStarred, hasAttachments bool
	err := h.Svc.Pool.QueryRow(r.Context(), `
		SELECT m.id, m.public_id, COALESCE(m.thread_id::text, ''), m.from_address::text,
		       m.from_display, m.subject, m.body_text, m.body_html, mm.created_at::text,
		       f.type, mm.is_read, mm.is_starred, m.has_attachments
		FROM mailbox_messages mm
		JOIN messages m ON m.id = mm.message_id
		JOIN folders f ON f.id = mm.folder_id
		WHERE mm.id = $1 AND mm.mailbox_id = $2`, mmID, mb.ID).
		Scan(&msgID, &publicID, &threadID, &from, &fromDisplay, &subject, &bodyText,
			&bodyHTML, &date, &folderType, &isRead, &isStarred, &hasAttachments)
	if errors.Is(err, pgx.ErrNoRows) {
		httpx.Error(w, r, http.StatusNotFound, "MESSAGE_NOT_FOUND", "Message not found")
		return
	}
	if err != nil {
		httpx.Internal(w, r)
		return
	}

	isSender := strings.EqualFold(from, mb.Address)
	recRows, err := h.Svc.Pool.Query(r.Context(), `
		SELECT kind, address::text FROM message_recipients
		WHERE message_id = $1 ORDER BY created_at`, msgID)
	if err != nil {
		httpx.Internal(w, r)
		return
	}
	defer recRows.Close()
	recipients := []recipientView{}
	for recRows.Next() {
		var rv recipientView
		if err := recRows.Scan(&rv.Kind, &rv.Address); err != nil {
			httpx.Internal(w, r)
			return
		}
		if rv.Kind == "bcc" && !isSender {
			continue
		}
		recipients = append(recipients, rv)
	}

	// Thread siblings the caller can access through their own mailbox.
	type threadItem struct {
		ID      string `json:"id"`
		Subject string `json:"subject"`
		From    string `json:"from"`
		Date    string `json:"date"`
	}
	thread := []threadItem{}
	if threadID != "" {
		tRows, err := h.Svc.Pool.Query(r.Context(), `
			SELECT mm.id, m.subject, m.from_address::text, mm.created_at::text
			FROM mailbox_messages mm
			JOIN messages m ON m.id = mm.message_id
			WHERE mm.mailbox_id = $1 AND m.thread_id = $2
			ORDER BY mm.created_at`, mb.ID, threadID)
		if err == nil {
			defer tRows.Close()
			for tRows.Next() {
				var ti threadItem
				if err := tRows.Scan(&ti.ID, &ti.Subject, &ti.From, &ti.Date); err == nil {
					thread = append(thread, ti)
				}
			}
		}
	}

	// Attachment metadata for the reader.
	type attachmentView struct {
		ID          string `json:"id"`
		Filename    string `json:"filename"`
		ContentType string `json:"content_type"`
		SizeBytes   int64  `json:"size_bytes"`
	}
	atts := []attachmentView{}
	if hasAttachments {
		aRows, err := h.Svc.Pool.Query(r.Context(), `
			SELECT public_id, filename, content_type, size_bytes
			FROM attachments WHERE message_id = $1 ORDER BY created_at`, msgID)
		if err == nil {
			defer aRows.Close()
			for aRows.Next() {
				var av attachmentView
				if err := aRows.Scan(&av.ID, &av.Filename, &av.ContentType, &av.SizeBytes); err == nil {
					atts = append(atts, av)
				}
			}
		}
	}

	// Opening a message marks it read.
	if !isRead {
		_, _ = h.Svc.Pool.Exec(r.Context(),
			`UPDATE mailbox_messages SET is_read = true WHERE id = $1`, mmID)
		isRead = true
	}

	httpx.JSON(w, http.StatusOK, map[string]any{
		"id":              mmID,
		"message_id":      publicID,
		"folder":          folderType,
		"from":            from,
		"from_display":    fromDisplay,
		"recipients":      recipients,
		"subject":         subject,
		"body_text":       bodyText,
		"body_html":       bodyHTML,
		"date":            date,
		"is_read":         isRead,
		"is_starred":      isStarred,
		"has_attachments": hasAttachments,
		"attachments":     atts,
		"thread":          thread,
	})
}

type patchRequest struct {
	IsRead     *bool   `json:"is_read"`
	IsStarred  *bool   `json:"is_starred"`
	FolderType *string `json:"folder"`
}

// Patch updates per-mailbox message state (read/star/move).
func (h *WebmailHandlers) Patch(w http.ResponseWriter, r *http.Request) {
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
	if req.IsRead != nil {
		if _, err := h.Svc.Pool.Exec(r.Context(), `
			UPDATE mailbox_messages SET is_read = $1 WHERE id = $2 AND mailbox_id = $3`,
			*req.IsRead, mmID, mb.ID); err != nil {
			httpx.Internal(w, r)
			return
		}
	}
	if req.IsStarred != nil {
		if _, err := h.Svc.Pool.Exec(r.Context(), `
			UPDATE mailbox_messages SET is_starred = $1 WHERE id = $2 AND mailbox_id = $3`,
			*req.IsStarred, mmID, mb.ID); err != nil {
			httpx.Internal(w, r)
			return
		}
	}
	if req.FolderType != nil {
		ct, err := h.Svc.Pool.Exec(r.Context(), `
			UPDATE mailbox_messages mm SET folder_id = f.id
			FROM folders f
			WHERE mm.id = $1 AND mm.mailbox_id = $2
			  AND f.mailbox_id = mm.mailbox_id AND f.type = $3`,
			mmID, mb.ID, *req.FolderType)
		if err != nil || ct.RowsAffected() == 0 {
			httpx.Error(w, r, http.StatusBadRequest, "INVALID_FOLDER", "Unknown target folder")
			return
		}
	}
	httpx.JSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// Delete moves a message to Trash, or removes the mailbox copy permanently
// when it is already in Trash. Drafts are deleted entirely.
func (h *WebmailHandlers) Delete(w http.ResponseWriter, r *http.Request) {
	_, mb, ok := h.senderMailbox(w, r)
	if !ok {
		return
	}
	mmID := chi.URLParam(r, "id")

	var folderType, msgID, msgStatus string
	err := h.Svc.Pool.QueryRow(r.Context(), `
		SELECT f.type, m.id, m.status
		FROM mailbox_messages mm
		JOIN folders f ON f.id = mm.folder_id
		JOIN messages m ON m.id = mm.message_id
		WHERE mm.id = $1 AND mm.mailbox_id = $2`, mmID, mb.ID).
		Scan(&folderType, &msgID, &msgStatus)
	if errors.Is(err, pgx.ErrNoRows) {
		httpx.Error(w, r, http.StatusNotFound, "MESSAGE_NOT_FOUND", "Message not found")
		return
	}
	if err != nil {
		httpx.Internal(w, r)
		return
	}

	switch {
	case msgStatus == statusDraft:
		// Deleting a draft removes the draft message itself.
		if _, err := h.Svc.Pool.Exec(r.Context(), `DELETE FROM messages WHERE id = $1`, msgID); err != nil {
			httpx.Internal(w, r)
			return
		}
	case folderType == "trash":
		if _, err := h.Svc.Pool.Exec(r.Context(),
			`DELETE FROM mailbox_messages WHERE id = $1 AND mailbox_id = $2`, mmID, mb.ID); err != nil {
			httpx.Internal(w, r)
			return
		}
	default:
		if _, err := h.Svc.Pool.Exec(r.Context(), `
			UPDATE mailbox_messages mm SET folder_id = f.id
			FROM folders f
			WHERE mm.id = $1 AND mm.mailbox_id = $2
			  AND f.mailbox_id = mm.mailbox_id AND f.type = 'trash'`, mmID, mb.ID); err != nil {
			httpx.Internal(w, r)
			return
		}
	}
	httpx.JSON(w, http.StatusOK, map[string]string{"status": "ok"})
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

// Send accepts a message for delivery and returns 202 with its public id.
func (h *WebmailHandlers) Send(w http.ResponseWriter, r *http.Request) {
	id, mb, ok := h.senderMailbox(w, r)
	if !ok {
		return
	}
	var req sendRequest
	if err := httpx.Decode(w, r, &req); err != nil {
		httpx.Error(w, r, http.StatusBadRequest, "INVALID_BODY", err.Error())
		return
	}
	publicID, err := h.Svc.Accept(r.Context(), id.TenantID, mb, id.DisplayName, SendInput{
		To: req.To, Cc: req.Cc, Bcc: req.Bcc,
		Subject: req.Subject, Text: req.Text, InReplyTo: req.InReplyTo,
		AttachmentIDs: req.AttachmentIDs, SenderUserID: id.UserID,
	})
	var vErr *ValidationError
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
