// Package delivery implements the local routing and delivery worker: it
// consumes email.accepted events from JetStream and delivers messages into
// recipient mailboxes (direct addresses and alias targets) idempotently.
package delivery

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nats-io/nats.go"

	"github.com/yelnurq/email-server/internal/events"
	"github.com/yelnurq/email-server/internal/security"
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
// the (mailbox_id, message_id, folder_id) unique constraint. The folder is
// part of the key so a self-addressed message can hold both the sender's
// Sent copy and the delivered Inbox copy in the same mailbox.
func (w *Worker) deliver(ctx context.Context, p events.AcceptedPayload) error {
	tx, err := w.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var size int64
	var status, fromAddress, msgOrgID, subject, bodyText string
	err = tx.QueryRow(ctx, `
		SELECT size_bytes, status, from_address::text, COALESCE(organization_id::text, ''),
		       subject, body_text
		FROM messages
		WHERE id = $1 AND tenant_id = $2
		FOR UPDATE`, p.MessageID, p.TenantID).
		Scan(&size, &status, &fromAddress, &msgOrgID, &subject, &bodyText)
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

	// The sender's organization drives group sender-restriction policies.
	var senderOrgID string
	_ = tx.QueryRow(ctx,
		`SELECT organization_id FROM mailboxes WHERE address = $1`, fromAddress).Scan(&senderOrgID)

	// Security verdict decides inbox / spam folder / quarantine.
	verdict, err := security.Evaluate(ctx, tx, p.TenantID, fromAddress, subject, bodyText)
	if err != nil {
		return err
	}
	targetFolder := "inbox"
	if verdict.Action == security.ActionSpam {
		targetFolder = "spam"
	}

	for _, r := range pending {
		if verdict.Action == security.ActionQuarantine {
			if err := w.quarantineRecipient(ctx, tx, p, msgOrgID, r.id, r.address, verdict); err != nil {
				return err
			}
			continue
		}
		targets, policyReason, err := w.resolveTargets(ctx, tx, p.TenantID, r.address, senderOrgID)
		if err != nil {
			return err
		}
		if len(targets) == 0 {
			reason := "no such mailbox"
			if policyReason != "" {
				reason = policyReason
			}
			if err := w.finishRecipient(ctx, tx, p, msgOrgID, r.id, "", "failed", reason); err != nil {
				return err
			}
			continue
		}
		deliveredTo := ""
		var lastErr string
		for _, t := range targets {
			ok, reason, err := w.deliverToMailbox(ctx, tx, t, p.MessageID, size, targetFolder)
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
			if err := w.finishRecipient(ctx, tx, p, msgOrgID, r.id, deliveredTo, "delivered", ""); err != nil {
				return err
			}
		} else {
			if err := w.finishRecipient(ctx, tx, p, msgOrgID, r.id, "", "failed", lastErr); err != nil {
				return err
			}
		}
	}

	// Final message status from recipient outcomes.
	var delivered, failed, quarantined int
	if err := tx.QueryRow(ctx, `
		SELECT count(*) FILTER (WHERE status = 'delivered'),
		       count(*) FILTER (WHERE status = 'failed'),
		       count(*) FILTER (WHERE status = 'quarantined')
		FROM message_recipients WHERE message_id = $1`, p.MessageID).
		Scan(&delivered, &failed, &quarantined); err != nil {
		return err
	}
	final := "delivered"
	switch {
	case delivered == 0 && quarantined > 0:
		final = "quarantined"
	case delivered == 0:
		final = "failed"
	case failed > 0 || quarantined > 0:
		final = "partially_delivered"
	}
	if _, err := tx.Exec(ctx, `UPDATE messages SET status = $1 WHERE id = $2`, final, p.MessageID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// resolveTargets maps an address to local mailbox ids: a direct mailbox,
// the targets of an active alias, or the members of an active group. Aliases
// and groups target mailboxes only (no nesting), so resolution depth is 1 and
// loops are structurally impossible. Unknown addresses return no targets (in
// Phase 1 there is no internet routing branch yet). policyReason is set when
// an address exists but a policy (e.g. internal-only group) blocks delivery.
func (w *Worker) resolveTargets(ctx context.Context, tx pgx.Tx, tenantID, address, senderOrgID string) (targets []string, policyReason string, err error) {
	var mailboxID string
	err = tx.QueryRow(ctx, `
		SELECT m.id FROM mailboxes m
		JOIN domains d ON d.id = m.domain_id
		WHERE m.address = $1 AND m.status = 'active' AND d.status = 'verified'`,
		address).Scan(&mailboxID)
	if err == nil {
		return []string{mailboxID}, "", nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return nil, "", err
	}

	rows, err := tx.Query(ctx, `
		SELECT t.mailbox_id
		FROM mailbox_aliases a
		JOIN mailbox_alias_targets t ON t.alias_id = a.id
		JOIN mailboxes m ON m.id = t.mailbox_id AND m.status = 'active'
		WHERE a.address = $1 AND a.status = 'active'`, address)
	if err != nil {
		return nil, "", err
	}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return nil, "", err
		}
		targets = append(targets, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, "", err
	}
	if len(targets) > 0 {
		return targets, "", nil
	}

	// Group resolution with sender restriction.
	var groupID, groupOrgID string
	var internalOnly bool
	err = tx.QueryRow(ctx, `
		SELECT id, organization_id, internal_only FROM mail_groups
		WHERE address = $1 AND status = 'active'`, address).
		Scan(&groupID, &groupOrgID, &internalOnly)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, "", nil
	}
	if err != nil {
		return nil, "", err
	}
	if internalOnly && groupOrgID != senderOrgID {
		return nil, "group accepts internal senders only", nil
	}
	gRows, err := tx.Query(ctx, `
		SELECT gm.mailbox_id
		FROM mail_group_members gm
		JOIN mailboxes m ON m.id = gm.mailbox_id AND m.status = 'active'
		WHERE gm.group_id = $1`, groupID)
	if err != nil {
		return nil, "", err
	}
	defer gRows.Close()
	for gRows.Next() {
		var id string
		if err := gRows.Scan(&id); err != nil {
			return nil, "", err
		}
		targets = append(targets, id)
	}
	if len(targets) == 0 {
		return nil, "group has no active members", nil
	}
	return targets, "", gRows.Err()
}

// quarantineRecipient parks a recipient's copy in the security quarantine
// instead of delivering it. Idempotent via (message_id, recipient_id).
func (w *Worker) quarantineRecipient(ctx context.Context, tx pgx.Tx, p events.AcceptedPayload, orgID, recipientID, address string, verdict security.Verdict) error {
	var mailboxID *string
	var mb string
	if err := tx.QueryRow(ctx,
		`SELECT id FROM mailboxes WHERE address = $1`, address).Scan(&mb); err == nil {
		mailboxID = &mb
	}
	signals, _ := json.Marshal(verdict.Signals)
	if _, err := tx.Exec(ctx, `
		INSERT INTO quarantine_items (tenant_id, message_id, recipient_id, recipient_address,
		                              mailbox_id, reason, signals, risk_score)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT (message_id, recipient_id) DO NOTHING`,
		p.TenantID, p.MessageID, recipientID, address, mailboxID,
		strings.Join(verdict.Signals, ", "), signals, verdict.Score); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		UPDATE message_recipients SET status = 'quarantined', error = 'quarantined by security policy'
		WHERE id = $1`, recipientID); err != nil {
		return err
	}
	detail, _ := json.Marshal(map[string]any{
		"recipient_id": recipientID, "score": verdict.Score, "signals": verdict.Signals,
	})
	if _, err := tx.Exec(ctx, `
		INSERT INTO message_events (message_id, type, detail) VALUES ($1, $2, $3)`,
		p.MessageID, events.SubjectQuarantined, detail); err != nil {
		return err
	}
	return events.Enqueue(ctx, tx, events.SubjectQuarantined, events.DeliveryPayload{
		MessageID: p.MessageID, PublicID: p.PublicID, TenantID: p.TenantID,
		OrganizationID: orgID, RecipientID: recipientID, Status: "quarantined",
	})
}

// deliverToMailbox inserts the mailbox copy (inbox or spam) and accounts
// quota. Returns ok=false with a reason for policy failures (quota).
func (w *Worker) deliverToMailbox(ctx context.Context, tx pgx.Tx, mailboxID, messageID string, size int64, folderType string) (bool, string, error) {
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
		SELECT id FROM folders WHERE mailbox_id = $1 AND type = $2`,
		mailboxID, folderType).Scan(&inboxID); err != nil {
		return false, "", err
	}
	ct, err := tx.Exec(ctx, `
		INSERT INTO mailbox_messages (mailbox_id, message_id, folder_id)
		VALUES ($1, $2, $3)
		ON CONFLICT (mailbox_id, message_id, folder_id) DO NOTHING`,
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

func (w *Worker) finishRecipient(ctx context.Context, tx pgx.Tx, p events.AcceptedPayload, orgID, recipientID, mailboxID, status, errMsg string) error {
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
		p.MessageID, eventType, raw); err != nil {
		return err
	}
	return events.Enqueue(ctx, tx, eventType, events.DeliveryPayload{
		MessageID: p.MessageID, PublicID: p.PublicID, TenantID: p.TenantID,
		OrganizationID: orgID, RecipientID: recipientID, Status: status, Error: errMsg,
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
