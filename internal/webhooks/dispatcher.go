package webhooks

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nats-io/nats.go"

	"github.com/yelnurq/email-server/internal/events"
)

const (
	consumerName = "webhooks"
	maxAttempts  = 8
)

// backoff returns the delay before the given attempt number (1-based).
func backoff(attempt int) time.Duration {
	schedule := []time.Duration{
		5 * time.Second, 15 * time.Second, time.Minute, 5 * time.Minute,
		15 * time.Minute, time.Hour, 2 * time.Hour, 6 * time.Hour,
	}
	if attempt <= 0 {
		return schedule[0]
	}
	if attempt > len(schedule) {
		return schedule[len(schedule)-1]
	}
	return schedule[attempt-1]
}

// Dispatcher consumes mail events and delivers them to subscribed endpoints.
type Dispatcher struct {
	Pool   *pgxpool.Pool
	NATS   *nats.Conn
	Log    *slog.Logger
	Client *http.Client
}

// Run starts the fanout consumer and the delivery loop; blocks until ctx ends.
func (d *Dispatcher) Run(ctx context.Context) error {
	if d.Client == nil {
		d.Client = &http.Client{Timeout: 10 * time.Second}
	}
	js, err := d.NATS.JetStream()
	if err != nil {
		return err
	}
	sub, err := js.PullSubscribe(events.SubjectPrefix+">", consumerName,
		nats.AckExplicit(),
		nats.MaxDeliver(5),
		nats.AckWait(30*time.Second),
	)
	if err != nil {
		return err
	}
	go d.senderLoop(ctx)
	d.Log.Info("webhook dispatcher consuming", slog.String("subject", events.SubjectPrefix+">"))

	for ctx.Err() == nil {
		msgs, err := sub.Fetch(20, nats.MaxWait(2*time.Second))
		if err != nil {
			if errors.Is(err, nats.ErrTimeout) || errors.Is(err, context.DeadlineExceeded) || ctx.Err() != nil {
				continue
			}
			d.Log.Error("webhook fetch failed", slog.String("error", err.Error()))
			select {
			case <-time.After(2 * time.Second):
			case <-ctx.Done():
			}
			continue
		}
		for _, m := range msgs {
			if err := d.fanout(ctx, m); err != nil {
				d.Log.Error("webhook fanout failed", slog.String("error", err.Error()))
				_ = m.NakWithDelay(5 * time.Second)
				continue
			}
			_ = m.Ack()
		}
	}
	return nil
}

// envelope is what subscribers receive as the request body.
type envelope struct {
	ID        string          `json:"id"`
	Type      string          `json:"type"`
	CreatedAt string          `json:"created_at"`
	Data      json.RawMessage `json:"data"`
}

// fanout records one pending delivery per matching webhook. Idempotent via
// the (webhook_id, event_id) unique constraint.
func (d *Dispatcher) fanout(ctx context.Context, m *nats.Msg) error {
	var meta struct {
		TenantID       string `json:"tenant_id"`
		OrganizationID string `json:"organization_id"`
	}
	if err := json.Unmarshal(m.Data, &meta); err != nil || meta.TenantID == "" {
		return nil // not a routable event; drop silently
	}
	eventID := m.Header.Get(nats.MsgIdHdr)
	if eventID == "" {
		if md, err := m.Metadata(); err == nil {
			eventID = "seq-" + itoa(int64(md.Sequence.Stream))
		} else {
			return nil
		}
	}

	rows, err := d.Pool.Query(ctx, `
		SELECT id FROM webhooks
		WHERE tenant_id = $1 AND enabled
		  AND (organization_id::text = $2 OR $2 = '')
		  AND $3 = ANY(events)`,
		meta.TenantID, meta.OrganizationID, m.Subject)
	if err != nil {
		return err
	}
	var hookIDs []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		hookIDs = append(hookIDs, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}

	env := envelope{
		ID:        eventID,
		Type:      m.Subject,
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
		Data:      json.RawMessage(m.Data),
	}
	payload, err := json.Marshal(env)
	if err != nil {
		return err
	}
	for _, hookID := range hookIDs {
		if _, err := d.Pool.Exec(ctx, `
			INSERT INTO webhook_deliveries (webhook_id, event_id, event_type, payload)
			VALUES ($1, $2, $3, $4)
			ON CONFLICT (webhook_id, event_id) DO NOTHING`,
			hookID, eventID, m.Subject, payload); err != nil {
			return err
		}
	}
	return nil
}

// senderLoop pushes due deliveries with retries until ctx ends.
func (d *Dispatcher) senderLoop(ctx context.Context) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			for {
				worked, err := d.sendOne(ctx)
				if err != nil {
					if ctx.Err() == nil {
						d.Log.Error("webhook send cycle failed", slog.String("error", err.Error()))
					}
					break
				}
				if !worked {
					break
				}
			}
		}
	}
}

// sendOne claims and attempts a single due delivery. Returns false when the
// queue is drained.
func (d *Dispatcher) sendOne(ctx context.Context) (bool, error) {
	tx, err := d.Pool.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer tx.Rollback(ctx)

	var (
		deliveryID int64
		attempts   int
		payload    []byte
		url        string
		secret     string
		eventType  string
		eventID    string
	)
	err = tx.QueryRow(ctx, `
		SELECT wd.id, wd.attempts, wd.payload, wd.event_type, wd.event_id, w.url, w.secret
		FROM webhook_deliveries wd
		JOIN webhooks w ON w.id = wd.webhook_id
		WHERE wd.status = 'pending' AND wd.next_attempt_at <= now() AND w.enabled
		ORDER BY wd.next_attempt_at
		LIMIT 1
		FOR UPDATE OF wd SKIP LOCKED`).
		Scan(&deliveryID, &attempts, &payload, &eventType, &eventID, &url, &secret)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}

	code, sendErr := d.attempt(ctx, url, secret, eventType, eventID, payload)
	attempts++
	switch {
	case sendErr == nil && code >= 200 && code < 300:
		_, err = tx.Exec(ctx, `
			UPDATE webhook_deliveries
			SET status = 'delivered', attempts = $1, last_status_code = $2,
			    last_error = '', delivered_at = now()
			WHERE id = $3`, attempts, code, deliveryID)
	case attempts >= maxAttempts:
		_, err = tx.Exec(ctx, `
			UPDATE webhook_deliveries
			SET status = 'failed', attempts = $1, last_status_code = $2, last_error = $3
			WHERE id = $4`, attempts, nullableCode(code), errText(sendErr, code), deliveryID)
	default:
		_, err = tx.Exec(ctx, `
			UPDATE webhook_deliveries
			SET attempts = $1, last_status_code = $2, last_error = $3,
			    next_attempt_at = now() + $4::interval
			WHERE id = $5`,
			attempts, nullableCode(code), errText(sendErr, code),
			backoff(attempts).String(), deliveryID)
	}
	if err != nil {
		return false, err
	}
	return true, tx.Commit(ctx)
}

func nullableCode(code int) any {
	if code == 0 {
		return nil
	}
	return code
}

func errText(err error, code int) string {
	if err != nil {
		msg := err.Error()
		if len(msg) > 500 {
			msg = msg[:500]
		}
		return msg
	}
	if code >= 300 {
		return "non-2xx response"
	}
	return ""
}

// attempt performs one signed POST.
func (d *Dispatcher) attempt(ctx context.Context, url, secret, eventType, eventID string, payload []byte) (int, error) {
	reqCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return 0, err
	}
	ts := time.Now().UTC().Unix()
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "MailPlatform-Webhooks/1.0")
	req.Header.Set("X-Mailplatform-Event", eventType)
	req.Header.Set("X-Mailplatform-Event-Id", eventID)
	req.Header.Set("X-Mailplatform-Timestamp", itoa(ts))
	req.Header.Set("X-Mailplatform-Signature", Sign(secret, ts, payload))

	resp, err := d.Client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
	return resp.StatusCode, nil
}

func itoa(n int64) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [21]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
