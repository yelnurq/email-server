package bulkmail

import (
	"context"
	"errors"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/yelnurq/email-server/internal/audit"
	"github.com/yelnurq/email-server/internal/auth"
	"github.com/yelnurq/email-server/internal/httpx"
	"github.com/yelnurq/email-server/internal/messages"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

type Handlers struct {
	Pool  *pgxpool.Pool
	Audit *audit.Logger
}
type Campaign struct {
	ID                string   `json:"id"`
	Subject           string   `json:"subject"`
	Status            string   `json:"status"`
	DepartmentIDs     []string `json:"department_ids"`
	WholeOrganization bool     `json:"whole_organization"`
	ScheduledAt       string   `json:"scheduled_at,omitempty"`
	CreatedAt         string   `json:"created_at"`
	Total             int      `json:"total"`
	Queued            int      `json:"queued"`
	Sent              int      `json:"sent"`
	Delivered         int      `json:"delivered"`
	Deferred          int      `json:"deferred"`
	Bounced           int      `json:"bounced"`
	Failed            int      `json:"failed"`
	Read              int      `json:"read"`
}

func (h *Handlers) List(w http.ResponseWriter, r *http.Request) {
	id := auth.IdentityFrom(r.Context())
	rows, err := h.Pool.Query(r.Context(), `SELECT c.id,c.subject,c.status,c.department_ids,c.whole_organization,COALESCE(c.scheduled_at::text,''),c.created_at::text,count(x.*),count(*)FILTER(WHERE x.status='queued'),count(*)FILTER(WHERE x.status='sent'),count(*)FILTER(WHERE x.status='delivered'),count(*)FILTER(WHERE x.status='deferred'),count(*)FILTER(WHERE x.status='bounced'),count(*)FILTER(WHERE x.status='failed'),count(*)FILTER(WHERE x.status='read') FROM bulk_email_campaigns c LEFT JOIN bulk_email_recipients x ON x.campaign_id=c.id WHERE c.tenant_id=$1 AND c.organization_id=$2 GROUP BY c.id ORDER BY c.created_at DESC`, id.TenantID, id.OrganizationID)
	if err != nil {
		httpx.Internal(w, r)
		return
	}
	defer rows.Close()
	out := []Campaign{}
	for rows.Next() {
		var c Campaign
		if rows.Scan(&c.ID, &c.Subject, &c.Status, &c.DepartmentIDs, &c.WholeOrganization, &c.ScheduledAt, &c.CreatedAt, &c.Total, &c.Queued, &c.Sent, &c.Delivered, &c.Deferred, &c.Bounced, &c.Failed, &c.Read) != nil {
			httpx.Internal(w, r)
			return
		}
		out = append(out, c)
	}
	httpx.JSON(w, 200, map[string]any{"campaigns": out})
}

type createRequest struct {
	Subject           string   `json:"subject"`
	BodyText          string   `json:"body_text"`
	DepartmentIDs     []string `json:"department_ids"`
	UserIDs           []string `json:"user_ids"`
	WholeOrganization bool     `json:"whole_organization"`
	ScheduledAt       string   `json:"scheduled_at"`
}

func (h *Handlers) Create(w http.ResponseWriter, r *http.Request) {
	id := auth.IdentityFrom(r.Context())
	var req createRequest
	if httpx.Decode(w, r, &req) != nil {
		return
	}
	req.Subject = strings.TrimSpace(req.Subject)
	if req.Subject == "" || strings.TrimSpace(req.BodyText) == "" {
		httpx.Error(w, r, 400, "INVALID_CAMPAIGN", "Subject and body are required")
		return
	}
	var scheduled any
	if req.ScheduledAt != "" {
		scheduled = req.ScheduledAt
	}
	tx, err := h.Pool.Begin(r.Context())
	if err != nil {
		httpx.Internal(w, r)
		return
	}
	defer tx.Rollback(r.Context())
	var cid string
	err = tx.QueryRow(r.Context(), `INSERT INTO bulk_email_campaigns(tenant_id,organization_id,created_by,subject,body_text,department_ids,user_ids,whole_organization,scheduled_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9) RETURNING id`, id.TenantID, id.OrganizationID, id.UserID, req.Subject, req.BodyText, req.DepartmentIDs, req.UserIDs, req.WholeOrganization, scheduled).Scan(&cid)
	if err != nil {
		httpx.Error(w, r, 400, "INVALID_CAMPAIGN", "Invalid campaign fields")
		return
	}
	tag, err := tx.Exec(r.Context(), `INSERT INTO bulk_email_recipients(campaign_id,user_id,address,department_id) SELECT $1,u.id,COALESCE(m.address,u.email),u.department_id FROM users u LEFT JOIN LATERAL(SELECT address FROM mailboxes WHERE user_id=u.id AND status='active' ORDER BY created_at LIMIT 1)m ON true WHERE u.tenant_id=$2 AND u.organization_id=$3 AND u.status='active' AND ($4 OR u.department_id=ANY($5::uuid[]) OR u.id=ANY($6::uuid[])) ON CONFLICT DO NOTHING`, cid, id.TenantID, id.OrganizationID, req.WholeOrganization, req.DepartmentIDs, req.UserIDs)
	if err != nil {
		httpx.Error(w, r, 400, "INVALID_RECIPIENTS", "Recipient selection is invalid")
		return
	}
	if tag.RowsAffected() == 0 {
		httpx.Error(w, r, 400, "NO_RECIPIENTS", "No recipients selected")
		return
	}
	if tx.Commit(r.Context()) != nil {
		httpx.Internal(w, r)
		return
	}
	h.Audit.Record(r.Context(), audit.Entry{TenantID: id.TenantID, ActorUserID: id.UserID, Action: "bulk_email.create", ResourceType: "bulk_email_campaign", ResourceID: cid, Detail: map[string]any{"recipient_count": tag.RowsAffected(), "department_count": len(req.DepartmentIDs), "whole_organization": req.WholeOrganization}})
	httpx.JSON(w, 201, map[string]any{"id": cid, "recipient_count": tag.RowsAffected()})
}

type sendRequest struct {
	ScheduledAt string `json:"scheduled_at"`
}

func (h *Handlers) Send(w http.ResponseWriter, r *http.Request) {
	id := auth.IdentityFrom(r.Context())
	var req sendRequest
	if r.ContentLength > 0 && httpx.Decode(w, r, &req) != nil {
		return
	}
	status := "queued"
	var schedule any
	if req.ScheduledAt != "" {
		status = "scheduled"
		schedule = req.ScheduledAt
	}
	tag, err := h.Pool.Exec(r.Context(), `UPDATE bulk_email_campaigns SET status=$1,scheduled_at=COALESCE($2,scheduled_at),updated_at=now() WHERE id=$3 AND tenant_id=$4 AND organization_id=$5 AND status='draft'`, status, schedule, chi.URLParam(r, "id"), id.TenantID, id.OrganizationID)
	if err != nil {
		httpx.Error(w, r, 400, "INVALID_SCHEDULE", "Invalid schedule")
		return
	}
	if tag.RowsAffected() == 0 {
		httpx.Error(w, r, 409, "CAMPAIGN_NOT_DRAFT", "Campaign is not a draft")
		return
	}
	h.Audit.Record(r.Context(), audit.Entry{TenantID: id.TenantID, ActorUserID: id.UserID, Action: "bulk_email.send", ResourceType: "bulk_email_campaign", ResourceID: chi.URLParam(r, "id"), Detail: map[string]any{"scheduled": status == "scheduled"}})
	w.WriteHeader(204)
}

type Processor struct {
	Pool     *pgxpool.Pool
	Messages *messages.Service
	Audit    *audit.Logger
	Log      *slog.Logger
}

func (p *Processor) Run(ctx context.Context) {
	tick := time.NewTicker(5 * time.Second)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			p.process(ctx)
			p.reconcile(ctx)
		}
	}
}
func (p *Processor) process(ctx context.Context) {
	var cid, tenant, org, creator, subject, body string
	err := p.Pool.QueryRow(ctx, `UPDATE bulk_email_campaigns SET status='sending',updated_at=now() WHERE id=(SELECT id FROM bulk_email_campaigns WHERE status='queued' OR(status='scheduled' AND scheduled_at<=now()) ORDER BY created_at FOR UPDATE SKIP LOCKED LIMIT 1) RETURNING id,tenant_id,organization_id,created_by,subject,body_text`).Scan(&cid, &tenant, &org, &creator, &subject, &body)
	if errors.Is(err, pgx.ErrNoRows) {
		return
	}
	if err != nil {
		return
	}
	mailbox, err := p.Messages.MailboxForUser(ctx, tenant, creator)
	if err != nil {
		_, _ = p.Pool.Exec(ctx, `UPDATE bulk_email_campaigns SET status='failed' WHERE id=$1`, cid)
		return
	}
	rows, err := p.Pool.Query(ctx, `SELECT address::text FROM bulk_email_recipients WHERE campaign_id=$1 AND status='queued'`, cid)
	if err != nil {
		return
	}
	addresses := []string{}
	for rows.Next() {
		var a string
		rows.Scan(&a)
		addresses = append(addresses, a)
	}
	rows.Close()
	failed := 0
	for _, address := range addresses {
		publicID, err := p.Messages.Accept(ctx, tenant, mailbox, "Bulk Email", messages.SendInput{To: []string{address}, Subject: subject, Text: body, SenderUserID: creator})
		status := "sent"
		detail := ""
		if err != nil {
			status = "failed"
			detail = err.Error()
			failed++
		}
		_, _ = p.Pool.Exec(ctx, `UPDATE bulk_email_recipients SET status=$1,error=$2,message_public_id=NULLIF($5,''),updated_at=now() WHERE campaign_id=$3 AND address=$4`, status, detail, cid, address, publicID)
	}
	status := "completed"
	if failed == len(addresses) {
		status = "failed"
	}
	_, _ = p.Pool.Exec(ctx, `UPDATE bulk_email_campaigns SET status=$1,updated_at=now() WHERE id=$2`, status, cid)
}

func (p *Processor) reconcile(ctx context.Context) {
	_, err := p.Pool.Exec(ctx, `UPDATE bulk_email_recipients b SET status=CASE WHEN mr.status='delivered' THEN 'delivered' WHEN mr.status='failed' THEN 'failed' ELSE b.status END,error=CASE WHEN mr.status='failed' THEN mr.error ELSE b.error END,updated_at=now() FROM messages m JOIN message_recipients mr ON mr.message_id=m.id WHERE b.message_public_id=m.public_id AND b.address=mr.address AND b.status IN('sent','deferred') AND mr.status IN('delivered','failed')`)
	if err != nil {
		p.Log.Error("bulk analytics reconcile failed", slog.String("error", err.Error()))
	}
}
