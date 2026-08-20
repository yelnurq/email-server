package official

import (
	"errors"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/yelnurq/email-server/internal/audit"
	"github.com/yelnurq/email-server/internal/auth"
	"github.com/yelnurq/email-server/internal/events"
	"github.com/yelnurq/email-server/internal/httpx"
	"net/http"
	"strings"
)

type Handlers struct {
	Pool  *pgxpool.Pool
	Audit *audit.Logger
}
type Message struct {
	ID                      string `json:"id"`
	SenderName              string `json:"sender_name"`
	SenderRole              string `json:"sender_role"`
	Title                   string `json:"title"`
	Body                    string `json:"body"`
	RequiresAcknowledgement bool   `json:"requires_acknowledgement"`
	ReadAt                  string `json:"read_at,omitempty"`
	AcknowledgedAt          string `json:"acknowledged_at,omitempty"`
	CreatedAt               string `json:"created_at"`
}

func (h *Handlers) List(w http.ResponseWriter, r *http.Request) {
	id := auth.IdentityFrom(r.Context())
	rows, err := h.Pool.Query(r.Context(), `SELECT o.id,COALESCE(u.display_name,u.email::text),o.sender_role,o.title,o.body,o.requires_acknowledgement,COALESCE(x.read_at::text,''),COALESCE(x.acknowledged_at::text,''),o.created_at::text FROM official_messages o JOIN official_message_recipients x ON x.official_message_id=o.id AND x.user_id=$1 JOIN users u ON u.id=o.sender_user_id WHERE o.tenant_id=$2 AND o.organization_id=$3 ORDER BY o.created_at DESC LIMIT 200`, id.UserID, id.TenantID, id.OrganizationID)
	if err != nil {
		httpx.Internal(w, r)
		return
	}
	defer rows.Close()
	out := []Message{}
	for rows.Next() {
		var m Message
		if rows.Scan(&m.ID, &m.SenderName, &m.SenderRole, &m.Title, &m.Body, &m.RequiresAcknowledgement, &m.ReadAt, &m.AcknowledgedAt, &m.CreatedAt) != nil {
			httpx.Internal(w, r)
			return
		}
		out = append(out, m)
	}
	httpx.JSON(w, 200, map[string]any{"messages": out})
}

type createRequest struct {
	Title                   string   `json:"title"`
	Body                    string   `json:"body"`
	RequiresAcknowledgement bool     `json:"requires_acknowledgement"`
	WholeOrganization       bool     `json:"whole_organization"`
	DepartmentIDs           []string `json:"department_ids"`
	UserIDs                 []string `json:"user_ids"`
}

func (h *Handlers) Create(w http.ResponseWriter, r *http.Request) {
	id := auth.IdentityFrom(r.Context())
	if !id.HasPermission("official.send.department") && !id.HasPermission("official.send.organization") {
		httpx.Error(w, r, http.StatusForbidden, "FORBIDDEN", "Official message send permission required")
		return
	}
	var req createRequest
	if httpx.Decode(w, r, &req) != nil {
		return
	}
	req.Title = strings.TrimSpace(req.Title)
	req.Body = strings.TrimSpace(req.Body)
	if req.Title == "" || req.Body == "" {
		httpx.Error(w, r, 400, "INVALID_OFFICIAL", "Title and body are required")
		return
	}
	if req.WholeOrganization && !id.HasPermission("official.send.organization") {
		httpx.Error(w, r, 403, "FORBIDDEN", "Organization-wide send permission required")
		return
	}
	if !req.WholeOrganization && len(req.DepartmentIDs) > 0 && !id.HasPermission("official.send.department") {
		httpx.Error(w, r, 403, "FORBIDDEN", "Department send permission required")
		return
	}
	tx, err := h.Pool.Begin(r.Context())
	if err != nil {
		httpx.Internal(w, r)
		return
	}
	defer tx.Rollback(r.Context())
	role := "member"
	if len(id.Roles) > 0 {
		role = id.Roles[0].Role
	}
	var oid string
	if tx.QueryRow(r.Context(), `INSERT INTO official_messages(tenant_id,organization_id,sender_user_id,sender_role,title,body,requires_acknowledgement) VALUES($1,$2,$3,$4,$5,$6,$7) RETURNING id`, id.TenantID, id.OrganizationID, id.UserID, role, req.Title, req.Body, req.RequiresAcknowledgement).Scan(&oid) != nil {
		httpx.Internal(w, r)
		return
	}
	tag, err := tx.Exec(r.Context(), `INSERT INTO official_message_recipients(official_message_id,user_id) SELECT $1,u.id FROM users u WHERE u.tenant_id=$2 AND u.organization_id=$3 AND u.status='active' AND ($4 OR u.id=ANY($5::uuid[]) OR u.department_id=ANY($6::uuid[])) ON CONFLICT DO NOTHING`, oid, id.TenantID, id.OrganizationID, req.WholeOrganization, req.UserIDs, req.DepartmentIDs)
	if err != nil {
		httpx.Error(w, r, 400, "INVALID_RECIPIENTS", "Recipients must belong to the organization")
		return
	}
	if tag.RowsAffected() == 0 {
		httpx.Error(w, r, 400, "NO_RECIPIENTS", "Select at least one recipient")
		return
	}
	rows, _ := tx.Query(r.Context(), `SELECT user_id::text FROM official_message_recipients WHERE official_message_id=$1`, oid)
	for rows.Next() {
		var uid string
		rows.Scan(&uid)
		_, _ = tx.Exec(r.Context(), `INSERT INTO notifications(tenant_id,organization_id,user_id,kind,title,body,target_url) VALUES($1,$2,$3,'official',$4,$5,'/mail/official')`, id.TenantID, id.OrganizationID, uid, req.Title, req.Body)
		_ = events.Enqueue(r.Context(), tx, "communication.user."+uid, map[string]any{"type": "official.created", "official_message_id": oid})
	}
	rows.Close()
	if tx.Commit(r.Context()) != nil {
		httpx.Internal(w, r)
		return
	}
	h.Audit.Record(r.Context(), audit.Entry{TenantID: id.TenantID, ActorUserID: id.UserID, Action: "official.create", ResourceType: "official_message", ResourceID: oid, Detail: map[string]any{"recipient_count": tag.RowsAffected(), "whole_organization": req.WholeOrganization, "department_count": len(req.DepartmentIDs)}})
	httpx.JSON(w, 201, map[string]any{"id": oid, "recipient_count": tag.RowsAffected()})
}
func (h *Handlers) Read(w http.ResponseWriter, r *http.Request) {
	id := auth.IdentityFrom(r.Context())
	tag, err := h.Pool.Exec(r.Context(), `UPDATE official_message_recipients x SET read_at=COALESCE(read_at,now()) FROM official_messages o WHERE x.official_message_id=o.id AND o.id=$1 AND x.user_id=$2 AND o.tenant_id=$3 AND o.organization_id=$4`, chi.URLParam(r, "id"), id.UserID, id.TenantID, id.OrganizationID)
	if err != nil {
		httpx.Internal(w, r)
		return
	}
	if tag.RowsAffected() == 0 {
		httpx.Error(w, r, 404, "OFFICIAL_NOT_FOUND", "Official message not found")
		return
	}
	w.WriteHeader(204)
}
func (h *Handlers) Acknowledge(w http.ResponseWriter, r *http.Request) {
	id := auth.IdentityFrom(r.Context())
	tag, err := h.Pool.Exec(r.Context(), `UPDATE official_message_recipients x SET read_at=COALESCE(read_at,now()),acknowledged_at=now() FROM official_messages o WHERE x.official_message_id=o.id AND o.id=$1 AND x.user_id=$2 AND o.tenant_id=$3 AND o.organization_id=$4 AND o.requires_acknowledgement`, chi.URLParam(r, "id"), id.UserID, id.TenantID, id.OrganizationID)
	if err != nil {
		httpx.Internal(w, r)
		return
	}
	if tag.RowsAffected() == 0 {
		httpx.Error(w, r, 404, "OFFICIAL_NOT_FOUND", "Acknowledgement not required or message not found")
		return
	}
	w.WriteHeader(204)
}
func (h *Handlers) Stats(w http.ResponseWriter, r *http.Request) {
	id := auth.IdentityFrom(r.Context())
	var sent, delivered, read, ack int
	err := h.Pool.QueryRow(r.Context(), `SELECT count(*),count(delivered_at),count(read_at),count(acknowledged_at) FROM official_message_recipients x JOIN official_messages o ON o.id=x.official_message_id WHERE o.id=$1 AND o.sender_user_id=$2 AND o.tenant_id=$3 AND o.organization_id=$4`, chi.URLParam(r, "id"), id.UserID, id.TenantID, id.OrganizationID).Scan(&sent, &delivered, &read, &ack)
	if errors.Is(err, pgx.ErrNoRows) {
		httpx.Error(w, r, 404, "OFFICIAL_NOT_FOUND", "Official message not found")
		return
	}
	if err != nil {
		httpx.Internal(w, r)
		return
	}
	httpx.JSON(w, 200, map[string]int{"sent": sent, "delivered": delivered, "read": read, "acknowledged": ack})
}
