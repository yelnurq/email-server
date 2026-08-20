package webhooks

import (
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"net/http"
	"net/url"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/yelnurq/email-server/internal/audit"
	"github.com/yelnurq/email-server/internal/auth"
	"github.com/yelnurq/email-server/internal/events"
	"github.com/yelnurq/email-server/internal/httpx"
)

// Handlers manages webhook subscriptions and their delivery log.
type Handlers struct {
	Pool  *pgxpool.Pool
	Audit *audit.Logger
	Log   *slog.Logger
}

var allowedEvents = map[string]bool{
	events.SubjectAccepted:       true,
	events.SubjectDeliveredLocal: true,
	events.SubjectFailed:         true,
}

type hookView struct {
	ID             string   `json:"id"`
	OrganizationID string   `json:"organization_id"`
	URL            string   `json:"url"`
	Events         []string `json:"events"`
	Enabled        bool     `json:"enabled"`
	CreatedAt      string   `json:"created_at"`
}

// List returns the tenant's webhooks (secrets are never returned).
func (h *Handlers) List(w http.ResponseWriter, r *http.Request) {
	id := auth.IdentityFrom(r.Context())
	query := `
		SELECT id, organization_id, url, events, enabled, created_at::text
		FROM webhooks WHERE tenant_id = $1`
	args := []any{id.TenantID}
	if !id.TenantWide() {
		query += ` AND organization_id = $2`
		args = append(args, id.OrganizationID)
	}
	query += ` ORDER BY created_at DESC`
	rows, err := h.Pool.Query(r.Context(), query, args...)
	if err != nil {
		httpx.Internal(w, r)
		return
	}
	defer rows.Close()
	out := []hookView{}
	for rows.Next() {
		var v hookView
		if err := rows.Scan(&v.ID, &v.OrganizationID, &v.URL, &v.Events, &v.Enabled, &v.CreatedAt); err != nil {
			httpx.Internal(w, r)
			return
		}
		out = append(out, v)
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"webhooks": out})
}

type createRequest struct {
	OrganizationID string   `json:"organization_id"`
	URL            string   `json:"url"`
	Events         []string `json:"events"`
}

// Create registers a webhook; the signing secret is returned exactly once.
func (h *Handlers) Create(w http.ResponseWriter, r *http.Request) {
	id := auth.IdentityFrom(r.Context())
	var req createRequest
	if err := httpx.Decode(w, r, &req); err != nil {
		httpx.Error(w, r, http.StatusBadRequest, "INVALID_BODY", err.Error())
		return
	}
	u, err := url.Parse(strings.TrimSpace(req.URL))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		httpx.Error(w, r, http.StatusBadRequest, "INVALID_URL", "url must be an http(s) endpoint")
		return
	}
	if len(req.Events) == 0 {
		req.Events = []string{events.SubjectAccepted, events.SubjectDeliveredLocal, events.SubjectFailed}
	}
	for _, e := range req.Events {
		if !allowedEvents[e] {
			httpx.Error(w, r, http.StatusBadRequest, "INVALID_EVENT", "Unknown event: "+e)
			return
		}
	}
	orgID := req.OrganizationID
	if orgID == "" {
		orgID = id.OrganizationID
	}
	if orgID == "" {
		httpx.Error(w, r, http.StatusBadRequest, "MISSING_ORGANIZATION", "organization_id is required")
		return
	}
	if !id.TenantWide() && orgID != id.OrganizationID {
		httpx.Error(w, r, http.StatusForbidden, "FORBIDDEN", "Cannot manage another organization")
		return
	}
	var orgOK bool
	if err := h.Pool.QueryRow(r.Context(),
		`SELECT EXISTS (SELECT 1 FROM organizations WHERE id = $1 AND tenant_id = $2)`,
		orgID, id.TenantID).Scan(&orgOK); err != nil || !orgOK {
		httpx.Error(w, r, http.StatusNotFound, "ORGANIZATION_NOT_FOUND", "Organization not found")
		return
	}

	var buf [24]byte
	_, _ = rand.Read(buf[:])
	secret := "whsec_" + hex.EncodeToString(buf[:])

	var hookID string
	if err := h.Pool.QueryRow(r.Context(), `
		INSERT INTO webhooks (tenant_id, organization_id, url, secret, events, created_by)
		VALUES ($1, $2, $3, $4, $5, $6) RETURNING id`,
		id.TenantID, orgID, u.String(), secret, req.Events, id.UserID).Scan(&hookID); err != nil {
		h.Log.Error("webhook create failed", slog.String("error", err.Error()))
		httpx.Internal(w, r)
		return
	}
	h.Audit.Record(r.Context(), audit.Entry{
		TenantID: id.TenantID, ActorUserID: id.UserID, Action: "webhook.create",
		ResourceType: "webhook", ResourceID: hookID,
		Detail: map[string]any{"url": u.String(), "events": req.Events},
	})
	httpx.JSON(w, http.StatusCreated, map[string]any{
		"id":     hookID,
		"url":    u.String(),
		"events": req.Events,
		// Shown exactly once; used to verify X-Mailplatform-Signature.
		"secret": secret,
	})
}

type patchRequest struct {
	Enabled *bool `json:"enabled"`
}

// Patch enables/disables a webhook.
func (h *Handlers) Patch(w http.ResponseWriter, r *http.Request) {
	id := auth.IdentityFrom(r.Context())
	hookID := chi.URLParam(r, "id")
	var req patchRequest
	if err := httpx.Decode(w, r, &req); err != nil || req.Enabled == nil {
		httpx.Error(w, r, http.StatusBadRequest, "INVALID_BODY", "Provide {\"enabled\": true|false}")
		return
	}
	query := `UPDATE webhooks SET enabled = $1 WHERE id = $2 AND tenant_id = $3`
	args := []any{*req.Enabled, hookID, id.TenantID}
	if !id.TenantWide() {
		query += ` AND organization_id = $4`
		args = append(args, id.OrganizationID)
	}
	ct, err := h.Pool.Exec(r.Context(), query, args...)
	if err != nil {
		httpx.Internal(w, r)
		return
	}
	if ct.RowsAffected() == 0 {
		httpx.Error(w, r, http.StatusNotFound, "WEBHOOK_NOT_FOUND", "Webhook not found")
		return
	}
	httpx.JSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// Delete removes a webhook and its delivery log.
func (h *Handlers) Delete(w http.ResponseWriter, r *http.Request) {
	id := auth.IdentityFrom(r.Context())
	hookID := chi.URLParam(r, "id")
	query := `DELETE FROM webhooks WHERE id = $1 AND tenant_id = $2`
	args := []any{hookID, id.TenantID}
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
		httpx.Error(w, r, http.StatusNotFound, "WEBHOOK_NOT_FOUND", "Webhook not found")
		return
	}
	h.Audit.Record(r.Context(), audit.Entry{
		TenantID: id.TenantID, ActorUserID: id.UserID, Action: "webhook.delete",
		ResourceType: "webhook", ResourceID: hookID,
	})
	httpx.JSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

type deliveryView struct {
	ID            int64   `json:"id"`
	EventID       string  `json:"event_id"`
	EventType     string  `json:"event_type"`
	Status        string  `json:"status"`
	Attempts      int     `json:"attempts"`
	LastCode      *int    `json:"last_status_code,omitempty"`
	LastError     string  `json:"last_error,omitempty"`
	NextAttemptAt string  `json:"next_attempt_at"`
	DeliveredAt   *string `json:"delivered_at,omitempty"`
	CreatedAt     string  `json:"created_at"`
}

// Deliveries returns the delivery log of one webhook.
func (h *Handlers) Deliveries(w http.ResponseWriter, r *http.Request) {
	id := auth.IdentityFrom(r.Context())
	hookID := chi.URLParam(r, "id")

	ownQuery := `SELECT organization_id FROM webhooks WHERE id = $1 AND tenant_id = $2`
	var orgID string
	if err := h.Pool.QueryRow(r.Context(), ownQuery, hookID, id.TenantID).Scan(&orgID); err != nil {
		httpx.Error(w, r, http.StatusNotFound, "WEBHOOK_NOT_FOUND", "Webhook not found")
		return
	}
	if !id.TenantWide() && orgID != id.OrganizationID {
		httpx.Error(w, r, http.StatusForbidden, "FORBIDDEN", "Cannot manage another organization")
		return
	}

	rows, err := h.Pool.Query(r.Context(), `
		SELECT id, event_id, event_type, status, attempts, last_status_code,
		       last_error, next_attempt_at::text, delivered_at::text, created_at::text
		FROM webhook_deliveries
		WHERE webhook_id = $1
		ORDER BY id DESC
		LIMIT 100`, hookID)
	if err != nil {
		httpx.Internal(w, r)
		return
	}
	defer rows.Close()
	out := []deliveryView{}
	for rows.Next() {
		var v deliveryView
		if err := rows.Scan(&v.ID, &v.EventID, &v.EventType, &v.Status, &v.Attempts,
			&v.LastCode, &v.LastError, &v.NextAttemptAt, &v.DeliveredAt, &v.CreatedAt); err != nil {
			httpx.Internal(w, r)
			return
		}
		out = append(out, v)
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"deliveries": out})
}

// Retry reschedules a failed or pending delivery for immediate attempt.
func (h *Handlers) Retry(w http.ResponseWriter, r *http.Request) {
	id := auth.IdentityFrom(r.Context())
	hookID := chi.URLParam(r, "id")
	deliveryID := chi.URLParam(r, "deliveryID")

	ct, err := h.Pool.Exec(r.Context(), `
		UPDATE webhook_deliveries wd
		SET status = 'pending', next_attempt_at = now(), attempts = 0
		FROM webhooks w
		WHERE wd.id = $1::bigint AND wd.webhook_id = w.id
		  AND w.id = $2 AND w.tenant_id = $3
		  AND wd.status IN ('failed', 'pending')`,
		deliveryID, hookID, id.TenantID)
	if err != nil {
		httpx.Internal(w, r)
		return
	}
	if ct.RowsAffected() == 0 {
		httpx.Error(w, r, http.StatusNotFound, "DELIVERY_NOT_FOUND", "Delivery not found")
		return
	}
	h.Audit.Record(r.Context(), audit.Entry{
		TenantID: id.TenantID, ActorUserID: id.UserID, Action: "webhook.retry",
		ResourceType: "webhook_delivery", ResourceID: deliveryID,
	})
	httpx.JSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
