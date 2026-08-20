// Package messages implements the message model: acceptance (sending),
// drafts and the webmail read/state API. Sending is not CRUD — an accepted
// message is persisted with its recipients and an email.accepted event in one
// transaction; the delivery worker routes it asynchronously via NATS.
package messages

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/yelnurq/email-server/internal/events"
	"github.com/yelnurq/email-server/internal/mailaddr"
)

// Limits for accepted messages.
const (
	MaxRecipients  = 100
	MaxSubjectLen  = 500
	MaxBodyBytes   = 1 << 20 // 1 MiB text body cap for Phase 1
	SnippetLen     = 160
	statusDraft    = "draft"
	statusAccepted = "accepted"
)

// Service persists messages and drafts.
type Service struct {
	Pool *pgxpool.Pool
}

// SenderMailbox describes the acting user's mailbox.
type SenderMailbox struct {
	ID      string
	Address string
	Domain  string
	OrgID   string
}

// ErrNoMailbox means the user has no active mailbox to send from.
var ErrNoMailbox = errors.New("user has no active mailbox")

// MailboxForUser returns the user's primary active mailbox.
func (s *Service) MailboxForUser(ctx context.Context, tenantID, userID string) (*SenderMailbox, error) {
	var m SenderMailbox
	err := s.Pool.QueryRow(ctx, `
		SELECT m.id, m.address::text, d.name::text, m.organization_id
		FROM mailboxes m
		JOIN domains d ON d.id = m.domain_id
		WHERE m.tenant_id = $1 AND m.user_id = $2 AND m.status = 'active'
		ORDER BY m.created_at
		LIMIT 1`, tenantID, userID).Scan(&m.ID, &m.Address, &m.Domain, &m.OrgID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNoMailbox
	}
	if err != nil {
		return nil, err
	}
	return &m, nil
}

// SendInput is a validated send request.
type SendInput struct {
	To      []string
	Cc      []string
	Bcc     []string
	Subject string
	Text    string
	// HTML is an optional HTML body (Email API); webmail sends text only.
	HTML string
	// InReplyTo is the public_id of the message being replied to (optional).
	InReplyTo string
	// AttachmentIDs are public ids of staged attachments uploaded by the
	// sending user.
	AttachmentIDs []string
	// SenderUserID authorizes attachment linking; empty for API-key sends
	// that never stage attachments this way.
	SenderUserID string
}

// ValidationError distinguishes user errors from internal failures.
type ValidationError struct{ Msg string }

func (e *ValidationError) Error() string { return e.Msg }

func normalizeRecipients(in SendInput) (to, cc, bcc []string, err error) {
	norm := func(list []string, seen map[string]bool) ([]string, error) {
		var out []string
		for _, a := range list {
			local, domain, err := mailaddr.NormalizeAddress(a)
			if err != nil {
				return nil, &ValidationError{Msg: fmt.Sprintf("invalid recipient address %q", a)}
			}
			addr := mailaddr.Join(local, domain)
			if !seen[addr] {
				seen[addr] = true
				out = append(out, addr)
			}
		}
		return out, nil
	}
	seen := map[string]bool{}
	if to, err = norm(in.To, seen); err != nil {
		return nil, nil, nil, err
	}
	if cc, err = norm(in.Cc, seen); err != nil {
		return nil, nil, nil, err
	}
	if bcc, err = norm(in.Bcc, seen); err != nil {
		return nil, nil, nil, err
	}
	if len(to) == 0 {
		return nil, nil, nil, &ValidationError{Msg: "at least one To recipient is required"}
	}
	if len(to)+len(cc)+len(bcc) > MaxRecipients {
		return nil, nil, nil, &ValidationError{Msg: fmt.Sprintf("too many recipients (max %d)", MaxRecipients)}
	}
	return to, cc, bcc, nil
}

func validateContent(subject, text string) error {
	if len(subject) > MaxSubjectLen {
		return &ValidationError{Msg: "subject too long"}
	}
	if len(text) > MaxBodyBytes {
		return &ValidationError{Msg: "message body too large"}
	}
	return nil
}

func makeSnippet(text string) string {
	fields := strings.Fields(text)
	s := strings.Join(fields, " ")
	if len(s) > SnippetLen {
		cut := s[:SnippetLen]
		if i := strings.LastIndexByte(cut, ' '); i > SnippetLen/2 {
			cut = cut[:i]
		}
		s = cut + "…"
	}
	return s
}

// Accept validates, persists and enqueues a message for delivery. Returns the
// public message id. The whole operation is one transaction: message,
// recipients, the sender's Sent copy, the accepted event and the outbox row.
func (s *Service) Accept(ctx context.Context, tenantID string, sender *SenderMailbox, senderDisplay string, in SendInput) (string, error) {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer tx.Rollback(ctx)

	publicID, err := s.AcceptInTx(ctx, tx, tenantID, sender, senderDisplay, in)
	if err != nil {
		return "", err
	}
	if err := tx.Commit(ctx); err != nil {
		return "", err
	}
	return publicID, nil
}

// AcceptInTx is Accept inside a caller-owned transaction (used by the Email
// API to combine acceptance with idempotency bookkeeping atomically).
func (s *Service) AcceptInTx(ctx context.Context, tx pgx.Tx, tenantID string, sender *SenderMailbox, senderDisplay string, in SendInput) (string, error) {
	to, cc, bcc, err := normalizeRecipients(in)
	if err != nil {
		return "", err
	}
	if err := validateContent(in.Subject, in.Text); err != nil {
		return "", err
	}
	if len(in.HTML) > MaxBodyBytes {
		return "", &ValidationError{Msg: "message html body too large"}
	}
	publicID, _, err := s.insertAccepted(ctx, tx, tenantID, sender, senderDisplay, in, to, cc, bcc)
	return publicID, err
}

// insertAccepted writes an accepted message inside tx and returns ids.
func (s *Service) insertAccepted(ctx context.Context, tx pgx.Tx, tenantID string, sender *SenderMailbox, senderDisplay string, in SendInput, to, cc, bcc []string) (publicID, msgID string, err error) {
	// Resolve threading.
	threadID, inReplyToRFC, referencesIDs := "", "", ""
	if in.InReplyTo != "" {
		var parentThread *string
		var parentRFC, parentRefs, parentSubject string
		err := tx.QueryRow(ctx, `
			SELECT thread_id, rfc_message_id, references_ids, subject
			FROM messages WHERE public_id = $1 AND tenant_id = $2`,
			in.InReplyTo, tenantID).Scan(&parentThread, &parentRFC, &parentRefs, &parentSubject)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return "", "", err
		}
		if err == nil {
			if parentThread != nil {
				threadID = *parentThread
			}
			inReplyToRFC = parentRFC
			referencesIDs = strings.TrimSpace(parentRefs + " " + parentRFC)
		}
	}
	if threadID == "" {
		if err := tx.QueryRow(ctx, `
			INSERT INTO threads (tenant_id, subject) VALUES ($1, $2) RETURNING id`,
			tenantID, in.Subject).Scan(&threadID); err != nil {
			return "", "", err
		}
	}

	publicID = NewPublicID()
	rfcID := NewRFCMessageID(sender.Domain)
	size := int64(len(in.Text) + len(in.HTML))
	snippetSource := in.Text
	if snippetSource == "" {
		snippetSource = in.HTML
	}
	var orgID *string
	if sender.OrgID != "" {
		orgID = &sender.OrgID
	}
	err = tx.QueryRow(ctx, `
		INSERT INTO messages (public_id, tenant_id, organization_id, thread_id, rfc_message_id,
		                      in_reply_to, references_ids, from_address, from_display, subject,
		                      snippet, body_text, body_html, size_bytes, status, sent_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, 'accepted', now())
		RETURNING id`,
		publicID, tenantID, orgID, threadID, rfcID, inReplyToRFC, referencesIDs,
		sender.Address, senderDisplay, in.Subject, makeSnippet(snippetSource),
		in.Text, in.HTML, size).Scan(&msgID)
	if err != nil {
		return "", "", err
	}

	// Link staged attachments (uploaded by the sender, not yet linked).
	if len(in.AttachmentIDs) > 0 {
		if len(in.AttachmentIDs) > 20 {
			return "", "", &ValidationError{Msg: "too many attachments (max 20)"}
		}
		ct, err := tx.Exec(ctx, `
			UPDATE attachments SET message_id = $1
			WHERE public_id = ANY($2) AND tenant_id = $3
			  AND uploader_user_id = $4 AND message_id IS NULL`,
			msgID, in.AttachmentIDs, tenantID, in.SenderUserID)
		if err != nil {
			return "", "", err
		}
		if int(ct.RowsAffected()) != len(in.AttachmentIDs) {
			return "", "", &ValidationError{Msg: "one or more attachments are unknown or already used"}
		}
		var attachSize int64
		if err := tx.QueryRow(ctx, `
			SELECT COALESCE(sum(size_bytes), 0) FROM attachments WHERE message_id = $1`,
			msgID).Scan(&attachSize); err != nil {
			return "", "", err
		}
		if _, err := tx.Exec(ctx, `
			UPDATE messages SET has_attachments = true, size_bytes = size_bytes + $1
			WHERE id = $2`, attachSize, msgID); err != nil {
			return "", "", err
		}
	}

	insertRecipients := func(kind string, list []string) error {
		for _, addr := range list {
			if _, err := tx.Exec(ctx, `
				INSERT INTO message_recipients (message_id, kind, address)
				VALUES ($1, $2, $3)`, msgID, kind, addr); err != nil {
				return err
			}
		}
		return nil
	}
	if err := insertRecipients("to", to); err != nil {
		return "", "", err
	}
	if err := insertRecipients("cc", cc); err != nil {
		return "", "", err
	}
	if err := insertRecipients("bcc", bcc); err != nil {
		return "", "", err
	}

	// Sender's Sent copy.
	var sentFolderID string
	if err := tx.QueryRow(ctx, `
		SELECT id FROM folders WHERE mailbox_id = $1 AND type = 'sent'`,
		sender.ID).Scan(&sentFolderID); err != nil {
		return "", "", err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO mailbox_messages (mailbox_id, message_id, folder_id, is_read)
		VALUES ($1, $2, $3, true)
		ON CONFLICT (mailbox_id, message_id, folder_id) DO NOTHING`,
		sender.ID, msgID, sentFolderID); err != nil {
		return "", "", err
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO message_events (message_id, type, detail)
		VALUES ($1, 'email.accepted', '{}')`, msgID); err != nil {
		return "", "", err
	}
	if err := events.Enqueue(ctx, tx, events.SubjectAccepted, events.AcceptedPayload{
		MessageID: msgID, PublicID: publicID, TenantID: tenantID,
		OrganizationID: sender.OrgID,
	}); err != nil {
		return "", "", err
	}
	return publicID, msgID, nil
}

// timeNow is a seam for tests.
var timeNow = func() time.Time { return time.Now().UTC() }
