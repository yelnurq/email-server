// Package collab implements organization-scoped realtime conversations.
package collab

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/gorilla/websocket"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nats-io/nats.go"

	"github.com/yelnurq/email-server/internal/auth"
	"github.com/yelnurq/email-server/internal/events"
	"github.com/yelnurq/email-server/internal/httpx"
)

type Handlers struct {
	Pool *pgxpool.Pool
	NATS *nats.Conn
	Log  *slog.Logger
}

type Conversation struct {
	ID          string `json:"id"`
	Kind        string `json:"kind"`
	Title       string `json:"title"`
	LastMessage string `json:"last_message"`
	UpdatedAt   string `json:"updated_at"`
	Unread      int    `json:"unread"`
}
type Message struct {
	ID             string       `json:"id"`
	ConversationID string       `json:"conversation_id"`
	SenderUserID   string       `json:"sender_user_id,omitempty"`
	SenderName     string       `json:"sender_name"`
	ReplyToID      string       `json:"reply_to_id,omitempty"`
	Body           string       `json:"body"`
	EditedAt       string       `json:"edited_at,omitempty"`
	DeletedAt      string       `json:"deleted_at,omitempty"`
	CreatedAt      string       `json:"created_at"`
	Attachments    []Attachment `json:"attachments"`
	ReadBy         int          `json:"read_by"`
}
type Attachment struct {
	ID          string `json:"id"`
	Filename    string `json:"filename"`
	ContentType string `json:"content_type"`
	SizeBytes   int64  `json:"size_bytes"`
}

func (h *Handlers) List(w http.ResponseWriter, r *http.Request) {
	id := auth.IdentityFrom(r.Context())
	rows, err := h.Pool.Query(r.Context(), `
		SELECT c.id,c.kind,CASE WHEN c.kind='direct' THEN COALESCE(other.display_name,other.email::text) ELSE c.title END,
		 COALESCE(lastm.body,''),c.updated_at::text,
		 count(unread.id) FILTER (WHERE unread.created_at > COALESCE(cm.last_read_at,'epoch'))
		FROM chat_conversations c JOIN chat_conversation_members cm ON cm.conversation_id=c.id AND cm.user_id=$1
		LEFT JOIN LATERAL (SELECT body FROM chat_messages WHERE conversation_id=c.id AND deleted_at IS NULL ORDER BY created_at DESC LIMIT 1) lastm ON true
		LEFT JOIN chat_conversation_members ocm ON c.kind='direct' AND ocm.conversation_id=c.id AND ocm.user_id<>$1
		LEFT JOIN users other ON other.id=ocm.user_id
		LEFT JOIN chat_messages unread ON unread.conversation_id=c.id AND unread.sender_user_id<>$1
		WHERE c.tenant_id=$2 AND c.organization_id=$3 GROUP BY c.id,cm.last_read_at,other.display_name,other.email,lastm.body ORDER BY c.updated_at DESC`, id.UserID, id.TenantID, id.OrganizationID)
	if err != nil {
		h.Log.Error("conversation list", slog.String("error", err.Error()))
		httpx.Internal(w, r)
		return
	}
	defer rows.Close()
	out := []Conversation{}
	for rows.Next() {
		var c Conversation
		if rows.Scan(&c.ID, &c.Kind, &c.Title, &c.LastMessage, &c.UpdatedAt, &c.Unread) != nil {
			httpx.Internal(w, r)
			return
		}
		out = append(out, c)
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"conversations": out})
}

type createConversationRequest struct {
	Kind    string   `json:"kind"`
	Title   string   `json:"title"`
	UserIDs []string `json:"user_ids"`
}

func (h *Handlers) Create(w http.ResponseWriter, r *http.Request) {
	id := auth.IdentityFrom(r.Context())
	var req createConversationRequest
	if httpx.Decode(w, r, &req) != nil {
		return
	}
	if req.Kind != "direct" && req.Kind != "group" {
		httpx.Error(w, r, 400, "INVALID_KIND", "Conversation kind must be direct or group")
		return
	}
	if req.Kind == "group" && !id.HasPermission("messages.group.create") {
		httpx.Error(w, r, 403, "FORBIDDEN", "Group conversation permission required")
		return
	}
	memberIDs := append([]string{id.UserID}, req.UserIDs...)
	unique := map[string]bool{}
	clean := []string{}
	for _, v := range memberIDs {
		if v != "" && !unique[v] {
			unique[v] = true
			clean = append(clean, v)
		}
	}
	if (req.Kind == "direct" && len(clean) != 2) || (req.Kind == "group" && (len(clean) < 2 || strings.TrimSpace(req.Title) == "")) {
		httpx.Error(w, r, 400, "INVALID_MEMBERS", "Invalid conversation members")
		return
	}
	tx, err := h.Pool.Begin(r.Context())
	if err != nil {
		httpx.Internal(w, r)
		return
	}
	defer tx.Rollback(r.Context())
	var count int
	if tx.QueryRow(r.Context(), `SELECT count(*) FROM users WHERE id=ANY($1::uuid[]) AND tenant_id=$2 AND organization_id=$3 AND status='active'`, clean, id.TenantID, id.OrganizationID).Scan(&count) != nil || count != len(clean) {
		httpx.Error(w, r, 400, "INVALID_MEMBERS", "Members must belong to your organization")
		return
	}
	if req.Kind == "direct" {
		var existing string
		err = tx.QueryRow(r.Context(), `SELECT c.id FROM chat_conversations c WHERE c.tenant_id=$1 AND c.organization_id=$2 AND c.kind='direct' AND (SELECT array_agg(user_id ORDER BY user_id) FROM chat_conversation_members WHERE conversation_id=c.id)=(SELECT array_agg(x::uuid ORDER BY x::uuid) FROM unnest($3::text[]) x) LIMIT 1`, id.TenantID, id.OrganizationID, clean).Scan(&existing)
		if err == nil {
			httpx.JSON(w, 200, map[string]string{"id": existing})
			return
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			httpx.Internal(w, r)
			return
		}
	}
	var cid string
	if tx.QueryRow(r.Context(), `INSERT INTO chat_conversations(tenant_id,organization_id,kind,title,created_by) VALUES($1,$2,$3,$4,$5) RETURNING id`, id.TenantID, id.OrganizationID, req.Kind, strings.TrimSpace(req.Title), id.UserID).Scan(&cid) != nil {
		httpx.Internal(w, r)
		return
	}
	if _, err = tx.Exec(r.Context(), `INSERT INTO chat_conversation_members(conversation_id,user_id,role) SELECT $1,x::uuid,CASE WHEN x::uuid=$2 THEN 'owner' ELSE 'member' END FROM unnest($3::text[]) x`, cid, id.UserID, clean); err != nil {
		httpx.Internal(w, r)
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		httpx.Internal(w, r)
		return
	}
	httpx.JSON(w, 201, map[string]string{"id": cid})
}

func (h *Handlers) Messages(w http.ResponseWriter, r *http.Request) {
	id := auth.IdentityFrom(r.Context())
	cid := chi.URLParam(r, "id")
	var member bool
	if err := h.Pool.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM chat_conversations c JOIN chat_conversation_members cm ON cm.conversation_id=c.id WHERE c.id=$1 AND c.tenant_id=$2 AND c.organization_id=$3 AND cm.user_id=$4)`, cid, id.TenantID, id.OrganizationID, id.UserID).Scan(&member); err != nil || !member {
		httpx.Error(w, r, http.StatusNotFound, "CONVERSATION_NOT_FOUND", "Conversation not found")
		return
	}
	rows, err := h.Pool.Query(r.Context(), `
		SELECT m.id,m.conversation_id,COALESCE(m.sender_user_id::text,''),COALESCE(u.display_name,u.email::text,''),COALESCE(m.reply_to_id::text,''),
		CASE WHEN m.deleted_at IS NULL THEN m.body ELSE '' END,COALESCE(m.edited_at::text,''),COALESCE(m.deleted_at::text,''),m.created_at::text,
		(SELECT count(*) FROM chat_conversation_members seen WHERE seen.conversation_id=m.conversation_id AND seen.user_id<>m.sender_user_id AND seen.last_read_at>=m.created_at)
		FROM chat_messages m JOIN chat_conversations c ON c.id=m.conversation_id JOIN chat_conversation_members cm ON cm.conversation_id=c.id AND cm.user_id=$1 LEFT JOIN users u ON u.id=m.sender_user_id
		WHERE c.id=$2 AND c.tenant_id=$3 AND c.organization_id=$4 ORDER BY m.created_at LIMIT 500`, id.UserID, cid, id.TenantID, id.OrganizationID)
	if err != nil {
		httpx.Internal(w, r)
		return
	}
	defer rows.Close()
	out := []Message{}
	for rows.Next() {
		var m Message
		if rows.Scan(&m.ID, &m.ConversationID, &m.SenderUserID, &m.SenderName, &m.ReplyToID, &m.Body, &m.EditedAt, &m.DeletedAt, &m.CreatedAt, &m.ReadBy) != nil {
			httpx.Internal(w, r)
			return
		}
		m.Attachments = []Attachment{}
		aRows, aErr := h.Pool.Query(r.Context(), `SELECT public_id,filename,content_type,size_bytes FROM attachments WHERE chat_message_id=$1 ORDER BY created_at`, m.ID)
		if aErr == nil {
			for aRows.Next() {
				var a Attachment
				if aRows.Scan(&a.ID, &a.Filename, &a.ContentType, &a.SizeBytes) == nil {
					m.Attachments = append(m.Attachments, a)
				}
			}
			aRows.Close()
		}
		out = append(out, m)
	}
	httpx.JSON(w, 200, map[string]any{"messages": out})
}

type sendRequest struct {
	Body          string   `json:"body"`
	ReplyToID     string   `json:"reply_to_id"`
	AttachmentIDs []string `json:"attachment_ids"`
}

func (h *Handlers) Send(w http.ResponseWriter, r *http.Request) {
	id := auth.IdentityFrom(r.Context())
	cid := chi.URLParam(r, "id")
	var req sendRequest
	if httpx.Decode(w, r, &req) != nil {
		return
	}
	req.Body = strings.TrimSpace(req.Body)
	if req.Body == "" || len(req.Body) > 10000 {
		httpx.Error(w, r, 400, "INVALID_MESSAGE", "Message body is required (max 10000)")
		return
	}
	tx, err := h.Pool.Begin(r.Context())
	if err != nil {
		httpx.Internal(w, r)
		return
	}
	defer tx.Rollback(r.Context())
	var allowed bool
	if tx.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM chat_conversations c JOIN chat_conversation_members cm ON cm.conversation_id=c.id WHERE c.id=$1 AND c.tenant_id=$2 AND c.organization_id=$3 AND cm.user_id=$4)`, cid, id.TenantID, id.OrganizationID, id.UserID).Scan(&allowed) != nil || !allowed {
		httpx.Error(w, r, 404, "CONVERSATION_NOT_FOUND", "Conversation not found")
		return
	}
	if len(req.AttachmentIDs) > 10 {
		httpx.Error(w, r, http.StatusBadRequest, "TOO_MANY_ATTACHMENTS", "Maximum 10 chat attachments")
		return
	}
	if len(req.AttachmentIDs) > 0 {
		var count int
		if err := tx.QueryRow(r.Context(), `SELECT count(*) FROM attachments WHERE public_id=ANY($1::text[]) AND tenant_id=$2 AND uploader_user_id=$3 AND message_id IS NULL AND chat_message_id IS NULL`, req.AttachmentIDs, id.TenantID, id.UserID).Scan(&count); err != nil || count != len(req.AttachmentIDs) {
			httpx.Error(w, r, http.StatusBadRequest, "INVALID_ATTACHMENTS", "Attachments must be your staged uploads")
			return
		}
	}
	var reply any
	if req.ReplyToID != "" {
		reply = req.ReplyToID
	}
	var m Message
	if tx.QueryRow(r.Context(), `INSERT INTO chat_messages(conversation_id,sender_user_id,reply_to_id,body,attachment_ids) VALUES($1,$2,$3,$4,COALESCE($5::text[],'{}')) RETURNING id,conversation_id,sender_user_id::text,body,created_at::text`, cid, id.UserID, reply, req.Body, req.AttachmentIDs).Scan(&m.ID, &m.ConversationID, &m.SenderUserID, &m.Body, &m.CreatedAt) != nil {
		httpx.Error(w, r, 400, "INVALID_MESSAGE", "Invalid reply or attachment")
		return
	}
	if len(req.AttachmentIDs) > 0 {
		if _, err = tx.Exec(r.Context(), `UPDATE attachments SET chat_message_id=$1 WHERE public_id=ANY($2::text[]) AND tenant_id=$3 AND uploader_user_id=$4`, m.ID, req.AttachmentIDs, id.TenantID, id.UserID); err != nil {
			httpx.Internal(w, r)
			return
		}
	}
	m.SenderName = id.DisplayName
	if _, err = tx.Exec(r.Context(), `UPDATE chat_conversations SET updated_at=now() WHERE id=$1`, cid); err != nil {
		httpx.Internal(w, r)
		return
	}
	raw, _ := json.Marshal(map[string]any{"type": "message.created", "message": m})
	var memberIDs []string
	rows, _ := tx.Query(r.Context(), `SELECT user_id::text FROM chat_conversation_members WHERE conversation_id=$1`, cid)
	for rows.Next() {
		var uid string
		rows.Scan(&uid)
		memberIDs = append(memberIDs, uid)
	}
	rows.Close()
	for _, uid := range memberIDs {
		if err = events.Enqueue(r.Context(), tx, "communication.user."+uid, json.RawMessage(raw)); err != nil {
			httpx.Internal(w, r)
			return
		}
		if uid != id.UserID {
			_, _ = tx.Exec(r.Context(), `INSERT INTO notifications(tenant_id,organization_id,user_id,kind,title,body,target_url) SELECT $1,$2,u.id,CASE WHEN $5 ILIKE '%@'||lower(split_part(u.display_name,' ',1))||'%' THEN 'mention' ELSE 'message' END,$4,$5,$6 FROM users u WHERE u.id=$3`, id.TenantID, id.OrganizationID, uid, id.DisplayName, req.Body, "/mail/messages?conversation="+cid)
		}
	}
	if err = tx.Commit(r.Context()); err != nil {
		httpx.Internal(w, r)
		return
	}
	httpx.JSON(w, 201, m)
}

func (h *Handlers) MarkRead(w http.ResponseWriter, r *http.Request) {
	id := auth.IdentityFrom(r.Context())
	tag, err := h.Pool.Exec(r.Context(), `UPDATE chat_conversation_members cm SET last_read_at=now() FROM chat_conversations c WHERE cm.conversation_id=c.id AND c.id=$1 AND cm.user_id=$2 AND c.tenant_id=$3 AND c.organization_id=$4`, chi.URLParam(r, "id"), id.UserID, id.TenantID, id.OrganizationID)
	if err != nil {
		httpx.Internal(w, r)
		return
	}
	if tag.RowsAffected() == 0 {
		httpx.Error(w, r, 404, "CONVERSATION_NOT_FOUND", "Conversation not found")
		return
	}
	w.WriteHeader(204)
}

type editRequest struct {
	Body string `json:"body"`
}

func (h *Handlers) Edit(w http.ResponseWriter, r *http.Request) {
	id := auth.IdentityFrom(r.Context())
	var req editRequest
	if httpx.Decode(w, r, &req) != nil {
		return
	}
	req.Body = strings.TrimSpace(req.Body)
	if req.Body == "" || len(req.Body) > 10000 {
		httpx.Error(w, r, 400, "INVALID_MESSAGE", "Message body is required")
		return
	}
	mid := chi.URLParam(r, "messageID")
	var cid string
	err := h.Pool.QueryRow(r.Context(), `UPDATE chat_messages m SET body=$1,edited_at=now() FROM chat_conversations c WHERE m.conversation_id=c.id AND m.id=$2 AND m.sender_user_id=$3 AND m.deleted_at IS NULL AND c.tenant_id=$4 AND c.organization_id=$5 RETURNING m.conversation_id`, req.Body, mid, id.UserID, id.TenantID, id.OrganizationID).Scan(&cid)
	if errors.Is(err, pgx.ErrNoRows) {
		httpx.Error(w, r, 404, "MESSAGE_NOT_FOUND", "Message not found")
		return
	}
	if err != nil {
		httpx.Internal(w, r)
		return
	}
	h.publishConversation(r, cid, map[string]any{"type": "message.updated", "conversation_id": cid, "message_id": mid, "body": req.Body})
	w.WriteHeader(204)
}
func (h *Handlers) Delete(w http.ResponseWriter, r *http.Request) {
	id := auth.IdentityFrom(r.Context())
	mid := chi.URLParam(r, "messageID")
	var cid string
	err := h.Pool.QueryRow(r.Context(), `UPDATE chat_messages m SET deleted_at=now(),body='' FROM chat_conversations c WHERE m.conversation_id=c.id AND m.id=$1 AND m.sender_user_id=$2 AND m.deleted_at IS NULL AND c.tenant_id=$3 AND c.organization_id=$4 RETURNING m.conversation_id`, mid, id.UserID, id.TenantID, id.OrganizationID).Scan(&cid)
	if errors.Is(err, pgx.ErrNoRows) {
		httpx.Error(w, r, 404, "MESSAGE_NOT_FOUND", "Message not found")
		return
	}
	if err != nil {
		httpx.Internal(w, r)
		return
	}
	h.publishConversation(r, cid, map[string]any{"type": "message.deleted", "conversation_id": cid, "message_id": mid})
	w.WriteHeader(204)
}

type reactionRequest struct {
	Emoji string `json:"emoji"`
}

func (h *Handlers) React(w http.ResponseWriter, r *http.Request) {
	id := auth.IdentityFrom(r.Context())
	var req reactionRequest
	if httpx.Decode(w, r, &req) != nil {
		return
	}
	if len(req.Emoji) < 1 || len(req.Emoji) > 32 {
		httpx.Error(w, r, 400, "INVALID_REACTION", "Invalid reaction")
		return
	}
	mid := chi.URLParam(r, "messageID")
	var cid string
	err := h.Pool.QueryRow(r.Context(), `SELECT m.conversation_id FROM chat_messages m JOIN chat_conversations c ON c.id=m.conversation_id JOIN chat_conversation_members cm ON cm.conversation_id=c.id AND cm.user_id=$1 WHERE m.id=$2 AND c.tenant_id=$3 AND c.organization_id=$4`, id.UserID, mid, id.TenantID, id.OrganizationID).Scan(&cid)
	if errors.Is(err, pgx.ErrNoRows) {
		httpx.Error(w, r, 404, "MESSAGE_NOT_FOUND", "Message not found")
		return
	}
	tag, err := h.Pool.Exec(r.Context(), `DELETE FROM chat_message_reactions WHERE message_id=$1 AND user_id=$2 AND emoji=$3`, mid, id.UserID, req.Emoji)
	if err != nil {
		httpx.Internal(w, r)
		return
	}
	active := false
	if tag.RowsAffected() == 0 {
		if _, err = h.Pool.Exec(r.Context(), `INSERT INTO chat_message_reactions(message_id,user_id,emoji) VALUES($1,$2,$3)`, mid, id.UserID, req.Emoji); err != nil {
			httpx.Internal(w, r)
			return
		}
		active = true
	}
	h.publishConversation(r, cid, map[string]any{"type": "message.reaction", "conversation_id": cid, "message_id": mid, "user_id": id.UserID, "emoji": req.Emoji, "active": active})
	w.WriteHeader(204)
}

func (h *Handlers) publishConversation(r *http.Request, cid string, event any) {
	raw, _ := json.Marshal(event)
	rows, err := h.Pool.Query(r.Context(), `SELECT user_id::text FROM chat_conversation_members WHERE conversation_id=$1`, cid)
	if err != nil {
		return
	}
	defer rows.Close()
	for rows.Next() {
		var uid string
		if rows.Scan(&uid) == nil {
			_ = h.NATS.Publish("communication.user."+uid, raw)
		}
	}
}

type typingRequest struct {
	Active bool `json:"active"`
}

func (h *Handlers) Typing(w http.ResponseWriter, r *http.Request) {
	id := auth.IdentityFrom(r.Context())
	var req typingRequest
	if httpx.Decode(w, r, &req) != nil {
		return
	}
	cid := chi.URLParam(r, "id")
	rows, err := h.Pool.Query(r.Context(), `SELECT cm.user_id::text FROM chat_conversation_members cm JOIN chat_conversations c ON c.id=cm.conversation_id WHERE c.id=$1 AND c.tenant_id=$2 AND c.organization_id=$3 AND EXISTS(SELECT 1 FROM chat_conversation_members mine WHERE mine.conversation_id=c.id AND mine.user_id=$4) AND cm.user_id<>$4`, cid, id.TenantID, id.OrganizationID, id.UserID)
	if err != nil {
		httpx.Internal(w, r)
		return
	}
	defer rows.Close()
	event := map[string]any{"type": map[bool]string{true: "typing.started", false: "typing.stopped"}[req.Active], "conversation_id": cid, "user_id": id.UserID, "display_name": id.DisplayName}
	raw, _ := json.Marshal(event)
	for rows.Next() {
		var uid string
		rows.Scan(&uid)
		_ = h.NATS.Publish("communication.user."+uid, raw)
	}
	w.WriteHeader(204)
}

func (h *Handlers) Realtime(w http.ResponseWriter, r *http.Request) {
	id := auth.IdentityFrom(r.Context())
	up := websocket.Upgrader{CheckOrigin: func(r *http.Request) bool {
		origin := r.Header.Get("Origin")
		if origin == "" {
			return true
		}
		parsed, err := url.Parse(origin)
		if err != nil {
			return false
		}
		requestHost := r.Host
		if host, _, splitErr := net.SplitHostPort(r.Host); splitErr == nil {
			requestHost = host
		}
		return parsed.Hostname() == requestHost
	}}
	conn, err := up.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close()
	h.setPresence(r, id, true)
	defer h.setPresence(r, id, false)
	sub, err := h.NATS.SubscribeSync("communication.user." + id.UserID)
	if err != nil {
		return
	}
	defer sub.Unsubscribe()
	_ = conn.SetReadDeadline(time.Now().Add(24 * time.Hour))
	go func() {
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				_ = sub.Unsubscribe()
				return
			}
		}
	}()
	for {
		msg, err := sub.NextMsg(30 * time.Second)
		if err == nats.ErrTimeout {
			if conn.WriteControl(websocket.PingMessage, nil, time.Now().Add(time.Second)) != nil {
				return
			}
			continue
		}
		if err != nil {
			return
		}
		if conn.WriteMessage(websocket.TextMessage, msg.Data) != nil {
			return
		}
	}
}

func (h *Handlers) setPresence(r *http.Request, id *auth.Identity, online bool) {
	_, _ = h.Pool.Exec(r.Context(), `INSERT INTO user_presence(user_id,tenant_id,organization_id,online,last_seen_at) VALUES($1,$2,$3,$4,now()) ON CONFLICT(user_id) DO UPDATE SET online=$4,last_seen_at=now()`, id.UserID, id.TenantID, id.OrganizationID, online)
	raw, _ := json.Marshal(map[string]any{"type": "presence.updated", "user_id": id.UserID, "online": online})
	rows, err := h.Pool.Query(r.Context(), `SELECT id::text FROM users WHERE tenant_id=$1 AND organization_id=$2 AND status='active'`, id.TenantID, id.OrganizationID)
	if err != nil {
		return
	}
	defer rows.Close()
	for rows.Next() {
		var uid string
		if rows.Scan(&uid) == nil {
			_ = h.NATS.Publish("communication.user."+uid, raw)
		}
	}
}

type Notification struct {
	ID        string `json:"id"`
	Kind      string `json:"kind"`
	Title     string `json:"title"`
	Body      string `json:"body"`
	TargetURL string `json:"target_url"`
	ReadAt    string `json:"read_at,omitempty"`
	CreatedAt string `json:"created_at"`
}

func (h *Handlers) Notifications(w http.ResponseWriter, r *http.Request) {
	id := auth.IdentityFrom(r.Context())
	rows, err := h.Pool.Query(r.Context(), `SELECT id,kind,title,body,target_url,COALESCE(read_at::text,''),created_at::text FROM notifications WHERE tenant_id=$1 AND organization_id=$2 AND user_id=$3 ORDER BY created_at DESC LIMIT 100`, id.TenantID, id.OrganizationID, id.UserID)
	if err != nil {
		httpx.Internal(w, r)
		return
	}
	defer rows.Close()
	out := []Notification{}
	unread := 0
	for rows.Next() {
		var n Notification
		if rows.Scan(&n.ID, &n.Kind, &n.Title, &n.Body, &n.TargetURL, &n.ReadAt, &n.CreatedAt) != nil {
			httpx.Internal(w, r)
			return
		}
		if n.ReadAt == "" {
			unread++
		}
		out = append(out, n)
	}
	httpx.JSON(w, 200, map[string]any{"notifications": out, "unread": unread})
}
func (h *Handlers) ReadNotification(w http.ResponseWriter, r *http.Request) {
	id := auth.IdentityFrom(r.Context())
	tag, err := h.Pool.Exec(r.Context(), `UPDATE notifications SET read_at=COALESCE(read_at,now()) WHERE id=$1 AND tenant_id=$2 AND organization_id=$3 AND user_id=$4`, chi.URLParam(r, "id"), id.TenantID, id.OrganizationID, id.UserID)
	if err != nil {
		httpx.Internal(w, r)
		return
	}
	if tag.RowsAffected() == 0 {
		httpx.Error(w, r, 404, "NOTIFICATION_NOT_FOUND", "Notification not found")
		return
	}
	w.WriteHeader(204)
}
func (h *Handlers) ReadAllNotifications(w http.ResponseWriter, r *http.Request) {
	id := auth.IdentityFrom(r.Context())
	if _, err := h.Pool.Exec(r.Context(), `UPDATE notifications SET read_at=COALESCE(read_at,now()) WHERE tenant_id=$1 AND organization_id=$2 AND user_id=$3`, id.TenantID, id.OrganizationID, id.UserID); err != nil {
		httpx.Internal(w, r)
		return
	}
	w.WriteHeader(204)
}
