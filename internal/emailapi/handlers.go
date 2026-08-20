// Package emailapi implements the developer-facing Email API: API-key
// authenticated sends with Idempotency-Key support, message status and
// delivery events. Requests return 202 immediately; delivery is async.
package emailapi

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/yelnurq/email-server/internal/apikeys"
	"github.com/yelnurq/email-server/internal/httpx"
	"github.com/yelnurq/email-server/internal/messages"
)

type Handlers struct {
	Pool *pgxpool.Pool
	Keys *apikeys.Service
	Svc  *messages.Service
	Log  *slog.Logger
}

type ctxKey struct{}

func keyFrom(ctx context.Context) *apikeys.Key {
	k, _ := ctx.Value(ctxKey{}).(*apikeys.Key)
	return k
}

// RequireAPIKey authenticates requests with a platform API key.
func (h *Handlers) RequireAPIKey(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		key, err := h.Keys.Verify(r.Context(), raw)
		if err != nil {
			httpx.Error(w, r, http.StatusUnauthorized, "INVALID_API_KEY", "A valid API key is required")
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), ctxKey{}, key)))
	})
}

// RequireScope gates a route on an API-key scope.
func RequireScope(scope string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if k := keyFrom(r.Context()); k == nil || !k.HasScope(scope) {
				httpx.Error(w, r, http.StatusForbidden, "INSUFFICIENT_SCOPE", "API key lacks scope "+scope)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

type sendRequest struct {
	From    string   `json:"from"`
	To      []string `json:"to"`
	Cc      []string `json:"cc"`
	Bcc     []string `json:"bcc"`
	Subject string   `json:"subject"`
	Text    string   `json:"text"`
	HTML    string   `json:"html"`
}

// senderForKey resolves the from address to an active mailbox inside the
// key's organization.
func (h *Handlers) senderForKey(ctx context.Context, key *apikeys.Key, from string) (*messages.SenderMailbox, string, error) {
	var mb messages.SenderMailbox
	var display string
	err := h.Pool.QueryRow(ctx, `
		SELECT m.id, m.address::text, d.name::text, m.organization_id,
		       COALESCE(u.display_name, '')
		FROM mailboxes m
		JOIN domains d ON d.id = m.domain_id
		LEFT JOIN users u ON u.id = m.user_id
		WHERE m.address = $1 AND m.tenant_id = $2 AND m.organization_id = $3
		  AND m.status = 'active' AND d.status = 'verified'`,
		strings.TrimSpace(strings.ToLower(from)), key.TenantID, key.OrganizationID).
		Scan(&mb.ID, &mb.Address, &mb.Domain, &mb.OrgID, &display)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, "", errFromNotAllowed
	}
	return &mb, display, err
}

var errFromNotAllowed = errors.New("from address is not a mailbox of this organization")

// Send accepts one message: POST /api/v1/emails.
func (h *Handlers) Send(w http.ResponseWriter, r *http.Request) {
	key := keyFrom(r.Context())

	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 2<<20))
	if err != nil {
		httpx.Error(w, r, http.StatusBadRequest, "INVALID_BODY", "Request body too large or unreadable")
		return
	}
	var req sendRequest
	if err := decodeStrict(body, &req); err != nil {
		httpx.Error(w, r, http.StatusBadRequest, "INVALID_BODY", err.Error())
		return
	}

	idemKey := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if len(idemKey) > 255 {
		httpx.Error(w, r, http.StatusBadRequest, "INVALID_IDEMPOTENCY_KEY", "Idempotency-Key is too long")
		return
	}
	sum := sha256.Sum256(body)
	requestHash := hex.EncodeToString(sum[:])

	tx, err := h.Pool.Begin(r.Context())
	if err != nil {
		httpx.Internal(w, r)
		return
	}
	defer tx.Rollback(r.Context())

	if idemKey != "" {
		ct, err := tx.Exec(r.Context(), `
			INSERT INTO idempotency_keys (api_key_id, idem_key, request_hash, message_id)
			VALUES ($1, $2, $3, '')
			ON CONFLICT (api_key_id, idem_key) DO NOTHING`,
			key.ID, idemKey, requestHash)
		if err != nil {
			httpx.Internal(w, r)
			return
		}
		if ct.RowsAffected() == 0 {
			// A previous (or concurrent) request holds this key. FOR UPDATE
			// waits for an in-flight transaction to finish.
			var storedHash, storedMsgID string
			if err := tx.QueryRow(r.Context(), `
				SELECT request_hash, message_id FROM idempotency_keys
				WHERE api_key_id = $1 AND idem_key = $2 FOR UPDATE`,
				key.ID, idemKey).Scan(&storedHash, &storedMsgID); err != nil {
				httpx.Internal(w, r)
				return
			}
			if storedHash != requestHash {
				httpx.Error(w, r, http.StatusConflict, "IDEMPOTENCY_CONFLICT",
					"This Idempotency-Key was already used with a different payload")
				return
			}
			if storedMsgID == "" {
				httpx.Error(w, r, http.StatusConflict, "IDEMPOTENCY_IN_PROGRESS",
					"The original request with this key did not complete; retry later")
				return
			}
			httpx.JSON(w, http.StatusOK, map[string]any{
				"message_id": storedMsgID, "replayed": true,
			})
			return
		}
	}

	sender, display, err := h.senderForKey(r.Context(), key, req.From)
	if errors.Is(err, errFromNotAllowed) {
		httpx.Error(w, r, http.StatusUnprocessableEntity, "FROM_NOT_ALLOWED",
			"from must be an active mailbox of your organization")
		return
	}
	if err != nil {
		httpx.Internal(w, r)
		return
	}

	publicID, err := h.Svc.AcceptInTx(r.Context(), tx, key.TenantID, sender, display, messages.SendInput{
		To: req.To, Cc: req.Cc, Bcc: req.Bcc,
		Subject: req.Subject, Text: req.Text, HTML: req.HTML,
	})
	var vErr *messages.ValidationError
	if errors.As(err, &vErr) {
		httpx.Error(w, r, http.StatusBadRequest, "INVALID_MESSAGE", vErr.Msg)
		return
	}
	if err != nil {
		h.Log.Error("email api accept failed", slog.String("error", err.Error()))
		httpx.Internal(w, r)
		return
	}
	if idemKey != "" {
		if _, err := tx.Exec(r.Context(), `
			UPDATE idempotency_keys SET message_id = $1
			WHERE api_key_id = $2 AND idem_key = $3`,
			publicID, key.ID, idemKey); err != nil {
			httpx.Internal(w, r)
			return
		}
	}
	if err := tx.Commit(r.Context()); err != nil {
		httpx.Internal(w, r)
		return
	}
	httpx.JSON(w, http.StatusAccepted, map[string]string{"message_id": publicID})
}

// SendBatch accepts up to 50 messages: POST /api/v1/emails/batch.
// Batch requests do not support Idempotency-Key (per-item results instead).
func (h *Handlers) SendBatch(w http.ResponseWriter, r *http.Request) {
	key := keyFrom(r.Context())
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 10<<20))
	if err != nil {
		httpx.Error(w, r, http.StatusBadRequest, "INVALID_BODY", "Request body too large or unreadable")
		return
	}
	var reqs []sendRequest
	if err := decodeStrict(body, &reqs); err != nil {
		httpx.Error(w, r, http.StatusBadRequest, "INVALID_BODY", "Body must be a JSON array of messages")
		return
	}
	if len(reqs) == 0 || len(reqs) > 50 {
		httpx.Error(w, r, http.StatusBadRequest, "INVALID_BATCH", "Batch must contain 1-50 messages")
		return
	}

	type result struct {
		MessageID string `json:"message_id,omitempty"`
		Error     string `json:"error,omitempty"`
	}
	results := make([]result, len(reqs))
	accepted := 0
	for i, req := range reqs {
		sender, display, err := h.senderForKey(r.Context(), key, req.From)
		if err != nil {
			results[i] = result{Error: "FROM_NOT_ALLOWED"}
			continue
		}
		publicID, err := h.Svc.Accept(r.Context(), key.TenantID, sender, display, messages.SendInput{
			To: req.To, Cc: req.Cc, Bcc: req.Bcc,
			Subject: req.Subject, Text: req.Text, HTML: req.HTML,
		})
		var vErr *messages.ValidationError
		switch {
		case errors.As(err, &vErr):
			results[i] = result{Error: vErr.Msg}
		case err != nil:
			results[i] = result{Error: "INTERNAL"}
		default:
			results[i] = result{MessageID: publicID}
			accepted++
		}
	}
	httpx.JSON(w, http.StatusAccepted, map[string]any{
		"accepted": accepted, "total": len(reqs), "results": results,
	})
}

// Get returns message status and recipients: GET /api/v1/emails/{id}.
func (h *Handlers) Get(w http.ResponseWriter, r *http.Request) {
	key := keyFrom(r.Context())
	publicID := chi.URLParam(r, "id")

	var msgID, from, subject, status, createdAt string
	err := h.Pool.QueryRow(r.Context(), `
		SELECT id, from_address::text, subject, status, created_at::text
		FROM messages
		WHERE public_id = $1 AND tenant_id = $2 AND organization_id = $3`,
		publicID, key.TenantID, key.OrganizationID).
		Scan(&msgID, &from, &subject, &status, &createdAt)
	if errors.Is(err, pgx.ErrNoRows) {
		httpx.Error(w, r, http.StatusNotFound, "MESSAGE_NOT_FOUND", "Message not found")
		return
	}
	if err != nil {
		httpx.Internal(w, r)
		return
	}

	rows, err := h.Pool.Query(r.Context(), `
		SELECT kind, address::text, status, error FROM message_recipients
		WHERE message_id = $1 ORDER BY created_at`, msgID)
	if err != nil {
		httpx.Internal(w, r)
		return
	}
	defer rows.Close()
	type rec struct {
		Kind    string `json:"kind"`
		Address string `json:"address"`
		Status  string `json:"status"`
		Error   string `json:"error,omitempty"`
	}
	recipients := []rec{}
	for rows.Next() {
		var x rec
		if err := rows.Scan(&x.Kind, &x.Address, &x.Status, &x.Error); err != nil {
			httpx.Internal(w, r)
			return
		}
		recipients = append(recipients, x)
	}
	httpx.JSON(w, http.StatusOK, map[string]any{
		"message_id": publicID,
		"from":       from,
		"subject":    subject,
		"status":     status,
		"created_at": createdAt,
		"recipients": recipients,
	})
}

// Events returns the delivery event log: GET /api/v1/emails/{id}/events.
func (h *Handlers) Events(w http.ResponseWriter, r *http.Request) {
	key := keyFrom(r.Context())
	publicID := chi.URLParam(r, "id")

	var msgID string
	err := h.Pool.QueryRow(r.Context(), `
		SELECT id FROM messages
		WHERE public_id = $1 AND tenant_id = $2 AND organization_id = $3`,
		publicID, key.TenantID, key.OrganizationID).Scan(&msgID)
	if errors.Is(err, pgx.ErrNoRows) {
		httpx.Error(w, r, http.StatusNotFound, "MESSAGE_NOT_FOUND", "Message not found")
		return
	}
	if err != nil {
		httpx.Internal(w, r)
		return
	}

	rows, err := h.Pool.Query(r.Context(), `
		SELECT type, detail, created_at::text FROM message_events
		WHERE message_id = $1 ORDER BY id`, msgID)
	if err != nil {
		httpx.Internal(w, r)
		return
	}
	defer rows.Close()
	type ev struct {
		Type      string         `json:"type"`
		Detail    map[string]any `json:"detail"`
		CreatedAt string         `json:"created_at"`
	}
	events := []ev{}
	for rows.Next() {
		var x ev
		if err := rows.Scan(&x.Type, &x.Detail, &x.CreatedAt); err != nil {
			httpx.Internal(w, r)
			return
		}
		events = append(events, x)
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"message_id": publicID, "events": events})
}
