package worklist

import (
	"context"
	"errors"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nats-io/nats.go"
	"github.com/yelnurq/email-server/internal/audit"
	"github.com/yelnurq/email-server/internal/auth"
	"github.com/yelnurq/email-server/internal/httpx"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

type Handlers struct {
	Pool  *pgxpool.Pool
	Audit *audit.Logger
}
type Task struct {
	ID               string `json:"id"`
	OwnerUserID      string `json:"owner_user_id"`
	AssignedByUserID string `json:"assigned_by_user_id,omitempty"`
	AssignedByName   string `json:"assigned_by_name,omitempty"`
	Title            string `json:"title"`
	Description      string `json:"description"`
	DueAt            string `json:"due_at,omitempty"`
	Priority         string `json:"priority"`
	Status           string `json:"status"`
	SourceType       string `json:"source_type"`
	SourceID         string `json:"source_id,omitempty"`
	ReminderAt       string `json:"reminder_at,omitempty"`
	CreatedAt        string `json:"created_at"`
}

func (h *Handlers) List(w http.ResponseWriter, r *http.Request) {
	id := auth.IdentityFrom(r.Context())
	rows, err := h.Pool.Query(r.Context(), `SELECT t.id,t.owner_user_id,COALESCE(t.assigned_by_user_id::text,''),COALESCE(a.display_name,a.email::text,''),t.title,t.description,COALESCE(t.due_at::text,''),t.priority,t.status,t.source_type,COALESCE(t.source_id::text,''),COALESCE(min(r.remind_at)::text,''),t.created_at::text FROM tasks t LEFT JOIN users a ON a.id=t.assigned_by_user_id LEFT JOIN reminders r ON r.task_id=t.id AND r.status='pending' WHERE t.tenant_id=$1 AND t.organization_id=$2 AND (t.owner_user_id=$3 OR t.assigned_by_user_id=$3) GROUP BY t.id,a.display_name,a.email ORDER BY (t.status='done'),t.due_at NULLS LAST,t.created_at DESC`, id.TenantID, id.OrganizationID, id.UserID)
	if err != nil {
		httpx.Internal(w, r)
		return
	}
	defer rows.Close()
	out := []Task{}
	for rows.Next() {
		var t Task
		if rows.Scan(&t.ID, &t.OwnerUserID, &t.AssignedByUserID, &t.AssignedByName, &t.Title, &t.Description, &t.DueAt, &t.Priority, &t.Status, &t.SourceType, &t.SourceID, &t.ReminderAt, &t.CreatedAt) != nil {
			httpx.Internal(w, r)
			return
		}
		out = append(out, t)
	}
	httpx.JSON(w, 200, map[string]any{"tasks": out})
}

type createRequest struct {
	OwnerUserID string `json:"owner_user_id"`
	Title       string `json:"title"`
	Description string `json:"description"`
	DueAt       string `json:"due_at"`
	ReminderAt  string `json:"reminder_at"`
	Priority    string `json:"priority"`
	SourceType  string `json:"source_type"`
	SourceID    string `json:"source_id"`
}

func (h *Handlers) Create(w http.ResponseWriter, r *http.Request) {
	id := auth.IdentityFrom(r.Context())
	var req createRequest
	if httpx.Decode(w, r, &req) != nil {
		return
	}
	req.Title = strings.TrimSpace(req.Title)
	if req.Title == "" {
		httpx.Error(w, r, 400, "INVALID_TASK", "Title is required")
		return
	}
	owner := req.OwnerUserID
	if owner == "" {
		owner = id.UserID
	}
	assigned := any(nil)
	if owner != id.UserID {
		if !id.HasPermission("tasks.assign.department") {
			httpx.Error(w, r, 403, "FORBIDDEN", "Task assignment permission required")
			return
		}
		var allowed bool
		if h.Pool.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM departments d JOIN users target ON target.department_id=d.id WHERE d.manager_user_id=$1 AND target.id=$2 AND d.tenant_id=$3 AND d.organization_id=$4)`, id.UserID, owner, id.TenantID, id.OrganizationID).Scan(&allowed) != nil || !allowed {
			httpx.Error(w, r, 403, "FORBIDDEN", "Managers may assign only within their department")
			return
		}
		assigned = id.UserID
	}
	priority := req.Priority
	if priority == "" {
		priority = "normal"
	}
	source := req.SourceType
	if source == "" {
		source = "manual"
	}
	var due, reminder, sourceID any
	if req.DueAt != "" {
		due = req.DueAt
	}
	if req.ReminderAt != "" {
		reminder = req.ReminderAt
	}
	if req.SourceID != "" {
		sourceID = req.SourceID
	}
	tx, err := h.Pool.Begin(r.Context())
	if err != nil {
		httpx.Internal(w, r)
		return
	}
	defer tx.Rollback(r.Context())
	var taskID string
	err = tx.QueryRow(r.Context(), `INSERT INTO tasks(tenant_id,organization_id,owner_user_id,assigned_by_user_id,title,description,due_at,priority,source_type,source_id) SELECT $1,$2,u.id,$4,$5,$6,$7,$8,$9,$10 FROM users u WHERE u.id=$3 AND u.tenant_id=$1 AND u.organization_id=$2 RETURNING id`, id.TenantID, id.OrganizationID, owner, assigned, req.Title, req.Description, due, priority, source, sourceID).Scan(&taskID)
	if errors.Is(err, pgx.ErrNoRows) {
		httpx.Error(w, r, 400, "INVALID_OWNER", "Owner must belong to the organization")
		return
	}
	if err != nil {
		httpx.Error(w, r, 400, "INVALID_TASK", "Invalid task fields")
		return
	}
	if reminder != nil {
		if _, err = tx.Exec(r.Context(), `INSERT INTO reminders(tenant_id,organization_id,user_id,created_by_user_id,task_id,source_type,source_id,title,remind_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)`, id.TenantID, id.OrganizationID, owner, id.UserID, taskID, source, sourceID, req.Title, reminder); err != nil {
			httpx.Error(w, r, 400, "INVALID_REMINDER", "Invalid reminder time")
			return
		}
	}
	if assigned != nil {
		_, _ = tx.Exec(r.Context(), `INSERT INTO notifications(tenant_id,organization_id,user_id,kind,title,body,target_url) VALUES($1,$2,$3,'assigned_task',$4,$5,'/mail/my-list')`, id.TenantID, id.OrganizationID, owner, req.Title, "Assigned by "+id.DisplayName)
	}
	if tx.Commit(r.Context()) != nil {
		httpx.Internal(w, r)
		return
	}
	if assigned != nil {
		h.Audit.Record(r.Context(), audit.Entry{TenantID: id.TenantID, ActorUserID: id.UserID, Action: "task.assign", ResourceType: "task", ResourceID: taskID, Detail: map[string]any{"owner_user_id": owner}})
	}
	httpx.JSON(w, 201, map[string]string{"id": taskID})
}

type patchRequest struct {
	Title       *string `json:"title"`
	Description *string `json:"description"`
	DueAt       *string `json:"due_at"`
	Priority    *string `json:"priority"`
	Status      *string `json:"status"`
}

func (h *Handlers) Patch(w http.ResponseWriter, r *http.Request) {
	id := auth.IdentityFrom(r.Context())
	var req patchRequest
	if httpx.Decode(w, r, &req) != nil {
		return
	}
	tag, err := h.Pool.Exec(r.Context(), `UPDATE tasks SET title=COALESCE($1,title),description=COALESCE($2,description),due_at=CASE WHEN $3::text IS NULL THEN due_at WHEN $3='' THEN NULL ELSE $3::timestamptz END,priority=COALESCE($4,priority),status=COALESCE($5,status),updated_at=now() WHERE id=$6 AND tenant_id=$7 AND organization_id=$8 AND owner_user_id=$9`, req.Title, req.Description, req.DueAt, req.Priority, req.Status, chi.URLParam(r, "id"), id.TenantID, id.OrganizationID, id.UserID)
	if err != nil {
		httpx.Error(w, r, 400, "INVALID_TASK", "Invalid task fields")
		return
	}
	if tag.RowsAffected() == 0 {
		httpx.Error(w, r, 404, "TASK_NOT_FOUND", "Task not found")
		return
	}
	w.WriteHeader(204)
}
func (h *Handlers) Delete(w http.ResponseWriter, r *http.Request) {
	id := auth.IdentityFrom(r.Context())
	tag, err := h.Pool.Exec(r.Context(), `DELETE FROM tasks WHERE id=$1 AND tenant_id=$2 AND organization_id=$3 AND owner_user_id=$4`, chi.URLParam(r, "id"), id.TenantID, id.OrganizationID, id.UserID)
	if err != nil {
		httpx.Internal(w, r)
		return
	}
	if tag.RowsAffected() == 0 {
		httpx.Error(w, r, 404, "TASK_NOT_FOUND", "Task not found")
		return
	}
	w.WriteHeader(204)
}

type Processor struct {
	Pool *pgxpool.Pool
	NATS *nats.Conn
	Log  *slog.Logger
}

func (p *Processor) Run(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			p.tick(ctx)
		}
	}
}
func (p *Processor) tick(ctx context.Context) {
	tx, err := p.Pool.Begin(ctx)
	if err != nil {
		return
	}
	defer tx.Rollback(ctx)
	rows, err := tx.Query(ctx, `SELECT r.id,r.tenant_id,r.organization_id,r.user_id,r.title FROM reminders r WHERE r.status='pending' AND r.remind_at<=now() FOR UPDATE SKIP LOCKED LIMIT 100`)
	if err != nil {
		return
	}
	type due struct{ id, tenant, org, user, title string }
	items := []due{}
	for rows.Next() {
		var d due
		if rows.Scan(&d.id, &d.tenant, &d.org, &d.user, &d.title) == nil {
			items = append(items, d)
		}
	}
	rows.Close()
	for _, d := range items {
		_, _ = tx.Exec(ctx, `INSERT INTO notifications(tenant_id,organization_id,user_id,kind,title,body,target_url) VALUES($1,$2,$3,'reminder',$4,'Reminder due','/mail/my-list')`, d.tenant, d.org, d.user, d.title)
		_, _ = tx.Exec(ctx, `UPDATE reminders SET notified_at=now(),status='overdue' WHERE id=$1`, d.id)
		_ = p.NATS.Publish("communication.user."+d.user, []byte(`{"type":"notification.created"}`))
	}
	_ = tx.Commit(ctx)
}
