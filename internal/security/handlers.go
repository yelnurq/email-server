package security

import (
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
	"github.com/yelnurq/email-server/internal/mailaddr"
)

// Handlers exposes the Security Center: quarantine and sender blocks.
type Handlers struct {
	Pool  *pgxpool.Pool
	Audit *audit.Logger
	Log   *slog.Logger
}

type quarantineView struct {
	ID               string   `json:"id"`
	MessagePublicID  string   `json:"message_id"`
	From             string   `json:"from"`
	Subject          string   `json:"subject"`
	RecipientAddress string   `json:"recipient_address"`
	Reason           string   `json:"reason"`
	Signals          []string `json:"signals"`
	RiskScore        int      `json:"risk_score"`
	Status           string   `json:"status"`
	CreatedAt        string   `json:"created_at"`
}

// Quarantine lists quarantine items for the tenant.
func (h *Handlers) Quarantine(w http.ResponseWriter, r *http.Request) {
	id := auth.IdentityFrom(r.Context())
	rows, err := h.Pool.Query(r.Context(), `
		SELECT q.id, m.public_id, m.from_address::text, m.subject,
		       q.recipient_address::text, q.reason, q.signals, q.risk_score,
		       q.status, q.created_at::text
		FROM quarantine_items q
		JOIN messages m ON m.id = q.message_id
		WHERE q.tenant_id = $1
		ORDER BY q.created_at DESC
		LIMIT 200`, id.TenantID)
	if err != nil {
		h.Log.Error("quarantine list failed", slog.String("error", err.Error()))
		httpx.Internal(w, r)
		return
	}
	defer rows.Close()
	out := []quarantineView{}
	for rows.Next() {
		var v quarantineView
		if err := rows.Scan(&v.ID, &v.MessagePublicID, &v.From, &v.Subject,
			&v.RecipientAddress, &v.Reason, &v.Signals, &v.RiskScore,
			&v.Status, &v.CreatedAt); err != nil {
			httpx.Internal(w, r)
			return
		}
		out = append(out, v)
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"quarantine": out})
}

// Release delivers a quarantined item into the recipient's Inbox.
func (h *Handlers) Release(w http.ResponseWriter, r *http.Request) {
	id := auth.IdentityFrom(r.Context())
	itemID := chi.URLParam(r, "id")

	tx, err := h.Pool.Begin(r.Context())
	if err != nil {
		httpx.Internal(w, r)
		return
	}
	defer tx.Rollback(r.Context())

	var messageID, recipientID string
	var mailboxID *string
	err = tx.QueryRow(r.Context(), `
		SELECT message_id, recipient_id, mailbox_id FROM quarantine_items
		WHERE id = $1 AND tenant_id = $2 AND status = 'pending'
		FOR UPDATE`, itemID, id.TenantID).Scan(&messageID, &recipientID, &mailboxID)
	if errors.Is(err, pgx.ErrNoRows) {
		httpx.Error(w, r, http.StatusNotFound, "ITEM_NOT_FOUND", "Quarantine item not found or already resolved")
		return
	}
	if err != nil {
		httpx.Internal(w, r)
		return
	}
	if mailboxID == nil {
		httpx.Error(w, r, http.StatusConflict, "NO_MAILBOX", "The recipient mailbox no longer exists")
		return
	}
	var inboxID string
	if err := tx.QueryRow(r.Context(),
		`SELECT id FROM folders WHERE mailbox_id = $1 AND type = 'inbox'`, *mailboxID).Scan(&inboxID); err != nil {
		httpx.Internal(w, r)
		return
	}
	if _, err := tx.Exec(r.Context(), `
		INSERT INTO mailbox_messages (mailbox_id, message_id, folder_id)
		VALUES ($1, $2, $3) ON CONFLICT (mailbox_id, message_id, folder_id) DO NOTHING`,
		*mailboxID, messageID, inboxID); err != nil {
		httpx.Internal(w, r)
		return
	}
	if _, err := tx.Exec(r.Context(),
		`UPDATE message_recipients SET status = 'delivered', error = '' WHERE id = $1`,
		recipientID); err != nil {
		httpx.Internal(w, r)
		return
	}
	if _, err := tx.Exec(r.Context(), `
		UPDATE quarantine_items SET status = 'released', resolved_at = now(), resolved_by = $1
		WHERE id = $2`, id.UserID, itemID); err != nil {
		httpx.Internal(w, r)
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		httpx.Internal(w, r)
		return
	}
	h.Audit.Record(r.Context(), audit.Entry{
		TenantID: id.TenantID, ActorUserID: id.UserID, Action: "quarantine.release",
		ResourceType: "quarantine_item", ResourceID: itemID,
	})
	httpx.JSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// DeleteItem resolves a quarantine item as deleted (message copy is never
// delivered).
func (h *Handlers) DeleteItem(w http.ResponseWriter, r *http.Request) {
	id := auth.IdentityFrom(r.Context())
	itemID := chi.URLParam(r, "id")
	ct, err := h.Pool.Exec(r.Context(), `
		UPDATE quarantine_items SET status = 'deleted', resolved_at = now(), resolved_by = $1
		WHERE id = $2 AND tenant_id = $3 AND status = 'pending'`,
		id.UserID, itemID, id.TenantID)
	if err != nil {
		httpx.Internal(w, r)
		return
	}
	if ct.RowsAffected() == 0 {
		httpx.Error(w, r, http.StatusNotFound, "ITEM_NOT_FOUND", "Quarantine item not found or already resolved")
		return
	}
	h.Audit.Record(r.Context(), audit.Entry{
		TenantID: id.TenantID, ActorUserID: id.UserID, Action: "quarantine.delete",
		ResourceType: "quarantine_item", ResourceID: itemID,
	})
	httpx.JSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

type blockView struct {
	ID        string `json:"id"`
	Pattern   string `json:"pattern"`
	Kind      string `json:"kind"`
	Reason    string `json:"reason"`
	CreatedAt string `json:"created_at"`
}

// Blocks lists sender blocks.
func (h *Handlers) Blocks(w http.ResponseWriter, r *http.Request) {
	id := auth.IdentityFrom(r.Context())
	rows, err := h.Pool.Query(r.Context(), `
		SELECT id, pattern::text, kind, reason, created_at::text
		FROM sender_blocks WHERE tenant_id = $1 ORDER BY created_at DESC`, id.TenantID)
	if err != nil {
		httpx.Internal(w, r)
		return
	}
	defer rows.Close()
	out := []blockView{}
	for rows.Next() {
		var v blockView
		if err := rows.Scan(&v.ID, &v.Pattern, &v.Kind, &v.Reason, &v.CreatedAt); err != nil {
			httpx.Internal(w, r)
			return
		}
		out = append(out, v)
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"blocks": out})
}

type blockRequest struct {
	Pattern string `json:"pattern"`
	Reason  string `json:"reason"`
}

// AddBlock blocks a sender address or a whole domain.
func (h *Handlers) AddBlock(w http.ResponseWriter, r *http.Request) {
	id := auth.IdentityFrom(r.Context())
	var req blockRequest
	if err := httpx.Decode(w, r, &req); err != nil {
		httpx.Error(w, r, http.StatusBadRequest, "INVALID_BODY", err.Error())
		return
	}
	pattern := strings.ToLower(strings.TrimSpace(req.Pattern))
	kind := "domain"
	if strings.Contains(pattern, "@") {
		kind = "address"
		local, domain, err := mailaddr.NormalizeAddress(pattern)
		if err != nil {
			httpx.Error(w, r, http.StatusBadRequest, "INVALID_PATTERN", "Invalid address")
			return
		}
		pattern = mailaddr.Join(local, domain)
	} else {
		var err error
		if pattern, err = mailaddr.NormalizeDomain(pattern); err != nil {
			httpx.Error(w, r, http.StatusBadRequest, "INVALID_PATTERN", "Invalid domain")
			return
		}
	}
	var blockID string
	err := h.Pool.QueryRow(r.Context(), `
		INSERT INTO sender_blocks (tenant_id, pattern, kind, reason, created_by)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (tenant_id, pattern) DO UPDATE SET reason = EXCLUDED.reason
		RETURNING id`,
		id.TenantID, pattern, kind, strings.TrimSpace(req.Reason), id.UserID).Scan(&blockID)
	if err != nil {
		httpx.Internal(w, r)
		return
	}
	h.Audit.Record(r.Context(), audit.Entry{
		TenantID: id.TenantID, ActorUserID: id.UserID, Action: "security.block_sender",
		ResourceType: "sender_block", ResourceID: blockID,
		Detail: map[string]any{"pattern": pattern, "kind": kind},
	})
	httpx.JSON(w, http.StatusCreated, map[string]string{"id": blockID, "pattern": pattern, "kind": kind})
}

// RemoveBlock deletes a sender block.
func (h *Handlers) RemoveBlock(w http.ResponseWriter, r *http.Request) {
	id := auth.IdentityFrom(r.Context())
	blockID := chi.URLParam(r, "id")
	ct, err := h.Pool.Exec(r.Context(),
		`DELETE FROM sender_blocks WHERE id = $1 AND tenant_id = $2`, blockID, id.TenantID)
	if err != nil {
		httpx.Internal(w, r)
		return
	}
	if ct.RowsAffected() == 0 {
		httpx.Error(w, r, http.StatusNotFound, "BLOCK_NOT_FOUND", "Sender block not found")
		return
	}
	h.Audit.Record(r.Context(), audit.Entry{
		TenantID: id.TenantID, ActorUserID: id.UserID, Action: "security.unblock_sender",
		ResourceType: "sender_block", ResourceID: blockID,
	})
	httpx.JSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
