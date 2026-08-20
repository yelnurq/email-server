package messages

import (
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"

	"github.com/yelnurq/email-server/internal/events"
	"github.com/yelnurq/email-server/internal/httpx"
	"github.com/yelnurq/email-server/internal/mailaddr"
)

type draftRequest struct {
	To      []string `json:"to"`
	Cc      []string `json:"cc"`
	Bcc     []string `json:"bcc"`
	Subject string   `json:"subject"`
	Text    string   `json:"text"`
}

// storeDraftRecipients replaces a draft's recipient rows. Draft recipients
// are stored unvalidated-but-normalized where possible; validation happens
// at send time.
func storeDraftRecipients(r *http.Request, tx pgx.Tx, msgID string, req draftRequest) error {
	if _, err := tx.Exec(r.Context(), `DELETE FROM message_recipients WHERE message_id = $1`, msgID); err != nil {
		return err
	}
	insert := func(kind string, list []string) error {
		for _, a := range list {
			local, domain, err := mailaddr.NormalizeAddress(a)
			addr := a
			if err == nil {
				addr = mailaddr.Join(local, domain)
			}
			if len(addr) > 320 {
				continue
			}
			if _, err := tx.Exec(r.Context(), `
				INSERT INTO message_recipients (message_id, kind, address) VALUES ($1, $2, $3)`,
				msgID, kind, addr); err != nil {
				return err
			}
		}
		return nil
	}
	if err := insert("to", req.To); err != nil {
		return err
	}
	if err := insert("cc", req.Cc); err != nil {
		return err
	}
	return insert("bcc", req.Bcc)
}

// CreateDraft stores a new draft in the caller's Drafts folder.
func (h *WebmailHandlers) CreateDraft(w http.ResponseWriter, r *http.Request) {
	id, mb, ok := h.senderMailbox(w, r)
	if !ok {
		return
	}
	var req draftRequest
	if err := httpx.Decode(w, r, &req); err != nil {
		httpx.Error(w, r, http.StatusBadRequest, "INVALID_BODY", err.Error())
		return
	}
	if err := validateContent(req.Subject, req.Text); err != nil {
		httpx.Error(w, r, http.StatusBadRequest, "INVALID_MESSAGE", err.Error())
		return
	}

	tx, err := h.Svc.Pool.Begin(r.Context())
	if err != nil {
		httpx.Internal(w, r)
		return
	}
	defer tx.Rollback(r.Context())

	publicID := NewPublicID()
	var msgID string
	if err := tx.QueryRow(r.Context(), `
		INSERT INTO messages (public_id, tenant_id, from_address, from_display, subject,
		                      snippet, body_text, size_bytes, status)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, 'draft')
		RETURNING id`,
		publicID, id.TenantID, mb.Address, id.DisplayName, req.Subject,
		makeSnippet(req.Text), req.Text, int64(len(req.Text))).Scan(&msgID); err != nil {
		httpx.Internal(w, r)
		return
	}
	if err := storeDraftRecipients(r, tx, msgID, req); err != nil {
		httpx.Internal(w, r)
		return
	}
	var draftsFolderID, mmID string
	if err := tx.QueryRow(r.Context(),
		`SELECT id FROM folders WHERE mailbox_id = $1 AND type = 'drafts'`, mb.ID).Scan(&draftsFolderID); err != nil {
		httpx.Internal(w, r)
		return
	}
	if err := tx.QueryRow(r.Context(), `
		INSERT INTO mailbox_messages (mailbox_id, message_id, folder_id, is_read)
		VALUES ($1, $2, $3, true) RETURNING id`,
		mb.ID, msgID, draftsFolderID).Scan(&mmID); err != nil {
		httpx.Internal(w, r)
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		httpx.Internal(w, r)
		return
	}
	httpx.JSON(w, http.StatusCreated, map[string]string{"id": mmID, "message_id": publicID})
}

// draftForUpdate loads and locks a draft owned by the caller's mailbox.
func (h *WebmailHandlers) draftForUpdate(r *http.Request, tx pgx.Tx, mmID, mailboxID string) (msgID string, err error) {
	err = tx.QueryRow(r.Context(), `
		SELECT m.id
		FROM mailbox_messages mm
		JOIN messages m ON m.id = mm.message_id
		WHERE mm.id = $1 AND mm.mailbox_id = $2 AND m.status = 'draft'
		FOR UPDATE OF m`, mmID, mailboxID).Scan(&msgID)
	return msgID, err
}

// UpdateDraft replaces a draft's content.
func (h *WebmailHandlers) UpdateDraft(w http.ResponseWriter, r *http.Request) {
	_, mb, ok := h.senderMailbox(w, r)
	if !ok {
		return
	}
	mmID := chi.URLParam(r, "id")
	var req draftRequest
	if err := httpx.Decode(w, r, &req); err != nil {
		httpx.Error(w, r, http.StatusBadRequest, "INVALID_BODY", err.Error())
		return
	}
	if err := validateContent(req.Subject, req.Text); err != nil {
		httpx.Error(w, r, http.StatusBadRequest, "INVALID_MESSAGE", err.Error())
		return
	}

	tx, err := h.Svc.Pool.Begin(r.Context())
	if err != nil {
		httpx.Internal(w, r)
		return
	}
	defer tx.Rollback(r.Context())

	msgID, err := h.draftForUpdate(r, tx, mmID, mb.ID)
	if errors.Is(err, pgx.ErrNoRows) {
		httpx.Error(w, r, http.StatusNotFound, "DRAFT_NOT_FOUND", "Draft not found")
		return
	}
	if err != nil {
		httpx.Internal(w, r)
		return
	}
	if _, err := tx.Exec(r.Context(), `
		UPDATE messages SET subject = $1, snippet = $2, body_text = $3, size_bytes = $4
		WHERE id = $5`,
		req.Subject, makeSnippet(req.Text), req.Text, int64(len(req.Text)), msgID); err != nil {
		httpx.Internal(w, r)
		return
	}
	if err := storeDraftRecipients(r, tx, msgID, req); err != nil {
		httpx.Internal(w, r)
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		httpx.Internal(w, r)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// SendDraft validates and promotes a draft into the delivery pipeline.
func (h *WebmailHandlers) SendDraft(w http.ResponseWriter, r *http.Request) {
	id, mb, ok := h.senderMailbox(w, r)
	if !ok {
		return
	}
	mmID := chi.URLParam(r, "id")

	tx, err := h.Svc.Pool.Begin(r.Context())
	if err != nil {
		httpx.Internal(w, r)
		return
	}
	defer tx.Rollback(r.Context())

	msgID, err := h.draftForUpdate(r, tx, mmID, mb.ID)
	if errors.Is(err, pgx.ErrNoRows) {
		httpx.Error(w, r, http.StatusNotFound, "DRAFT_NOT_FOUND", "Draft not found")
		return
	}
	if err != nil {
		httpx.Internal(w, r)
		return
	}

	// Re-validate stored recipients through the strict send path.
	rows, err := tx.Query(r.Context(),
		`SELECT kind, address::text FROM message_recipients WHERE message_id = $1`, msgID)
	if err != nil {
		httpx.Internal(w, r)
		return
	}
	var in SendInput
	for rows.Next() {
		var kind, addr string
		if err := rows.Scan(&kind, &addr); err != nil {
			rows.Close()
			httpx.Internal(w, r)
			return
		}
		switch kind {
		case "to":
			in.To = append(in.To, addr)
		case "cc":
			in.Cc = append(in.Cc, addr)
		case "bcc":
			in.Bcc = append(in.Bcc, addr)
		}
	}
	rows.Close()
	to, cc, bcc, err := normalizeRecipients(in)
	var vErr *ValidationError
	if errors.As(err, &vErr) {
		httpx.Error(w, r, http.StatusBadRequest, "INVALID_MESSAGE", vErr.Msg)
		return
	}
	if err != nil {
		httpx.Internal(w, r)
		return
	}
	_ = to
	_ = cc
	_ = bcc

	var publicID string
	if err := tx.QueryRow(r.Context(), `
		UPDATE messages SET status = 'accepted', sent_at = now(), rfc_message_id = $1
		WHERE id = $2 RETURNING public_id`,
		NewRFCMessageID(mb.Domain), msgID).Scan(&publicID); err != nil {
		httpx.Internal(w, r)
		return
	}
	// Attach the promoted draft to a fresh thread.
	var threadID string
	if err := tx.QueryRow(r.Context(), `
		INSERT INTO threads (tenant_id, subject)
		SELECT tenant_id, subject FROM messages WHERE id = $1
		RETURNING id`, msgID).Scan(&threadID); err != nil {
		httpx.Internal(w, r)
		return
	}
	if _, err := tx.Exec(r.Context(),
		`UPDATE messages SET thread_id = $1 WHERE id = $2`, threadID, msgID); err != nil {
		httpx.Internal(w, r)
		return
	}
	// Move the mailbox copy from Drafts to Sent.
	if _, err := tx.Exec(r.Context(), `
		UPDATE mailbox_messages mm SET folder_id = f.id, created_at = now()
		FROM folders f
		WHERE mm.id = $1 AND mm.mailbox_id = $2
		  AND f.mailbox_id = mm.mailbox_id AND f.type = 'sent'`, mmID, mb.ID); err != nil {
		httpx.Internal(w, r)
		return
	}
	if _, err := tx.Exec(r.Context(), `
		INSERT INTO message_events (message_id, type, detail) VALUES ($1, 'email.accepted', '{}')`,
		msgID); err != nil {
		httpx.Internal(w, r)
		return
	}
	if err := events.Enqueue(r.Context(), tx, events.SubjectAccepted, events.AcceptedPayload{
		MessageID: msgID, PublicID: publicID, TenantID: id.TenantID,
	}); err != nil {
		httpx.Internal(w, r)
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		httpx.Internal(w, r)
		return
	}
	httpx.JSON(w, http.StatusAccepted, map[string]string{"message_id": publicID})
}
