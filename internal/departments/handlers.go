// Package departments manages organization-scoped departments and the
// employee directory used by administration and mail recipient selection.
package departments

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/yelnurq/email-server/internal/audit"
	"github.com/yelnurq/email-server/internal/auth"
	"github.com/yelnurq/email-server/internal/httpx"
)

type Handlers struct {
	Pool  *pgxpool.Pool
	Audit *audit.Logger
	Log   *slog.Logger
}

type Department struct {
	ID             string `json:"id"`
	OrganizationID string `json:"organization_id"`
	Name           string `json:"name"`
	Description    string `json:"description"`
	ManagerUserID  string `json:"manager_user_id,omitempty"`
	ManagerName    string `json:"manager_name,omitempty"`
	ManagerEmail   string `json:"manager_email,omitempty"`
	EmployeeCount  int    `json:"employee_count"`
	CreatedAt      string `json:"created_at"`
}

type DirectoryUser struct {
	ID             string `json:"id"`
	DisplayName    string `json:"display_name"`
	Email          string `json:"email"`
	MailboxAddress string `json:"mailbox_address"`
	DepartmentID   string `json:"department_id,omitempty"`
	DepartmentName string `json:"department_name,omitempty"`
	IsOnline       bool   `json:"is_online"`
}

// List returns departments in one organization with manager and member count.
func (h *Handlers) List(w http.ResponseWriter, r *http.Request) {
	id := auth.IdentityFrom(r.Context())
	orgID := strings.TrimSpace(r.URL.Query().Get("organization_id"))
	if orgID == "" {
		orgID = id.OrganizationID
	}
	if orgID == "" || (!id.TenantWide() && orgID != id.OrganizationID) {
		httpx.Error(w, r, http.StatusForbidden, "FORBIDDEN", "Organization access denied")
		return
	}
	rows, err := h.Pool.Query(r.Context(), `
		SELECT d.id, d.organization_id, d.name, d.description,
		       COALESCE(d.manager_user_id::text, ''), COALESCE(mu.display_name, ''),
		       COALESCE(mu.email::text, ''), count(u.id), d.created_at::text
		FROM departments d
		LEFT JOIN users mu ON mu.id = d.manager_user_id
		LEFT JOIN users u ON u.department_id = d.id AND u.status = 'active'
		WHERE d.tenant_id = $1 AND d.organization_id = $2
		GROUP BY d.id, mu.display_name, mu.email
		ORDER BY lower(d.name)`, id.TenantID, orgID)
	if err != nil {
		h.Log.Error("department list failed", slog.String("error", err.Error()))
		httpx.Internal(w, r)
		return
	}
	depts := []Department{}
	defer rows.Close()
	for rows.Next() {
		var d Department
		if err := rows.Scan(&d.ID, &d.OrganizationID, &d.Name, &d.Description, &d.ManagerUserID, &d.ManagerName, &d.ManagerEmail, &d.EmployeeCount, &d.CreatedAt); err != nil {
			httpx.Internal(w, r)
			return
		}
		depts = append(depts, d)
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"departments": depts})
}

type createRequest struct {
	OrganizationID string `json:"organization_id"`
	Name           string `json:"name"`
	Description    string `json:"description"`
	ManagerUserID  string `json:"manager_user_id"`
}

func (h *Handlers) Create(w http.ResponseWriter, r *http.Request) {
	id := auth.IdentityFrom(r.Context())
	var req createRequest
	if err := httpx.Decode(w, r, &req); err != nil {
		httpx.Error(w, r, http.StatusBadRequest, "INVALID_BODY", err.Error())
		return
	}
	orgID := strings.TrimSpace(req.OrganizationID)
	if orgID == "" {
		orgID = id.OrganizationID
	}
	if orgID == "" || (!id.TenantWide() && orgID != id.OrganizationID) {
		httpx.Error(w, r, http.StatusForbidden, "FORBIDDEN", "Organization access denied")
		return
	}
	name := strings.TrimSpace(req.Name)
	description := strings.TrimSpace(req.Description)
	if name == "" || len(name) > 120 || len(description) > 1000 {
		httpx.Error(w, r, http.StatusBadRequest, "INVALID_DEPARTMENT", "Name is required (max 120); description max 1000")
		return
	}
	var manager any
	if req.ManagerUserID != "" {
		manager = req.ManagerUserID
	}
	var d Department
	err := h.Pool.QueryRow(r.Context(), `
		INSERT INTO departments (tenant_id, organization_id, name, description, manager_user_id)
		SELECT $1, o.id, $3, $4, $5 FROM organizations o
		WHERE o.id = $2 AND o.tenant_id = $1
		ON CONFLICT DO NOTHING
		RETURNING id, organization_id, name, description, COALESCE(manager_user_id::text, ''), created_at::text`,
		id.TenantID, orgID, name, description, manager).
		Scan(&d.ID, &d.OrganizationID, &d.Name, &d.Description, &d.ManagerUserID, &d.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		httpx.Error(w, r, http.StatusConflict, "DEPARTMENT_EXISTS", "Department name already exists or organization is invalid")
		return
	}
	if err != nil {
		h.Log.Error("department create failed", slog.String("error", err.Error()))
		httpx.Error(w, r, http.StatusBadRequest, "INVALID_MANAGER", "Manager must belong to the same organization")
		return
	}
	h.Audit.Record(r.Context(), audit.Entry{TenantID: id.TenantID, ActorUserID: id.UserID, Action: "department.create", ResourceType: "department", ResourceID: d.ID, Detail: map[string]any{"name": d.Name, "organization_id": orgID}})
	httpx.JSON(w, http.StatusCreated, d)
}

type patchRequest struct {
	Name          *string `json:"name"`
	Description   *string `json:"description"`
	ManagerUserID *string `json:"manager_user_id"`
}

func (h *Handlers) Patch(w http.ResponseWriter, r *http.Request) {
	id := auth.IdentityFrom(r.Context())
	var req patchRequest
	if err := httpx.Decode(w, r, &req); err != nil {
		httpx.Error(w, r, http.StatusBadRequest, "INVALID_BODY", err.Error())
		return
	}
	name, description := any(nil), any(nil)
	managerSet, manager := false, any(nil)
	if req.Name != nil {
		v := strings.TrimSpace(*req.Name)
		if v == "" || len(v) > 120 {
			httpx.Error(w, r, http.StatusBadRequest, "INVALID_NAME", "Name is required (max 120)")
			return
		}
		name = v
	}
	if req.Description != nil {
		v := strings.TrimSpace(*req.Description)
		if len(v) > 1000 {
			httpx.Error(w, r, http.StatusBadRequest, "INVALID_DESCRIPTION", "Description max 1000")
			return
		}
		description = v
	}
	if req.ManagerUserID != nil {
		managerSet = true
		if *req.ManagerUserID != "" {
			manager = *req.ManagerUserID
		}
	}
	tag, err := h.Pool.Exec(r.Context(), `
		UPDATE departments SET
		 name = COALESCE($5, name), description = COALESCE($6, description),
		 manager_user_id = CASE WHEN $7 THEN $8::uuid ELSE manager_user_id END, updated_at = now()
		WHERE id = $1 AND tenant_id = $2 AND ($3 OR organization_id = $4)`,
		chi.URLParam(r, "id"), id.TenantID, id.TenantWide(), id.OrganizationID, name, description, managerSet, manager)
	if err != nil {
		httpx.Error(w, r, http.StatusBadRequest, "INVALID_DEPARTMENT", "Name must be unique and manager must belong to the organization")
		return
	}
	if tag.RowsAffected() == 0 {
		httpx.Error(w, r, http.StatusNotFound, "DEPARTMENT_NOT_FOUND", "Department not found")
		return
	}
	h.Audit.Record(r.Context(), audit.Entry{TenantID: id.TenantID, ActorUserID: id.UserID, Action: "department.update", ResourceType: "department", ResourceID: chi.URLParam(r, "id")})
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handlers) Delete(w http.ResponseWriter, r *http.Request) {
	id := auth.IdentityFrom(r.Context())
	tag, err := h.Pool.Exec(r.Context(), `DELETE FROM departments WHERE id = $1 AND tenant_id = $2 AND ($3 OR organization_id = $4)`, chi.URLParam(r, "id"), id.TenantID, id.TenantWide(), id.OrganizationID)
	if err != nil {
		httpx.Internal(w, r)
		return
	}
	if tag.RowsAffected() == 0 {
		httpx.Error(w, r, http.StatusNotFound, "DEPARTMENT_NOT_FOUND", "Department not found")
		return
	}
	h.Audit.Record(r.Context(), audit.Entry{TenantID: id.TenantID, ActorUserID: id.UserID, Action: "department.delete", ResourceType: "department", ResourceID: chi.URLParam(r, "id")})
	w.WriteHeader(http.StatusNoContent)
}

// Directory searches active employees with mailboxes inside the caller's own organization.
func (h *Handlers) Directory(w http.ResponseWriter, r *http.Request) {
	id := auth.IdentityFrom(r.Context())
	departmentID := strings.TrimSpace(r.URL.Query().Get("department_id"))
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	limit := 100
	if requested, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && requested > 0 {
		limit = min(requested, 1000)
	}
	like := "%" + q + "%"
	rows, err := h.Pool.Query(r.Context(), `
		SELECT u.id, u.display_name, u.email::text, COALESCE(m.address::text, u.email::text),
		       COALESCE(d.id::text, ''), COALESCE(d.name, ''),COALESCE(p.online AND p.last_seen_at>now()-interval '2 minutes',false)
		FROM users u
		LEFT JOIN LATERAL (
			SELECT address FROM mailboxes
			WHERE user_id = u.id AND status = 'active'
			ORDER BY created_at LIMIT 1
		) m ON true
		LEFT JOIN departments d ON d.id = u.department_id
		LEFT JOIN user_presence p ON p.user_id=u.id
		WHERE u.tenant_id = $1 AND u.organization_id = $2 AND u.status = 'active'
		  AND ($3 = '' OR u.department_id::text = $3)
		  AND ($4 = '' OR u.display_name ILIKE $5 OR u.email::text ILIKE $5 OR m.address::text ILIKE $5 OR d.name ILIKE $5)
		ORDER BY lower(u.display_name), lower(u.email::text)
		LIMIT $6`, id.TenantID, id.OrganizationID, departmentID, q, like, limit)
	if err != nil {
		h.Log.Error("directory search failed", slog.String("error", err.Error()))
		httpx.Internal(w, r)
		return
	}
	defer rows.Close()
	users := []DirectoryUser{}
	for rows.Next() {
		var u DirectoryUser
		if err := rows.Scan(&u.ID, &u.DisplayName, &u.Email, &u.MailboxAddress, &u.DepartmentID, &u.DepartmentName, &u.IsOnline); err != nil {
			httpx.Internal(w, r)
			return
		}
		users = append(users, u)
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"users": users, "limit": limit})
}

type membersRequest struct {
	UserIDs []string `json:"user_ids"`
}

// UpdateMembers atomically replaces department membership. Each user is
// validated against the same tenant and organization before any update.
func (h *Handlers) UpdateMembers(w http.ResponseWriter, r *http.Request) {
	id := auth.IdentityFrom(r.Context())
	var req membersRequest
	if err := httpx.Decode(w, r, &req); err != nil {
		httpx.Error(w, r, http.StatusBadRequest, "INVALID_BODY", err.Error())
		return
	}
	tx, err := h.Pool.Begin(r.Context())
	if err != nil {
		httpx.Internal(w, r)
		return
	}
	defer tx.Rollback(r.Context())
	deptID := chi.URLParam(r, "id")
	var departmentOrgID string
	if err := tx.QueryRow(r.Context(), `SELECT organization_id FROM departments WHERE id=$1 AND tenant_id=$2 AND ($3 OR organization_id=$4)`, deptID, id.TenantID, id.TenantWide(), id.OrganizationID).Scan(&departmentOrgID); errors.Is(err, pgx.ErrNoRows) {
		httpx.Error(w, r, http.StatusNotFound, "DEPARTMENT_NOT_FOUND", "Department not found")
		return
	} else if err != nil {
		httpx.Internal(w, r)
		return
	}
	if len(req.UserIDs) > 0 {
		var count int
		if err := tx.QueryRow(r.Context(), `SELECT count(*) FROM users WHERE id = ANY($1::uuid[]) AND tenant_id=$2 AND organization_id=$3`, req.UserIDs, id.TenantID, departmentOrgID).Scan(&count); err != nil || count != len(req.UserIDs) {
			httpx.Error(w, r, http.StatusBadRequest, "INVALID_MEMBERS", "All members must belong to the same organization")
			return
		}
	}
	if _, err := tx.Exec(r.Context(), `UPDATE users SET department_id=NULL WHERE department_id=$1 AND tenant_id=$2 AND organization_id=$3`, deptID, id.TenantID, departmentOrgID); err != nil {
		httpx.Internal(w, r)
		return
	}
	if len(req.UserIDs) > 0 {
		if _, err := tx.Exec(r.Context(), `UPDATE users SET department_id=$1 WHERE id=ANY($2::uuid[]) AND tenant_id=$3 AND organization_id=$4`, deptID, req.UserIDs, id.TenantID, departmentOrgID); err != nil {
			httpx.Internal(w, r)
			return
		}
	}
	if err := tx.Commit(r.Context()); err != nil {
		httpx.Internal(w, r)
		return
	}
	h.Audit.Record(r.Context(), audit.Entry{TenantID: id.TenantID, ActorUserID: id.UserID, Action: "department.members_update", ResourceType: "department", ResourceID: deptID, Detail: map[string]any{"member_count": len(req.UserIDs)}})
	w.WriteHeader(http.StatusNoContent)
}
