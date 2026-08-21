package mailservice

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/smtp"
	"sort"
	"strings"
	"sync"
	"time"
)

// Service is the application mail layer: mailbox semantics for webmail and
// the delivery worker on top of the authoritative mail store.
type Service struct {
	JMAP *JMAP

	// SubmitAddr is the SMTP submission endpoint (host:port) used for
	// relaying mail to remote recipients through the mail core's queue.
	SubmitAddr string
	// SubmitInsecureTLS accepts the mail core's self-signed certificate.
	// Development-only; production must present a real certificate.
	SubmitInsecureTLS bool

	mu    sync.Mutex
	cache map[string]*accountInfo
}

type mailboxInfo struct {
	ID   string
	Name string
}

type accountInfo struct {
	AccountID string
	ByRole    map[string]mailboxInfo // role → mailbox
	fetched   time.Time
}

const accountCacheTTL = 60 * time.Second

// roleOrder is the canonical folder presentation order; "junk" is exposed to
// the UI under the legacy type name "spam".
var roleOrder = []string{"inbox", "sent", "drafts", "junk", "trash"}

// RoleForType maps the UI folder type to the JMAP mailbox role.
func RoleForType(t string) string {
	if t == "spam" {
		return "junk"
	}
	return t
}

// TypeForRole maps a JMAP mailbox role to the UI folder type.
func TypeForRole(r string) string {
	if r == "junk" {
		return "spam"
	}
	return r
}

// account returns cached account/mailbox identifiers for an email.
func (s *Service) account(ctx context.Context, email string) (*accountInfo, error) {
	s.mu.Lock()
	if s.cache == nil {
		s.cache = map[string]*accountInfo{}
	}
	if info, ok := s.cache[strings.ToLower(email)]; ok && time.Since(info.fetched) < accountCacheTTL {
		s.mu.Unlock()
		return info, nil
	}
	s.mu.Unlock()

	// The server is the authority on account ids.
	sess, err := s.JMAP.Session(ctx, email)
	if err != nil {
		return nil, err
	}
	accountID := sess.primaryMailAccount()
	if accountID == "" {
		return nil, fmt.Errorf("mail store has no mail account for %s", email)
	}
	var resp struct {
		List []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
			Role string `json:"role"`
		} `json:"list"`
	}
	out, err := s.JMAP.Request(ctx, email, Call("0", "Mailbox/get", map[string]any{
		"accountId":  accountID,
		"properties": []string{"id", "name", "role"},
	}))
	if err != nil {
		return nil, err
	}
	if err := Decode(out[0], &resp); err != nil {
		return nil, err
	}
	info := &accountInfo{AccountID: accountID, ByRole: map[string]mailboxInfo{}, fetched: time.Now()}
	for _, m := range resp.List {
		if m.Role != "" {
			info.ByRole[m.Role] = mailboxInfo{ID: m.ID, Name: m.Name}
		}
	}
	s.mu.Lock()
	s.cache[strings.ToLower(email)] = info
	s.mu.Unlock()
	return info, nil
}

// Folder is one mailbox folder with counters.
type Folder struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Type   string `json:"type"`
	Unread int    `json:"unread"`
	Total  int    `json:"total"`
}

// Summary returns the account's system folders with live counters.
func (s *Service) Summary(ctx context.Context, email string) ([]Folder, error) {
	info, err := s.account(ctx, email)
	if err != nil {
		return nil, err
	}
	var resp struct {
		List []struct {
			ID           string `json:"id"`
			Name         string `json:"name"`
			Role         string `json:"role"`
			TotalEmails  int    `json:"totalEmails"`
			UnreadEmails int    `json:"unreadEmails"`
		} `json:"list"`
	}
	out, err := s.JMAP.Request(ctx, email, Call("0", "Mailbox/get", map[string]any{
		"accountId":  info.AccountID,
		"properties": []string{"id", "name", "role", "totalEmails", "unreadEmails"},
	}))
	if err != nil {
		return nil, err
	}
	if err := Decode(out[0], &resp); err != nil {
		return nil, err
	}
	byRole := map[string]Folder{}
	for _, m := range resp.List {
		if m.Role == "" {
			continue
		}
		byRole[m.Role] = Folder{
			ID: m.ID, Name: m.Name, Type: TypeForRole(m.Role),
			Unread: m.UnreadEmails, Total: m.TotalEmails,
		}
	}
	folders := make([]Folder, 0, len(roleOrder))
	for _, role := range roleOrder {
		if f, ok := byRole[role]; ok {
			folders = append(folders, f)
		}
	}
	return folders, nil
}

// ListItem is one row of a folder listing (webmail JSON shape).
type ListItem struct {
	ID             string `json:"id"`
	MessageID      string `json:"message_id"` // RFC Message-ID
	From           string `json:"from"`
	FromDisplay    string `json:"from_display"`
	Subject        string `json:"subject"`
	Snippet        string `json:"snippet"`
	Date           string `json:"date"`
	IsRead         bool   `json:"is_read"`
	IsStarred      bool   `json:"is_starred"`
	HasAttachments bool   `json:"has_attachments"`
}

// ListQuery filters a folder listing.
type ListQuery struct {
	FolderType     string
	Text           string
	UnreadOnly     bool
	StarredOnly    bool
	HasAttachments bool
	Limit          int
	Offset         int
}

type emailAddress struct {
	Name  string `json:"name"`
	Email string `json:"email"`
}

type emailMeta struct {
	ID            string           `json:"id"`
	ThreadID      string           `json:"threadId"`
	MessageID     []string         `json:"messageId"`
	Keywords      map[string]bool  `json:"keywords"`
	From          []emailAddress   `json:"from"`
	Subject       string           `json:"subject"`
	Preview       string           `json:"preview"`
	ReceivedAt    string           `json:"receivedAt"`
	HasAttachment bool             `json:"hasAttachment"`
	MailboxIds    map[string]bool  `json:"mailboxIds"`
}

func metaToItem(m emailMeta) ListItem {
	it := ListItem{
		ID: m.ID, Subject: m.Subject, Snippet: m.Preview, Date: m.ReceivedAt,
		IsRead: m.Keywords["$seen"], IsStarred: m.Keywords["$flagged"],
		HasAttachments: m.HasAttachment,
	}
	if len(m.MessageID) > 0 {
		it.MessageID = m.MessageID[0]
	}
	if len(m.From) > 0 {
		it.From = m.From[0].Email
		it.FromDisplay = m.From[0].Name
	}
	return it
}

var listProperties = []string{
	"id", "threadId", "messageId", "keywords", "from", "subject",
	"preview", "receivedAt", "hasAttachment",
}

// List returns one folder page plus the folder's total match count.
func (s *Service) List(ctx context.Context, email string, q ListQuery) ([]ListItem, int, error) {
	info, err := s.account(ctx, email)
	if err != nil {
		return nil, 0, err
	}
	role := RoleForType(q.FolderType)
	mb, ok := info.ByRole[role]
	if !ok {
		return nil, 0, fmt.Errorf("no folder with role %q", role)
	}
	filter := map[string]any{"inMailbox": mb.ID}
	var conds []map[string]any
	if q.Text != "" {
		conds = append(conds, map[string]any{"text": q.Text})
	}
	if q.UnreadOnly {
		conds = append(conds, map[string]any{"notKeyword": "$seen"})
	}
	if q.StarredOnly {
		conds = append(conds, map[string]any{"hasKeyword": "$flagged"})
	}
	if q.HasAttachments {
		conds = append(conds, map[string]any{"hasAttachment": true})
	}
	if len(conds) > 0 {
		filter = map[string]any{
			"operator":   "AND",
			"conditions": append([]map[string]any{{"inMailbox": mb.ID}}, conds...),
		}
	}
	limit := q.Limit
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	out, err := s.JMAP.Request(ctx, email,
		Call("0", "Email/query", map[string]any{
			"accountId":      info.AccountID,
			"filter":         filter,
			"sort":           []map[string]any{{"property": "receivedAt", "isAscending": false}},
			"position":       q.Offset,
			"limit":          limit,
			"calculateTotal": true,
		}),
		Call("1", "Email/get", map[string]any{
			"accountId":  info.AccountID,
			"#ids":       map[string]any{"resultOf": "0", "name": "Email/query", "path": "/ids"},
			"properties": listProperties,
		}),
	)
	if err != nil {
		return nil, 0, err
	}
	var queryResp struct {
		IDs   []string `json:"ids"`
		Total int      `json:"total"`
	}
	if err := Decode(out[0], &queryResp); err != nil {
		return nil, 0, err
	}
	var getResp struct {
		List []emailMeta `json:"list"`
	}
	if err := Decode(out[1], &getResp); err != nil {
		return nil, 0, err
	}
	// Email/get returns unordered; restore query order.
	byID := map[string]emailMeta{}
	for _, m := range getResp.List {
		byID[m.ID] = m
	}
	items := make([]ListItem, 0, len(queryResp.IDs))
	for _, id := range queryResp.IDs {
		if m, ok := byID[id]; ok {
			items = append(items, metaToItem(m))
		}
	}
	return items, queryResp.Total, nil
}

// Recipient mirrors the webmail recipient JSON shape.
type Recipient struct {
	Kind    string `json:"kind"`
	Address string `json:"address"`
}

// AttachmentView is one attachment of a stored message.
type AttachmentView struct {
	ID          string `json:"id"` // blobId
	Filename    string `json:"filename"`
	ContentType string `json:"content_type"`
	SizeBytes   int64  `json:"size_bytes"`
}

// ThreadItem is one sibling in a conversation.
type ThreadItem struct {
	ID      string `json:"id"`
	Subject string `json:"subject"`
	From    string `json:"from"`
	Date    string `json:"date"`
}

// Message is the full reading-pane payload.
type Message struct {
	ID             string           `json:"id"`
	MessageID      string           `json:"message_id"`
	Folder         string           `json:"folder"`
	From           string           `json:"from"`
	FromDisplay    string           `json:"from_display"`
	Recipients     []Recipient      `json:"recipients"`
	Subject        string           `json:"subject"`
	BodyText       string           `json:"body_text"`
	BodyHTML       string           `json:"body_html"`
	Date           string           `json:"date"`
	IsRead         bool             `json:"is_read"`
	IsStarred      bool             `json:"is_starred"`
	HasAttachments bool             `json:"has_attachments"`
	Attachments    []AttachmentView `json:"attachments"`
	Thread         []ThreadItem     `json:"thread"`
	RFCInReplyTo   string           `json:"-"`
	RFCReferences  string           `json:"-"`
}

type bodyPart struct {
	PartID string `json:"partId"`
	BlobID string `json:"blobId"`
	Type   string `json:"type"`
	Name   string `json:"name"`
	Size   int64  `json:"size"`
}

// GetMessage loads one message with bodies, attachments and its thread, and
// marks it read (protocol flag, visible to every client).
func (s *Service) GetMessage(ctx context.Context, email, id string) (*Message, error) {
	info, err := s.account(ctx, email)
	if err != nil {
		return nil, err
	}
	out, err := s.JMAP.Request(ctx, email, Call("0", "Email/get", map[string]any{
		"accountId": info.AccountID,
		"ids":       []string{id},
		"properties": []string{
			"id", "threadId", "mailboxIds", "keywords", "messageId", "inReplyTo",
			"references", "from", "to", "cc", "bcc", "subject", "receivedAt",
			"hasAttachment", "bodyValues", "textBody", "htmlBody", "attachments",
		},
		"fetchTextBodyValues": true,
		"fetchHTMLBodyValues": true,
		"maxBodyValueBytes":   1 << 20,
	}))
	if err != nil {
		return nil, err
	}
	var resp struct {
		List []struct {
			emailMeta
			InReplyTo   []string       `json:"inReplyTo"`
			References  []string       `json:"references"`
			To          []emailAddress `json:"to"`
			Cc          []emailAddress `json:"cc"`
			Bcc         []emailAddress `json:"bcc"`
			BodyValues  map[string]struct {
				Value string `json:"value"`
			} `json:"bodyValues"`
			TextBody    []bodyPart `json:"textBody"`
			HTMLBody    []bodyPart `json:"htmlBody"`
			Attachments []bodyPart `json:"attachments"`
		} `json:"list"`
	}
	if err := Decode(out[0], &resp); err != nil {
		return nil, err
	}
	if len(resp.List) == 0 {
		return nil, nil
	}
	e := resp.List[0]

	msg := &Message{
		ID: e.ID, Subject: e.Subject, Date: e.ReceivedAt,
		IsRead: e.Keywords["$seen"], IsStarred: e.Keywords["$flagged"],
		HasAttachments: e.HasAttachment,
		Recipients:     []Recipient{},
		Attachments:    []AttachmentView{},
		Thread:         []ThreadItem{},
	}
	if len(e.MessageID) > 0 {
		msg.MessageID = e.MessageID[0]
	}
	if len(e.InReplyTo) > 0 {
		msg.RFCInReplyTo = e.InReplyTo[0]
	}
	if len(e.References) > 0 {
		msg.RFCReferences = strings.Join(e.References, " ")
	}
	if len(e.From) > 0 {
		msg.From = e.From[0].Email
		msg.FromDisplay = e.From[0].Name
	}
	for _, a := range e.To {
		msg.Recipients = append(msg.Recipients, Recipient{Kind: "to", Address: a.Email})
	}
	for _, a := range e.Cc {
		msg.Recipients = append(msg.Recipients, Recipient{Kind: "cc", Address: a.Email})
	}
	for _, a := range e.Bcc {
		msg.Recipients = append(msg.Recipients, Recipient{Kind: "bcc", Address: a.Email})
	}
	for _, p := range e.TextBody {
		if v, ok := e.BodyValues[p.PartID]; ok {
			msg.BodyText += v.Value
		}
	}
	for _, p := range e.HTMLBody {
		if v, ok := e.BodyValues[p.PartID]; ok {
			msg.BodyHTML += v.Value
		}
	}
	for _, a := range e.Attachments {
		msg.Attachments = append(msg.Attachments, AttachmentView{
			ID: a.BlobID, Filename: a.Name, ContentType: a.Type, SizeBytes: a.Size,
		})
	}
	// Folder from the mailbox role.
	for role, mb := range info.ByRole {
		if e.MailboxIds[mb.ID] {
			msg.Folder = TypeForRole(role)
			break
		}
	}

	// Conversation thread.
	if e.ThreadID != "" {
		tout, err := s.JMAP.Request(ctx, email,
			Call("0", "Thread/get", map[string]any{
				"accountId": info.AccountID,
				"ids":       []string{e.ThreadID},
			}),
			Call("1", "Email/get", map[string]any{
				"accountId":  info.AccountID,
				"#ids":       map[string]any{"resultOf": "0", "name": "Thread/get", "path": "/list/*/emailIds"},
				"properties": []string{"id", "subject", "from", "receivedAt"},
			}),
		)
		if err == nil {
			var tresp struct {
				List []emailMeta `json:"list"`
			}
			if Decode(tout[1], &tresp) == nil {
				sort.Slice(tresp.List, func(i, j int) bool {
					return tresp.List[i].ReceivedAt < tresp.List[j].ReceivedAt
				})
				for _, t := range tresp.List {
					ti := ThreadItem{ID: t.ID, Subject: t.Subject, Date: t.ReceivedAt}
					if len(t.From) > 0 {
						ti.From = t.From[0].Email
					}
					msg.Thread = append(msg.Thread, ti)
				}
			}
		}
	}

	if !msg.IsRead {
		if err := s.SetFlags(ctx, email, id, boolPtr(true), nil); err == nil {
			msg.IsRead = true
		}
	}
	return msg, nil
}

func boolPtr(b bool) *bool { return &b }

// emailSet runs one Email/set and surfaces per-item failures.
func (s *Service) emailSet(ctx context.Context, email, accountID string, args map[string]any) error {
	args["accountId"] = accountID
	out, err := s.JMAP.Request(ctx, email, Call("0", "Email/set", args))
	if err != nil {
		return err
	}
	var resp struct {
		NotUpdated   map[string]json.RawMessage `json:"notUpdated"`
		NotDestroyed map[string]json.RawMessage `json:"notDestroyed"`
		NotCreated   map[string]json.RawMessage `json:"notCreated"`
	}
	if err := Decode(out[0], &resp); err != nil {
		return err
	}
	if len(resp.NotUpdated)+len(resp.NotDestroyed)+len(resp.NotCreated) > 0 {
		return fmt.Errorf("mail store rejected the change")
	}
	return nil
}

// SetFlags updates $seen / $flagged.
func (s *Service) SetFlags(ctx context.Context, email, id string, read, starred *bool) error {
	info, err := s.account(ctx, email)
	if err != nil {
		return err
	}
	patch := map[string]any{}
	if read != nil {
		patch["keywords/$seen"] = keywordValue(*read)
	}
	if starred != nil {
		patch["keywords/$flagged"] = keywordValue(*starred)
	}
	return s.emailSet(ctx, email, info.AccountID, map[string]any{
		"update": map[string]any{id: patch},
	})
}

func keywordValue(on bool) any {
	if on {
		return true
	}
	return nil // JMAP patch: null removes the keyword
}

// Move places the message into the folder with the given UI type.
func (s *Service) Move(ctx context.Context, email, id, folderType string) error {
	info, err := s.account(ctx, email)
	if err != nil {
		return err
	}
	mb, ok := info.ByRole[RoleForType(folderType)]
	if !ok {
		return fmt.Errorf("no folder with role %q", folderType)
	}
	return s.emailSet(ctx, email, info.AccountID, map[string]any{
		"update": map[string]any{id: map[string]any{
			"mailboxIds": map[string]bool{mb.ID: true},
		}},
	})
}

// Destroy permanently removes the message.
func (s *Service) Destroy(ctx context.Context, email, id string) error {
	info, err := s.account(ctx, email)
	if err != nil {
		return err
	}
	return s.emailSet(ctx, email, info.AccountID, map[string]any{
		"destroy": []string{id},
	})
}

// Import stores a raw RFC822 message directly into a folder of the account
// (delivery, Sent copies, quarantine release, legacy migration).
func (s *Service) Import(ctx context.Context, email, folderType string, raw []byte, seen bool) (jmapID string, err error) {
	info, err := s.account(ctx, email)
	if err != nil {
		return "", err
	}
	mb, ok := info.ByRole[RoleForType(folderType)]
	if !ok {
		return "", fmt.Errorf("no folder with role %q", folderType)
	}
	blobID, err := s.JMAP.UploadBlob(ctx, email, info.AccountID, "message/rfc822", raw)
	if err != nil {
		return "", err
	}
	keywords := map[string]bool{}
	if seen {
		keywords["$seen"] = true
	}
	out, err := s.JMAP.Request(ctx, email, Call("0", "Email/import", map[string]any{
		"accountId": info.AccountID,
		"emails": map[string]any{
			"m1": map[string]any{
				"blobId":     blobID,
				"mailboxIds": map[string]bool{mb.ID: true},
				"keywords":   keywords,
			},
		},
	}))
	if err != nil {
		return "", err
	}
	var resp struct {
		Created map[string]struct {
			ID string `json:"id"`
		} `json:"created"`
		NotCreated map[string]methodError `json:"notCreated"`
	}
	if err := Decode(out[0], &resp); err != nil {
		return "", err
	}
	if c, ok := resp.Created["m1"]; ok {
		return c.ID, nil
	}
	if nc, ok := resp.NotCreated["m1"]; ok {
		return "", fmt.Errorf("import rejected: %s %s", nc.Type, nc.Description)
	}
	return "", errors.New("import produced no result")
}

// SubmitExternal relays a message to remote recipients through the mail
// core's SMTP submission (master-authenticated), which owns MX lookup,
// queueing and retries.
func (s *Service) SubmitExternal(ctx context.Context, fromAccount, mailFrom string, rcpts []string, raw []byte) error {
	host := s.SubmitAddr
	if h, _, err := net.SplitHostPort(s.SubmitAddr); err == nil {
		host = h
	}
	d := net.Dialer{Timeout: 15 * time.Second}
	conn, err := d.DialContext(ctx, "tcp", s.SubmitAddr)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	c, err := smtp.NewClient(conn, host)
	if err != nil {
		conn.Close()
		return fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	defer c.Close()
	if err := c.StartTLS(&tls.Config{ServerName: host, InsecureSkipVerify: s.SubmitInsecureTLS}); err != nil {
		return fmt.Errorf("starttls: %w", err)
	}
	if err := c.Auth(smtp.PlainAuth("", fromAccount+"%"+s.JMAP.MasterUser, s.JMAP.MasterSecret, host)); err != nil {
		return fmt.Errorf("submission auth: %w", err)
	}
	if err := c.Mail(mailFrom); err != nil {
		return fmt.Errorf("mail from: %w", err)
	}
	for _, r := range rcpts {
		if err := c.Rcpt(r); err != nil {
			return fmt.Errorf("rcpt %s: %w", r, err)
		}
	}
	w, err := c.Data()
	if err != nil {
		return fmt.Errorf("data: %w", err)
	}
	if _, err := w.Write(raw); err != nil {
		return fmt.Errorf("write: %w", err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("finish: %w", err)
	}
	return c.Quit()
}

// OpenBlob streams one blob (attachment) from the account's store.
func (s *Service) OpenBlob(ctx context.Context, email, blobID, name, accept string) (io.ReadCloser, int64, error) {
	info, err := s.account(ctx, email)
	if err != nil {
		return nil, 0, err
	}
	return s.JMAP.DownloadBlob(ctx, email, info.AccountID, blobID, name, accept)
}

// Draft management: drafts live in the store's Drafts folder so IMAP/JMAP
// clients see the same drafts as webmail.

// SaveDraft creates or replaces a draft and returns its JMAP id. When
// replacing, the previous copy is destroyed after the new one is written.
func (s *Service) SaveDraft(ctx context.Context, email string, previousID string, raw []byte) (string, error) {
	newID, err := s.importDraft(ctx, email, raw)
	if err != nil {
		return "", err
	}
	if previousID != "" {
		_ = s.Destroy(ctx, email, previousID) // best effort; stale copy only
	}
	return newID, nil
}

func (s *Service) importDraft(ctx context.Context, email string, raw []byte) (string, error) {
	info, err := s.account(ctx, email)
	if err != nil {
		return "", err
	}
	mb, ok := info.ByRole["drafts"]
	if !ok {
		return "", errors.New("no drafts folder")
	}
	blobID, err := s.JMAP.UploadBlob(ctx, email, info.AccountID, "message/rfc822", raw)
	if err != nil {
		return "", err
	}
	out, err := s.JMAP.Request(ctx, email, Call("0", "Email/import", map[string]any{
		"accountId": info.AccountID,
		"emails": map[string]any{"m1": map[string]any{
			"blobId":     blobID,
			"mailboxIds": map[string]bool{mb.ID: true},
			"keywords":   map[string]bool{"$seen": true, "$draft": true},
		}},
	}))
	if err != nil {
		return "", err
	}
	var resp struct {
		Created map[string]struct {
			ID string `json:"id"`
		} `json:"created"`
	}
	if err := Decode(out[0], &resp); err != nil {
		return "", err
	}
	if c, ok := resp.Created["m1"]; ok {
		return c.ID, nil
	}
	return "", errors.New("draft import produced no result")
}
