package mailservice

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/yelnurq/email-server/internal/storage"
)

// Queryer covers pgx.Tx and *pgxpool.Pool for read-only rendering.
type Queryer interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// RenderMessage reconstructs the canonical RFC822 bytes of an accepted
// message from the control-plane metadata and attachment store. The same
// bytes are imported into every recipient mailbox, the sender's Sent folder
// and (for remote recipients) handed to SMTP submission, so all views of
// the message are identical.
func RenderMessage(ctx context.Context, q Queryer, store storage.ObjectStore, messageID string) ([]byte, error) {
	var env Envelope
	var created time.Time
	var hasAttachments bool
	err := q.QueryRow(ctx, `
		SELECT from_address::text, from_display, subject, body_text, body_html,
		       rfc_message_id, in_reply_to, references_ids, has_attachments, created_at
		FROM messages WHERE id = $1`, messageID).
		Scan(&env.From, &env.FromDisplay, &env.Subject, &env.Text, &env.HTML,
			&env.RFCID, &env.InReplyTo, &env.References, &hasAttachments, &created)
	if err != nil {
		return nil, err
	}
	env.Date = created.UTC()

	rows, err := q.Query(ctx, `
		SELECT kind, address::text FROM message_recipients
		WHERE message_id = $1 ORDER BY created_at`, messageID)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var kind, addr string
		if err := rows.Scan(&kind, &addr); err != nil {
			rows.Close()
			return nil, err
		}
		switch kind {
		case "to":
			env.To = append(env.To, addr)
		case "cc":
			env.Cc = append(env.Cc, addr)
			// bcc never appears in headers
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	if hasAttachments {
		aRows, err := q.Query(ctx, `
			SELECT storage_key, filename, content_type FROM attachments
			WHERE message_id = $1 ORDER BY created_at`, messageID)
		if err != nil {
			return nil, err
		}
		type attMeta struct{ key, name, ctype string }
		var metas []attMeta
		for aRows.Next() {
			var m attMeta
			if err := aRows.Scan(&m.key, &m.name, &m.ctype); err != nil {
				aRows.Close()
				return nil, err
			}
			metas = append(metas, m)
		}
		aRows.Close()
		if err := aRows.Err(); err != nil {
			return nil, err
		}
		for _, m := range metas {
			rc, err := store.Get(ctx, m.key)
			if err != nil {
				return nil, fmt.Errorf("attachment %s: %w", m.name, err)
			}
			data, err := io.ReadAll(io.LimitReader(rc, 64<<20))
			rc.Close()
			if err != nil {
				return nil, fmt.Errorf("attachment %s: %w", m.name, err)
			}
			env.Attachments = append(env.Attachments, AttachmentPart{
				Filename: m.name, ContentType: m.ctype, Data: data,
			})
		}
	}
	return BuildMessage(env), nil
}
