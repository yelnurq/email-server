// Package events owns the NATS JetStream topology and the transactional
// outbox publisher. State changes and their events are committed in one
// PostgreSQL transaction (outbox table); a background publisher moves them
// to JetStream, so an accepted message can never lose its event.
package events

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nats-io/nats.go"
)

// Stream configuration for the mail plane.
const (
	StreamName    = "EMAIL"
	SubjectPrefix = "email."

	SubjectAccepted       = "email.accepted"
	SubjectDeliveredLocal = "email.delivered_local"
	SubjectFailed         = "email.failed"
)

// AcceptedPayload is the body of an email.accepted event.
type AcceptedPayload struct {
	MessageID      string `json:"message_id"` // internal uuid
	PublicID       string `json:"public_id"`
	TenantID       string `json:"tenant_id"`
	OrganizationID string `json:"organization_id,omitempty"`
}

// DeliveryPayload is the body of email.delivered_local / email.failed events.
type DeliveryPayload struct {
	MessageID      string `json:"message_id"` // internal uuid
	PublicID       string `json:"public_id"`
	TenantID       string `json:"tenant_id"`
	OrganizationID string `json:"organization_id,omitempty"`
	RecipientID    string `json:"recipient_id"`
	Status         string `json:"status"`
	Error          string `json:"error,omitempty"`
}

// EnsureStream creates or updates the EMAIL stream. Idempotent.
func EnsureStream(nc *nats.Conn) error {
	js, err := nc.JetStream()
	if err != nil {
		return err
	}
	cfg := &nats.StreamConfig{
		Name:      StreamName,
		Subjects:  []string{SubjectPrefix + ">"},
		Retention: nats.LimitsPolicy,
		Storage:   nats.FileStorage,
		MaxAge:    7 * 24 * time.Hour,
	}
	if _, err := js.StreamInfo(StreamName); err != nil {
		_, err = js.AddStream(cfg)
		return err
	}
	_, err = js.UpdateStream(cfg)
	return err
}

// Enqueue writes an event into the outbox inside the caller's transaction.
func Enqueue(ctx context.Context, tx pgx.Tx, subject string, payload any) error {
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO outbox (subject, payload) VALUES ($1, $2)`, subject, raw)
	return err
}

// Publisher drains the outbox to JetStream.
type Publisher struct {
	Pool *pgxpool.Pool
	NATS *nats.Conn
	Log  *slog.Logger
}

// Run polls the outbox until ctx is cancelled. Rows are locked with
// SKIP LOCKED so multiple API instances can run publishers concurrently.
func (p *Publisher) Run(ctx context.Context) {
	ticker := time.NewTicker(300 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := p.drainOnce(ctx); err != nil && ctx.Err() == nil {
				p.Log.Error("outbox drain failed", slog.String("error", err.Error()))
			}
		}
	}
}

func (p *Publisher) drainOnce(ctx context.Context) error {
	js, err := p.NATS.JetStream()
	if err != nil {
		return err
	}
	for {
		tx, err := p.Pool.Begin(ctx)
		if err != nil {
			return err
		}
		var id int64
		var subject string
		var payload []byte
		err = tx.QueryRow(ctx, `
			SELECT id, subject, payload FROM outbox
			WHERE published_at IS NULL
			ORDER BY id
			LIMIT 1
			FOR UPDATE SKIP LOCKED`).Scan(&id, &subject, &payload)
		if err != nil {
			tx.Rollback(ctx)
			if err == pgx.ErrNoRows {
				return nil // drained
			}
			return err
		}
		// Deduplicate on the broker side via Nats-Msg-Id in case the row is
		// published but the transaction fails to commit afterwards.
		msg := nats.NewMsg(subject)
		msg.Data = payload
		msg.Header.Set(nats.MsgIdHdr, "outbox-"+itoa(id))
		if _, err := js.PublishMsg(msg); err != nil {
			tx.Rollback(ctx)
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE outbox SET published_at = now() WHERE id = $1`, id); err != nil {
			tx.Rollback(ctx)
			return err
		}
		if err := tx.Commit(ctx); err != nil {
			return err
		}
	}
}

func itoa(n int64) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}
