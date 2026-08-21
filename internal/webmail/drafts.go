package webmail

import (
	"errors"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/yelnurq/email-server/internal/auth"
	"github.com/yelnurq/email-server/internal/httpx"
	"github.com/yelnurq/email-server/internal/mailservice"
	"github.com/yelnurq/email-server/internal/messages"
)

// Drafts live in the mail store's Drafts folder (ADR-003), so IMAP/JMAP
// clients and webmail share them. The store has no in-place draft update:
// every save writes a fresh draft and destroys the previous copy, so the
// draft id changes on each save and the response carries the current id.

type draftRequest struct {
	To      []string `json:"to"`
	Cc      []string `json:"cc"`
	Bcc     []string `json:"bcc"`
	Subject string   `json:"subject"`
	Text    string   `json:"text"`
}

func (h *Handlers) renderDraft(id *auth.Identity, mb *messages.SenderMailbox, req draftRequest, rfcID string) []byte {
	return mailservice.BuildMessage(mailservice.Envelope{
		From:        mb.Address,
		FromDisplay: id.DisplayName,
		To:          req.To,
		Cc:          req.Cc,
		Subject:     req.Subject,
		Date:        time.Now().UTC(),
		RFCID:       rfcID,
		Text:        req.Text,
	})
}

// CreateDraft stores a new draft.
func (h *Handlers) CreateDraft(w http.ResponseWriter, r *http.Request) {
	id, mb, ok := h.senderMailbox(w, r)
	if !ok {
		return
	}
	var req draftRequest
	if err := httpx.Decode(w, r, &req); err != nil {
		httpx.Error(w, r, http.StatusBadRequest, "INVALID_BODY", err.Error())
		return
	}
	// Every draft carries a Message-ID from birth: it keeps its identity
	// across saves (each save is a new store object) and lets reconciliation
	// recognize stale copies left by a failed destroy.
	rfcID := messages.NewRFCMessageID(mb.Domain)
	draftID, err := h.Mail.SaveDraft(r.Context(), mb.Address, "", h.renderDraft(id, mb, req, rfcID))
	if err != nil {
		h.storeError(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, map[string]string{"id": draftID})
}

// UpdateDraft replaces a draft's content; the response carries the new id.
func (h *Handlers) UpdateDraft(w http.ResponseWriter, r *http.Request) {
	id, mb, ok := h.senderMailbox(w, r)
	if !ok {
		return
	}
	var req draftRequest
	if err := httpx.Decode(w, r, &req); err != nil {
		httpx.Error(w, r, http.StatusBadRequest, "INVALID_BODY", err.Error())
		return
	}
	previousID := chi.URLParam(r, "id")
	// Reuse the draft's Message-ID so all saves share one identity; a draft
	// created before V4 (or an unreadable previous copy) gets a fresh id.
	rfcID, err := h.Mail.MessageIDOf(r.Context(), mb.Address, previousID)
	if err != nil || rfcID == "" {
		rfcID = messages.NewRFCMessageID(mb.Domain)
	}
	draftID, err := h.Mail.SaveDraft(r.Context(), mb.Address, previousID, h.renderDraft(id, mb, req, rfcID))
	if err != nil {
		h.storeError(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]string{"status": "ok", "id": draftID})
}

// SendDraft promotes a stored draft into the delivery pipeline and removes
// the draft copy.
func (h *Handlers) SendDraft(w http.ResponseWriter, r *http.Request) {
	id, mb, ok := h.senderMailbox(w, r)
	if !ok {
		return
	}
	draftID := chi.URLParam(r, "id")
	msg, err := h.Mail.GetMessage(r.Context(), mb.Address, draftID)
	if err != nil {
		h.storeError(w, r, err)
		return
	}
	if msg == nil || msg.Folder != "drafts" {
		httpx.Error(w, r, http.StatusNotFound, "DRAFT_NOT_FOUND", "Draft not found")
		return
	}
	in := messages.SendInput{
		Subject: msg.Subject, Text: msg.BodyText, SenderUserID: id.UserID,
	}
	for _, rc := range msg.Recipients {
		switch rc.Kind {
		case "to":
			in.To = append(in.To, rc.Address)
		case "cc":
			in.Cc = append(in.Cc, rc.Address)
		case "bcc":
			in.Bcc = append(in.Bcc, rc.Address)
		}
	}
	publicID, err := h.Accept.Accept(r.Context(), id.TenantID, mb, id.DisplayName, in)
	var vErr *messages.ValidationError
	if errors.As(err, &vErr) {
		httpx.Error(w, r, http.StatusBadRequest, "INVALID_MESSAGE", vErr.Msg)
		return
	}
	if err != nil {
		httpx.Internal(w, r)
		return
	}
	// The pipeline will import the canonical Sent copy; drop the draft.
	_ = h.Mail.Destroy(r.Context(), mb.Address, draftID)
	httpx.JSON(w, http.StatusAccepted, map[string]string{"message_id": publicID})
}
