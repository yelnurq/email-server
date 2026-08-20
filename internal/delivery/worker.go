// Package delivery implements the local routing and delivery worker: it
// consumes email.accepted events from JetStream and delivers messages into
// recipient mailboxes (direct addresses and alias targets) idempotently.
package delivery

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nats-io/nats.go"

	"github.com/yelnurq/email-server/internal/events"
)

const (
	consumerName = "delivery"
	maxDeliver   = 5
	nakDelay     = 5 * time.Second
)

// Worker consumes accepted messages and routes them locally.
type Worker struct {
	Pool *pgxpool.Pool
	NATS *nats.Conn
	Log  *slog.Logger
}

// Run subscribes the durable consumer and blocks until ctx is cancelled.
func (w *Worker) Run(ctx context.Context) error {
	if err := events.EnsureStream(w.NATS); err != nil {
		return err
	}
	js, err := w.NATS.JetStream()
	if err != nil {
		return err
	}
	sub, err := js.PullSubscribe(events.SubjectAccepted, consumerName,
		nats.AckExplicit(),
		nats.MaxDeliver(maxDeliver),
		nats.AckWait(30*time.Second),
	)
	if err != nil {
		return err
	}
	w.Log.Info("delivery worker consuming", slog.String("subject", events.SubjectAccepted))

	for ctx.Err() == nil {
		msgs, err := sub.Fetch(10, nats.MaxWait(2*time.Second))
		if err != nil {
			if errors.Is(err, nats.ErrTimeout) || errors.Is(err, context.DeadlineExceeded) {
				continue
			}
			if ctx.Err() != nil {
				break
			}
			w.Log.Error("fetch failed", slog.String("error", err.Error()))
			select {
			case <-time.After(2 * time.Second):
			case <-ctx.Done():
			}
			continue
		}
		for _, m := range msgs {
			w.handle(ctx, m)
		}
	}
	return nil
}

func (w *Worker) handle(ctx context.Context, m *nats.Msg) {
	var payload events.AcceptedPayload
	if err := json.Unmarshal(m.Data, &payload); err != nil {
		// Poison message: never parseable, terminate it.
		w.Log.Error("poison message terminated", slog.String("error", err.Error()))
		_ = m.Term()
		return
	}
	log := w.Log.With(slog.String("message_id", payload.PublicID))

	err := w.deliver(ctx, payload)
	switch {
	case err == nil:
		_ = m.Ack()
	case errors.Is(err, errPermanent):
		log.Error("permanent delivery failure", slog.String("error", err.Error()))
		_ = m.Term()
	default:
		meta, _ := m.Metadata()
		if meta != nil && meta.NumDelivered >= maxDeliver {
			log.Error("delivery retries exhausted", slog.String("error", err.Error()))
			w.markFailed(ctx, payload, "delivery retries exhausted")
			_ = m.Term()
			return
		}
		log.Warn("delivery attempt failed, will retry", slog.String("error", err.Error()))
		_ = m.NakWithDelay(nakDelay)
	}
}

var errPermanent = errors.New("permanent failure")

// deliver routes one accepted message. Fully idempotent: redelivery skips
// recipients that are no longer pending and mailbox copies conflict-away on
// the (mailbox_id, message_id) unique constraint.
func (w *Worker) deliver(ctx context.Context, p events.AcceptedPayload) error {
	tx, err := w.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var size int64
	var status string
	err = tx.QueryRow(ctx, `
		SELECT size_bytes, status FROM messages WHERE id = $1 AND tenant_id = $2
		FOR UPDATE`, p.MessageID, p.TenantID).Scan(&size, &status)
	if errors.Is(err, pgx.ErrNoRows) {
		return errPermanent // message vanished; never deliverable
	}
	if err != nil {
		return err
	}
	if status == "draft" {
		return errPermanent
	}

	type recipient struct {
		id, address string
	}
	rows, err := tx.Query(ctx, `
		SELECT id, address::text FROM message_recipients
		WHERE message_id = $1 AND status = 'pending'
		FOR UPDATE`, p.MessageID)
	if err != nil {
		return err
	}
	var pending []recipient
	for rows.Next() {
		var r recipient
		if err := rows.Scan(&r.id, &r.address); err != nil {
			rows.Close()
			return err
		}
		pending = append(pending, r)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}

	for _, r := range pending {
		targets, err := w.resolveTargets(ctx, tx, p.TenantID, r.address)
		if err != nil {
			return err
		}
		if len(targets) == 0 {
			if err := w.finishRecipient(ctx, tx, p.MessageID, r.id, "", "failed", "no such mailbox"); err != nil {
				return err
			}
			continue
		}
		deliveredTo := ""
		var lastErr string
		for _, t := range targets {
			ok, reason, err := w.deliverToMailbox(ctx, tx, t, p.MessageID, size)
			if err != nil {
				return err
			}
			if ok {
				deliveredTo = t
			} else {
				lastErr = reason
			}
		}
		if deliveredTo != "" {
			if err := w.finishRecipient(ctx, tx, p.MessageID, r.id, deliveredTo, "delivered", ""); err != nil {
				return err
			}
		} else {
			if err := w.finishRecipient(ctx, tx, p.MessageID, r.id, "", "failed", lastErr); err != nil {
				return err
			}
		}
	}

	// Final message status from recipient outcomes.
	var delivered, failed int
	if err := tx.QueryRow(ctx, `
		SELECT count(*) FILTER (WHERE status = 'delivered'),
		       count(*) FILTER (WHERE status = 'failed')
		FROM message_recipients WHERE message_id = $1`, p.MessageID).
		Scan(&delivered, &failed); err != nil {
		return err
	}
	final := "delivered"
	switch {
	case delivered == 0:
		final = "failed"
	case failed > 0:
		final = "partially_delivered"
	}
	if _, err := tx.Exec(ctx, `UPDATE messages SET status = $1 WHERE id = $2`, final, p.MessageID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// resolveTargets maps an address to local mailbox ids: a direct mailbox or
// the targets of an active alias. Unknown addresses return no targets (in
// Phase 1 there is no internet routing branch yet).
func (w *Worker) resolveTargets(ctx context.Context, tx pgx.Tx, tenantID, address string) ([]string, error) {
	var mailboxID string
	err := tx.QueryRow(ctx, `
		SELECT m.id FROM mailboxes m
		JOIN domains d ON d.id = m.domain_id
		WHERE m.address = $1 AND m.status = 'active' AND d.status = 'verified'`,
		address).Scan(&mailboxID)
	if err == nil {
		return []string{mailboxID}, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return nil, err
	}
	rows, err := tx.Query(ctx, `
		SELECT t.mailbox_id
		FROM mailbox_aliases a
		JOIN mailbox_alias_targets t ON t.alias_id = a.id
		JOIN mailboxes m ON m.id = t.mailbox_id AND m.status = 'active'
		WHERE a.address = $1 AND a.status = 'active'`, address)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var targets []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		targets = append(targets, id)
	}
	return targets, rows.Err()
}

// deliverToMailbox inserts the inbox copy and accounts quota. Returns
// ok=false with a reason for policy failures (quota).
func (w *Worker) deliverToMailbox(ctx context.Context, tx pgx.Tx, mailboxID, messageID string, size int64) (bool, string, error) {
	var quota, used int64
	if err := tx.QueryRow(ctx, `
		SELECT quota_bytes, used_bytes FROM mailboxes WHERE id = $1 FOR UPDATE`,
		mailboxID).Scan(&quota, &used); err != nil {
		return false, "", err
	}
	if used+size > quota {
		return false, "quota exceeded", nil
	}
	var inboxID string
	if err := tx.QueryRow(ctx, `
		SELECT id FROM folders WHERE mailbox_id = $1 AND type = 'inbox'`,
		mailboxID).Scan(&inboxID); err != nil {
		return false, "", err
	}
	ct, err := tx.Exec(ctx, `
		INSERT INTO mailbox_messages (mailbox_id, message_id, folder_id)
		VALUES ($1, $2, $3)
		ON CONFLICT (mailbox_id, message_id) DO NOTHING`,
		mailboxID, messageID, inboxID)
	if err != nil {
		return false, "", err
	}
	if ct.RowsAffected() > 0 {
		if _, err := tx.Exec(ctx, `
			UPDATE mailboxes SET used_bytes = used_bytes + $1 WHERE id = $2`,
			size, mailboxID); err != nil {
			return false, "", err
		}
	}
	return true, "", nil
}

func (w *Worker) finishRecipient(ctx context.Context, tx pgx.Tx, messageID, recipientID, mailboxID, status, errMsg string) error {
	var mb *string
	if mailboxID != "" {
		mb = &mailboxID
	}
	if _, err := tx.Exec(ctx, `
		UPDATE message_recipients SET status = $1, error = $2, mailbox_id = COALESCE($3, mailbox_id)
		WHERE id = $4`, status, errMsg, mb, recipientID); err != nil {
		return err
	}
	eventType := events.SubjectDeliveredLocal
	detail := map[string]any{"recipient_id": recipientID}
	if status == "failed" {
		eventType = events.SubjectFailed
		detail["error"] = errMsg
	}
	raw, _ := json.Marshal(detail)
	if _, err := tx.Exec(ctx, `
		INSERT INTO message_events (message_id, type, detail) VALUES ($1, $2, $3)`,
		messageID, eventType, raw); err != nil {
		return err
	}
	return events.Enqueue(ctx, tx, eventType, map[string]any{
		"message_id": messageID, "recipient_id": recipientID, "status": status,
	})
}

// markFailed records terminal failure after exhausted retries (best effort).
func (w *Worker) markFailed(ctx context.Context, p events.AcceptedPayload, reason string) {
	_, err := w.Pool.Exec(ctx, `
		UPDATE messages SET status = 'failed' WHERE id = $1 AND status IN ('accepted', 'processing')`,
		p.MessageID)
	if err != nil {
		w.Log.Error("mark failed errored", slog.String("error", err.Error()))
	}
	_, _ = w.Pool.Exec(ctx, `
		INSERT INTO message_events (message_id, type, detail)
		VALUES ($1, 'email.failed', jsonb_build_object('error', $2::text))`,
		p.MessageID, reason)
}
