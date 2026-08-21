// Package delivery implements the routing and delivery worker: it consumes
// email.accepted events from JetStream and delivers messages into the
// authoritative mail store (ADR-003). Local recipients receive an
// Email/import into their Stalwart mailbox (Inbox or Junk per security
// verdict), the sender gets a Sent copy the same way, and remote recipients
// are relayed through the mail core's SMTP submission queue.
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
	"github.com/yelnurq/email-server/internal/mailcore"
	"github.com/yelnurq/email-server/internal/mailservice"
	"github.com/yelnurq/email-server/internal/security"
	"github.com/yelnurq/email-server/internal/storage"
)

const (
	consumerName = "delivery"
	// Delivery now depends on the mail store being reachable, so retries
	// must outlast a realistic restart or outage: exponential backoff from
	// 5s reaches ~1h of total tolerance over 10 attempts.
	maxDeliver   = 10
	baseNakDelay = 5 * time.Second
	maxNakDelay  = 15 * time.Minute
	// ackWait covers rendering plus mail-store imports and SMTP submission
	// for every recipient of one message.
	ackWait = 60 * time.Second
)

// nakBackoff grows the redelivery delay with the attempt count. The first
// retry is quick so a message sent moments after a mailbox was created
// lands as soon as its provisioning job completes.
func nakBackoff(attempt uint64) time.Duration {
	if attempt <= 1 {
		return 2 * time.Second
	}
	d := baseNakDelay
	for i := uint64(1); i < attempt && d < maxNakDelay; i++ {
		d *= 2
	}
	if d > maxNakDelay {
		d = maxNakDelay
	}
	return d
}

// Worker consumes accepted messages and routes them into the mail store.
type Worker struct {
	Pool  *pgxpool.Pool
	NATS  *nats.Conn
	Log   *slog.Logger
	Mail  *mailservice.Service
	Store storage.ObjectStore
	// Scan is the security engine (deterministic rules + Rspamd/ClamAV).
	Scan *security.Engine
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
	// Reconcile the durable consumer: delivery now performs network I/O
	// (mail-store imports, SMTP submission), so it needs a longer ack wait
	// than earlier versions configured. PullSubscribe refuses to attach to a
	// consumer whose stored config differs, so update it first.
	wantCfg := &nats.ConsumerConfig{
		Durable:       consumerName,
		AckPolicy:     nats.AckExplicitPolicy,
		MaxDeliver:    maxDeliver,
		AckWait:       ackWait,
		FilterSubject: events.SubjectAccepted,
	}
	// AckWait cannot always be changed in place (JetStream rejects the
	// update on some server versions). Bind to whatever config the durable
	// consumer actually has rather than refusing to start: an existing
	// consumer with a shorter ack wait is safe, only less forgiving.
	bindExisting := false
	if info, err := js.ConsumerInfo(events.StreamName, consumerName); err == nil {
		bindExisting = true
		if info.Config.AckWait != ackWait || info.Config.MaxDeliver != maxDeliver {
			if _, err := js.UpdateConsumer(events.StreamName, wantCfg); err != nil {
				w.Log.Warn("keeping existing delivery consumer config",
					slog.String("ack_wait", info.Config.AckWait.String()),
					slog.String("error", err.Error()))
			}
		}
	}
	opts := []nats.SubOpt{nats.Bind(events.StreamName, consumerName)}
	if !bindExisting {
		opts = []nats.SubOpt{
			nats.AckExplicit(),
			nats.MaxDeliver(maxDeliver),
			nats.AckWait(ackWait),
		}
	}
	sub, err := js.PullSubscribe(events.SubjectAccepted, consumerName, opts...)
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
		var attempt uint64 = 1
		if meta != nil {
			attempt = meta.NumDelivered
		}
		delay := nakBackoff(attempt)
		log.Warn("delivery attempt failed, will retry",
			slog.String("error", err.Error()), slog.Duration("retry_in", delay))
		_ = m.NakWithDelay(delay)
	}
}

var errPermanent = errors.New("permanent failure")

type recipient struct {
	id, address string
}

// deliver routes one accepted message. Recipient rows are the idempotency
// guard: a redelivery only reprocesses recipients still pending. Mail-store
// imports used to be at-least-once (a crash between an import and its
// status commit duplicated one mailbox copy on retry); the pre-import
// Message-ID check via Mail.HasMessage closes that window — a redelivery
// that finds its Message-ID already in the target folder marks the
// recipient delivered without importing again.
func (w *Worker) deliver(ctx context.Context, p events.AcceptedPayload) error {
	tx, err := w.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var status, fromAddress, msgOrgID, subject, bodyText, rfcID string
	var sentImported *time.Time
	err = tx.QueryRow(ctx, `
		SELECT status, from_address::text, COALESCE(organization_id::text, ''),
		       subject, body_text, rfc_message_id, sent_imported_at
		FROM messages
		WHERE id = $1 AND tenant_id = $2
		FOR UPDATE`, p.MessageID, p.TenantID).
		Scan(&status, &fromAddress, &msgOrgID, &subject, &bodyText, &rfcID, &sentImported)
	if errors.Is(err, pgx.ErrNoRows) {
		return errPermanent // message vanished; never deliverable
	}
	if err != nil {
		return err
	}
	if status == "draft" {
		return errPermanent
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

	// Canonical RFC822 bytes: identical for every local copy, the Sent copy
	// and the outbound relay — and the input for the security scan.
	raw, err := mailservice.RenderMessage(ctx, tx, w.Store, p.MessageID)
	if err != nil {
		return err
	}

	// Security verdict decides inbox / junk folder / quarantine. A scan
	// deferral (attachments present, no malware scanner reachable) is a
	// transient error: the delivery retries with backoff per ADR-004.
	verdict, err := w.Scan.Evaluate(ctx, tx, p.TenantID, fromAddress, subject, bodyText, raw)
	if err != nil {
		return err
	}
	w.recordScan(ctx, tx, p.MessageID, verdict)
	targetFolder := "inbox"
	if verdict.Action == security.ActionSpam {
		targetFolder = "spam"
	}

	var remote []recipient
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
			if policyReason == "" {
				// Unknown address: local domains reject (unknown user);
				// foreign domains go to the mail core's outbound queue.
				local, err := w.isLocalDomain(ctx, tx, r.address)
				if err != nil {
					return err
				}
				if !local {
					remote = append(remote, r)
					continue
				}
				policyReason = "no such mailbox"
			}
			if err := w.finishRecipient(ctx, tx, p, msgOrgID, r.id, "", "failed", policyReason); err != nil {
				return err
			}
			continue
		}
		deliveredTo := ""
		var lastErr string
		for _, t := range targets {
			if !t.provisioned {
				// Retry until the provisioning job finishes; the message is
				// never lost and never wrongly bounced.
				return errNotProvisioned
			}
			// Crash-safe idempotency: a previous attempt may have imported
			// this copy and died before the status commit. The Message-ID
			// check is folder-scoped, so a legitimate Sent+Inbox self-mail
			// pair is never mistaken for a duplicate.
			if dup, derr := w.alreadyImported(ctx, t.address, targetFolder, rfcID); derr != nil {
				return derr // transient: retry the whole delivery
			} else if dup {
				deliveredTo = t.mailboxID
				continue
			}
			if _, err := w.Mail.Import(ctx, t.address, targetFolder, raw, false); err != nil {
				if errors.Is(err, mailservice.ErrUnavailable) || errors.Is(err, mailcore.ErrUnavailable) {
					return err // transient: retry the whole delivery
				}
				lastErr = "mail store rejected the message"
				w.Log.Error("mailbox import failed",
					slog.String("address", t.address), slog.String("error", err.Error()))
				continue
			}
			deliveredTo = t.mailboxID
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

	// Remote recipients: one submission through the mail core's queue.
	if len(remote) > 0 {
		rcpts := make([]string, len(remote))
		for i, r := range remote {
			rcpts[i] = r.address
		}
		if err := w.Mail.SubmitExternal(ctx, fromAddress, fromAddress, rcpts, raw); err != nil {
			return err // transient or auth issue: retry with backoff
		}
		for _, r := range remote {
			if err := w.finishRecipientEvent(ctx, tx, p, msgOrgID, r.id, "relayed", "", events.SubjectRelayed); err != nil {
				return err
			}
		}
	}

	// Sender's Sent copy (once per message).
	if sentImported == nil {
		var mbExists bool
		_ = tx.QueryRow(ctx, `
			SELECT EXISTS (SELECT 1 FROM mailboxes
			               WHERE address = $1 AND status = 'active'
			                 AND provisioning_status IN ('active', 'skipped'))`,
			fromAddress).Scan(&mbExists)
		if mbExists {
			// Same crash-safe guard as local delivery: sent_imported_at may
			// have been lost to a crash after a successful import.
			dup, derr := w.alreadyImported(ctx, fromAddress, "sent", rfcID)
			if derr != nil {
				return derr
			}
			if dup {
				if _, err := tx.Exec(ctx,
					`UPDATE messages SET sent_imported_at = now() WHERE id = $1`, p.MessageID); err != nil {
					return err
				}
			} else if _, err := w.Mail.Import(ctx, fromAddress, "sent", raw, true); err != nil {
				if errors.Is(err, mailservice.ErrUnavailable) || errors.Is(err, mailcore.ErrUnavailable) {
					return err
				}
				w.Log.Error("sent copy import failed", slog.String("error", err.Error()))
			} else {
				if _, err := tx.Exec(ctx,
					`UPDATE messages SET sent_imported_at = now() WHERE id = $1`, p.MessageID); err != nil {
					return err
				}
			}
		}
	}

	// Final message status from recipient outcomes.
	var delivered, relayed, failed, quarantined int
	if err := tx.QueryRow(ctx, `
		SELECT count(*) FILTER (WHERE status = 'delivered'),
		       count(*) FILTER (WHERE status = 'relayed'),
		       count(*) FILTER (WHERE status = 'failed'),
		       count(*) FILTER (WHERE status = 'quarantined')
		FROM message_recipients WHERE message_id = $1`, p.MessageID).
		Scan(&delivered, &relayed, &failed, &quarantined); err != nil {
		return err
	}
	final := "delivered"
	switch {
	case delivered+relayed == 0 && quarantined > 0:
		final = "quarantined"
	case delivered+relayed == 0:
		final = "failed"
	case failed > 0 || quarantined > 0:
		final = "partially_delivered"
	}
	if _, err := tx.Exec(ctx, `UPDATE messages SET status = $1 WHERE id = $2`, final, p.MessageID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// alreadyImported is the crash-dedupe guard around mail-store imports.
// Transient store failures propagate (the whole delivery retries); any
// other guard failure degrades to "not imported" so delivery falls back to
// the previous at-least-once behavior instead of blocking mail flow.
func (w *Worker) alreadyImported(ctx context.Context, address, folder, rfcID string) (bool, error) {
	if rfcID == "" {
		return false, nil
	}
	dup, err := w.Mail.HasMessage(ctx, address, folder, rfcID)
	if err != nil {
		if errors.Is(err, mailservice.ErrUnavailable) || errors.Is(err, mailcore.ErrUnavailable) {
			return false, err
		}
		w.Log.Warn("duplicate check unavailable, importing anyway",
			slog.String("address", address), slog.String("error", err.Error()))
		return false, nil
	}
	return dup, nil
}

type deliveryTarget struct {
	mailboxID, address string
	// provisioned is false while the mailbox account has not been created
	// in the mail store yet (provisioning runs asynchronously).
	provisioned bool
}

// errNotProvisioned means a target mailbox exists in the control plane but
// its mail-store account is still being provisioned. Delivery retries until
// the provisioning job completes rather than bouncing the message.
var errNotProvisioned = errors.New("recipient mailbox is not provisioned yet")

// isLocalDomain reports whether the address's domain is served by this
// platform (mail domains are globally unique across tenants).
func (w *Worker) isLocalDomain(ctx context.Context, tx pgx.Tx, address string) (bool, error) {
	at := strings.LastIndexByte(address, '@')
	if at < 0 {
		return false, nil
	}
	var exists bool
	err := tx.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM domains WHERE name = $1)`, address[at+1:]).Scan(&exists)
	return exists, err
}

// resolveTargets maps an address to local delivery targets: a direct
// mailbox, the targets of an active alias, or the members of an active
// group. Aliases and groups target mailboxes only (no nesting), so
// resolution depth is 1 and loops are structurally impossible. policyReason
// is set when an address exists but a policy blocks delivery.
func (w *Worker) resolveTargets(ctx context.Context, tx pgx.Tx, tenantID, address, senderOrgID string) (targets []deliveryTarget, policyReason string, err error) {
	var t deliveryTarget
	var provStatus string
	err = tx.QueryRow(ctx, `
		SELECT m.id, m.address::text, m.provisioning_status FROM mailboxes m
		JOIN domains d ON d.id = m.domain_id
		WHERE m.address = $1 AND m.status = 'active' AND d.status = 'verified'`,
		address).Scan(&t.mailboxID, &t.address, &provStatus)
	if err == nil {
		t.provisioned = provStatus == "active" || provStatus == "skipped"
		return []deliveryTarget{t}, "", nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return nil, "", err
	}

	rows, err := tx.Query(ctx, `
		SELECT m.id, m.address::text, m.provisioning_status
		FROM mailbox_aliases a
		JOIN mailbox_alias_targets tt ON tt.alias_id = a.id
		JOIN mailboxes m ON m.id = tt.mailbox_id AND m.status = 'active'
		WHERE a.address = $1 AND a.status = 'active'`, address)
	if err != nil {
		return nil, "", err
	}
	for rows.Next() {
		var t deliveryTarget
		var ps string
		if err := rows.Scan(&t.mailboxID, &t.address, &ps); err != nil {
			rows.Close()
			return nil, "", err
		}
		t.provisioned = ps == "active" || ps == "skipped"
		targets = append(targets, t)
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
		SELECT m.id, m.address::text, m.provisioning_status
		FROM mail_group_members gm
		JOIN mailboxes m ON m.id = gm.mailbox_id AND m.status = 'active'
		WHERE gm.group_id = $1`, groupID)
	if err != nil {
		return nil, "", err
	}
	defer gRows.Close()
	for gRows.Next() {
		var t deliveryTarget
		var ps string
		if err := gRows.Scan(&t.mailboxID, &t.address, &ps); err != nil {
			return nil, "", err
		}
		t.provisioned = ps == "active" || ps == "skipped"
		targets = append(targets, t)
	}
	if len(targets) == 0 {
		return nil, "group has no active members", nil
	}
	return targets, "", gRows.Err()
}

// recordScan persists the security verdict as message metadata and a trace
// event (§41, §65). Best effort: a metadata write failure never blocks
// delivery, and the scan event is written once per message.
func (w *Worker) recordScan(ctx context.Context, tx pgx.Tx, messageID string, v security.Verdict) {
	meta, err := json.Marshal(v)
	if err != nil {
		return
	}
	if _, err := tx.Exec(ctx, `
		UPDATE messages SET security_scan = $2 WHERE id = $1 AND security_scan IS NULL`,
		messageID, meta); err != nil {
		w.Log.Warn("security scan metadata write failed", slog.String("error", err.Error()))
		return
	}
	detail, _ := json.Marshal(map[string]any{
		"score": v.Score, "action": v.Action, "signals": v.Signals,
		"rspamd_score": v.RspamdScore, "rspamd_action": v.RspamdAction,
	})
	_, _ = tx.Exec(ctx, `
		INSERT INTO message_events (message_id, type, detail)
		SELECT $1, 'email.scanned', $2
		WHERE NOT EXISTS (
			SELECT 1 FROM message_events WHERE message_id = $1 AND type = 'email.scanned'
		)`, messageID, detail)
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
	reason := verdict.Reason
	if reason == "" {
		reason = strings.Join(verdict.Signals, ", ")
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO quarantine_items (tenant_id, message_id, recipient_id, recipient_address,
		                              mailbox_id, reason, signals, risk_score)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT (message_id, recipient_id) DO NOTHING`,
		p.TenantID, p.MessageID, recipientID, address, mailboxID,
		reason, signals, verdict.Score); err != nil {
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

func (w *Worker) finishRecipient(ctx context.Context, tx pgx.Tx, p events.AcceptedPayload, orgID, recipientID, mailboxID, status, errMsg string) error {
	eventType := events.SubjectDeliveredLocal
	if status == "failed" {
		eventType = events.SubjectFailed
	}
	return w.finishRecipientWith(ctx, tx, p, orgID, recipientID, mailboxID, status, errMsg, eventType)
}

func (w *Worker) finishRecipientEvent(ctx context.Context, tx pgx.Tx, p events.AcceptedPayload, orgID, recipientID, status, errMsg, eventType string) error {
	return w.finishRecipientWith(ctx, tx, p, orgID, recipientID, "", status, errMsg, eventType)
}

func (w *Worker) finishRecipientWith(ctx context.Context, tx pgx.Tx, p events.AcceptedPayload, orgID, recipientID, mailboxID, status, errMsg, eventType string) error {
	var mb *string
	if mailboxID != "" {
		mb = &mailboxID
	}
	if _, err := tx.Exec(ctx, `
		UPDATE message_recipients SET status = $1, error = $2, mailbox_id = COALESCE($3, mailbox_id)
		WHERE id = $4`, status, errMsg, mb, recipientID); err != nil {
		return err
	}
	detail := map[string]any{"recipient_id": recipientID}
	if errMsg != "" {
		detail["error"] = errMsg
	}
	rawDetail, _ := json.Marshal(detail)
	if _, err := tx.Exec(ctx, `
		INSERT INTO message_events (message_id, type, detail) VALUES ($1, $2, $3)`,
		p.MessageID, eventType, rawDetail); err != nil {
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
