// Package queueops exposes the mail core's outbound queue to the Admin
// Queue Center (V4 §59-64): summary, message list with per-recipient retry
// state and verbatim (sanitized) remote replies, plus retry-now and cancel
// actions.
package queueops

import (
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/yelnurq/email-server/internal/audit"
	"github.com/yelnurq/email-server/internal/auth"
	"github.com/yelnurq/email-server/internal/httpx"
	"github.com/yelnurq/email-server/internal/mailcore"
)

type Handlers struct {
	// Queue is nil when no mail core is configured.
	Queue mailcore.QueueManager
	Audit *audit.Logger
	Log   *slog.Logger
}

// summary aggregates the visible queue (§60).
type summary struct {
	Total        int     `json:"total"`
	Scheduled    int     `json:"scheduled"`
	Deferred     int     `json:"deferred"`
	Retrying     int     `json:"retrying"`
	OldestQueued *string `json:"oldest_queued,omitempty"`
	NextRetry    *string `json:"next_retry,omitempty"`
	// Listed is how many messages the response carries; when Listed < Total
	// the UI must say so instead of implying full coverage.
	Listed int `json:"listed"`
}

// requireQueue guards handlers: a real mail core and a tenant-wide admin.
// The queue is platform-level infrastructure — org-scoped admins cannot see
// other organizations' traffic through it (§87-88).
func (h *Handlers) requireQueue(w http.ResponseWriter, r *http.Request) bool {
	id := auth.IdentityFrom(r.Context())
	if !id.TenantWide() {
		httpx.Error(w, r, http.StatusForbidden, "FORBIDDEN", "Queue operations need platform-wide administration rights")
		return false
	}
	if h.Queue == nil {
		httpx.Error(w, r, http.StatusServiceUnavailable, "MAIL_CORE_DISABLED", "No mail core is configured")
		return false
	}
	return true
}

// List returns the queue summary and messages.
func (h *Handlers) List(w http.ResponseWriter, r *http.Request) {
	if !h.requireQueue(w, r) {
		return
	}
	msgs, total, err := h.Queue.ListQueue(r.Context(), 50)
	if err != nil {
		h.queueError(w, r, err, "queue list failed")
		return
	}
	s := summary{Total: total, Listed: len(msgs)}
	var oldest *time.Time
	var next *time.Time
	for _, m := range msgs {
		m := m
		if oldest == nil || m.Created.Before(*oldest) {
			oldest = &m.Created
		}
		for _, rc := range m.Recipients {
			switch rc.Status {
			case "temp_fail":
				s.Deferred++
			case "scheduled":
				s.Scheduled++
			}
			if rc.RetryNum > 0 {
				s.Retrying++
			}
			if rc.NextRetry != nil && (next == nil || rc.NextRetry.Before(*next)) {
				next = rc.NextRetry
			}
		}
	}
	if oldest != nil {
		v := oldest.UTC().Format(time.RFC3339)
		s.OldestQueued = &v
	}
	if next != nil {
		v := next.UTC().Format(time.RFC3339)
		s.NextRetry = &v
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"summary": s, "messages": msgs})
}

// Retry reschedules one message for immediate delivery (§63).
func (h *Handlers) Retry(w http.ResponseWriter, r *http.Request) {
	if !h.requireQueue(w, r) {
		return
	}
	id := auth.IdentityFrom(r.Context())
	qid := chi.URLParam(r, "id")
	if err := h.Queue.RetryQueued(r.Context(), qid); err != nil {
		h.queueError(w, r, err, "queue retry failed")
		return
	}
	h.Audit.Record(r.Context(), audit.Entry{
		TenantID: id.TenantID, ActorUserID: id.UserID, Action: "queue.retry",
		ResourceType: "queue_message", ResourceID: qid,
	})
	httpx.JSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// Cancel removes one message from the queue permanently (§63). The UI asks
// for explicit confirmation; the API records the actor in the audit trail.
func (h *Handlers) Cancel(w http.ResponseWriter, r *http.Request) {
	if !h.requireQueue(w, r) {
		return
	}
	id := auth.IdentityFrom(r.Context())
	qid := chi.URLParam(r, "id")
	if err := h.Queue.CancelQueued(r.Context(), qid); err != nil {
		h.queueError(w, r, err, "queue cancel failed")
		return
	}
	h.Audit.Record(r.Context(), audit.Entry{
		TenantID: id.TenantID, ActorUserID: id.UserID, Action: "queue.cancel",
		ResourceType: "queue_message", ResourceID: qid,
	})
	httpx.JSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handlers) queueError(w http.ResponseWriter, r *http.Request, err error, msg string) {
	if errors.Is(err, mailcore.ErrUnavailable) {
		httpx.Error(w, r, http.StatusServiceUnavailable, "MAIL_CORE_UNAVAILABLE",
			"The mail core is unavailable; try again")
		return
	}
	h.Log.Error(msg, slog.String("error", err.Error()))
	httpx.Internal(w, r)
}
