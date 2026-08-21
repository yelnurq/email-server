package mailcore

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// QueueRecipient is one recipient of a queued outbound message.
type QueueRecipient struct {
	Address string `json:"address"`
	Queue   string `json:"queue"`
	// Status: scheduled | temp_fail | perm_fail | completed | unknown.
	Status string `json:"status"`
	// StatusDetail carries the remote server's verbatim reply, sanitized
	// (control characters stripped, length capped — V4 §66).
	StatusDetail string     `json:"status_detail,omitempty"`
	RetryNum     int        `json:"retry_num"`
	NextRetry    *time.Time `json:"next_retry,omitempty"`
	NextNotify   *time.Time `json:"next_notify,omitempty"`
	Expires      *time.Time `json:"expires,omitempty"`
}

// QueueMessage is one message in the mail core's outbound queue.
type QueueMessage struct {
	ID         string           `json:"id"`
	ReturnPath string           `json:"return_path"`
	Created    time.Time        `json:"created"`
	Size       int64            `json:"size"`
	Recipients []QueueRecipient `json:"recipients"`
}

// QueueManager is the outbound-queue management surface (V4 §59-64).
// Implemented by Stalwart; absent when no mail core is configured.
type QueueManager interface {
	// ListQueue returns up to limit queued messages (newest last) plus the
	// total number of ids in the queue.
	ListQueue(ctx context.Context, limit int) ([]QueueMessage, int, error)
	// RetryQueued reschedules one queued message for immediate delivery.
	RetryQueued(ctx context.Context, id string) error
	// CancelQueued removes one message from the queue permanently.
	CancelQueued(ctx context.Context, id string) error
}

// sanitizeSMTPReply bounds and cleans a remote SMTP reply before it is
// stored or displayed: control characters become spaces (no log injection)
// and length is capped (§66).
func sanitizeSMTPReply(s string) string {
	s = strings.Map(func(r rune) rune {
		if r < 32 || r == 127 {
			return ' '
		}
		return r
	}, s)
	if len(s) > 500 {
		s = s[:500] + "…"
	}
	return strings.TrimSpace(s)
}

// ListQueue implements QueueManager on the Stalwart management API
// (GET /api/queue/messages returns ids; each id is fetched for detail —
// verified live on v0.13.4).
func (s *Stalwart) ListQueue(ctx context.Context, limit int) ([]QueueMessage, int, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	var list struct {
		Data struct {
			Items []json.Number `json:"items"`
			Total int           `json:"total"`
		} `json:"data"`
	}
	if err := s.do(ctx, http.MethodGet, "/api/queue/messages", nil, &list); err != nil {
		return nil, 0, err
	}
	out := make([]QueueMessage, 0, min(limit, len(list.Data.Items)))
	for i, id := range list.Data.Items {
		if i >= limit {
			break
		}
		var detail struct {
			Data struct {
				ID         json.Number `json:"id"`
				ReturnPath string      `json:"return_path"`
				Created    time.Time   `json:"created"`
				Size       int64       `json:"size"`
				Recipients []struct {
					Address    string          `json:"address"`
					Queue      string          `json:"queue"`
					Status     json.RawMessage `json:"status"`
					RetryNum   int             `json:"retry_num"`
					NextRetry  *time.Time      `json:"next_retry"`
					NextNotify *time.Time      `json:"next_notify"`
					Expires    *time.Time      `json:"expires"`
				} `json:"recipients"`
			} `json:"data"`
		}
		if err := s.do(ctx, http.MethodGet, "/api/queue/messages/"+id.String(), nil, &detail); err != nil {
			// The message may have completed between listing and fetching.
			continue
		}
		qm := QueueMessage{
			ID:         detail.Data.ID.String(),
			ReturnPath: detail.Data.ReturnPath,
			Created:    detail.Data.Created,
			Size:       detail.Data.Size,
		}
		for _, r := range detail.Data.Recipients {
			status, statusDetail := parseQueueStatus(r.Status)
			qm.Recipients = append(qm.Recipients, QueueRecipient{
				Address: r.Address, Queue: r.Queue,
				Status: status, StatusDetail: statusDetail,
				RetryNum: r.RetryNum, NextRetry: r.NextRetry,
				NextNotify: r.NextNotify, Expires: r.Expires,
			})
		}
		out = append(out, qm)
	}
	return out, list.Data.Total, nil
}

// parseQueueStatus decodes Stalwart's recipient status, which is either a
// plain string ("scheduled") or an object keyed by outcome
// ({"temp_fail": "<verbatim remote reply>"}).
func parseQueueStatus(raw json.RawMessage) (status, detail string) {
	if len(raw) == 0 {
		return "scheduled", ""
	}
	var plain string
	if json.Unmarshal(raw, &plain) == nil {
		return plain, ""
	}
	var obj map[string]string
	if json.Unmarshal(raw, &obj) == nil {
		for k, v := range obj {
			return k, sanitizeSMTPReply(v)
		}
	}
	return "unknown", ""
}

// RetryQueued reschedules the message for immediate delivery
// (PATCH /api/queue/messages/{id}, verified live).
func (s *Stalwart) RetryQueued(ctx context.Context, id string) error {
	if err := validQueueID(id); err != nil {
		return err
	}
	return s.do(ctx, http.MethodPatch, "/api/queue/messages/"+id, nil, nil)
}

// CancelQueued removes the message from the queue
// (DELETE /api/queue/messages/{id}, verified live).
func (s *Stalwart) CancelQueued(ctx context.Context, id string) error {
	if err := validQueueID(id); err != nil {
		return err
	}
	return s.do(ctx, http.MethodDelete, "/api/queue/messages/"+id, nil, nil)
}

// validQueueID guards the URL path: queue ids are numeric.
func validQueueID(id string) error {
	if id == "" {
		return fmt.Errorf("empty queue id")
	}
	for _, r := range id {
		if r < '0' || r > '9' {
			return fmt.Errorf("invalid queue id")
		}
	}
	return nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
