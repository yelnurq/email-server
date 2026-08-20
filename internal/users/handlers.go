// Package users implements admin user management: creating users with
// credentials, role grants and optional mailbox provisioning in one
// transaction.
package users

import (
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/yelnurq/email-server/internal/audit"
	"github.com/yelnurq/email-server/internal/auth"
	"github.com/yelnurq/email-server/internal/httpx"
	"github.com/yelnurq/email-server/internal/mailaddr"
	"github.com/yelnurq/email-server/internal/mailbox"
)

type Handlers struct {
	Pool  *pgxpool.Pool
	Audit *audit.Logger
	Log   *slog.Logger
}

type User struct {
	ID             string `json:"id"`
	OrganizationID string `json:"organization_id,omitempty"`
	Email          string `json:"email"`
	DisplayName    string `json:"display_name"`
	Status         string `json:"status"`
	MailboxAddress string `json:"mailbox_address,omitempty"`
	CreatedAt      string `json:"created_at"`
}

// List returns the tenant's users (org-scoped admins: their org only).
func (h *Handlers) List(w http.ResponseWriter, r *http.Request) {
	id := auth.IdentityFrom(r.Context())
	query := `
		SELECT u.id, COALESCE(u.organization_id::text, ''), u.email::text, u.display_name,
		       u.status, COALESCE(m.address::text, ''), u.created_at::text
		FROM users u
		LEFT JOIN mailboxes m ON m.user_id = u.id
		WHERE u.tenant_id = $1`
	args := []any{id.TenantID}
	if !id.TenantWide() {
		query += ` AND u.organization_id = $2`
		args = append(args, id.OrganizationID)
	}
	query += ` ORDER BY u.created_at`
	rows, err := h.Pool.Query(r.Context(), query, args...)
	if err != nil {
		h.Log.Error("user list failed", slog.String("error", err.Error()))
		httpx.Internal(w, r)
		return
	}
	defer rows.Close()
	out := []User{}
	for rows.Next() {
		var u User
		if err := rows.Scan(&u.ID, &u.OrganizationID, &u.Email, &u.DisplayName, &u.Status, &u.MailboxAddress, &u.CreatedAt); err != nil {
			httpx.Internal(w, r)
			return
		}
		out = append(out, u)
	}
	httpx.JSON(w, http.StatusOK, map[string]any{"users": out})
}

type createRequest struct {
	OrganizationID string `json:"organization_id"`
	Email          string `json:"email"`
	DisplayName    string `json:"display_name"`
	Password       string `json:"password"`
	Role           string `json:"role"`
	// Optional mailbox provisioning on a verified domain.
	MailboxDomainID  string `json:"mailbox_domain_id"`
	MailboxLocalPart string `json:"mailbox_local_part"`
}

var allowedRoles = map[string]bool{
	"member": true, "org_admin": true, "domain_admin": true,
	"developer": true, "security_analyst": true, "auditor": true,
}

// Create adds a user (credentials + role) and optionally provisions their
// mailbox atomically.
func (h *Handlers) Create(w http.ResponseWriter, r *http.Request) {
	id := auth.IdentityFrom(r.Context())
	var req createRequest
	if err := httpx.Decode(w, r, &req); err != nil {
		httpx.Error(w, r, http.StatusBadRequest, "INVALID_BODY", err.Error())
		return
	}

	email := strings.ToLower(strings.TrimSpace(req.Email))
	if _, _, err := mailaddr.NormalizeAddress(email); err != nil {
		httpx.Error(w, r, http.StatusBadRequest, "INVALID_EMAIL", "Invalid email address")
		return
	}
	if len(req.Password) < 10 {
		httpx.Error(w, r, http.StatusBadRequest, "WEAK_PASSWORD", "Password must be at least 10 characters")
		return
	}
	role := req.Role
	if role == "" {
		role = "member"
	}
	if !allowedRoles[role] {
		httpx.Error(w, r, http.StatusBadRequest, "INVALID_ROLE", "Unknown or non-assignable role")
		return
	}
	orgID := req.OrganizationID
	if orgID == "" {
		orgID = id.OrganizationID
	}
	if orgID == "" {
		httpx.Error(w, r, http.StatusBadRequest, "MISSING_ORGANIZATION", "organization_id is required")
		return
	}
	if !id.TenantWide() && orgID != id.OrganizationID {
		httpx.Error(w, r, http.StatusForbidden, "FORBIDDEN", "Cannot manage another organization")
		return
	}

	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		httpx.Internal(w, r)
		return
	}

	tx, err := h.Pool.Begin(r.Context())
	if err != nil {
		httpx.Internal(w, r)
		return
	}
	defer tx.Rollback(r.Context())

	var orgOK bool
	if err := tx.QueryRow(r.Context(),
		`SELECT EXISTS (SELECT 1 FROM organizations WHERE id = $1 AND tenant_id = $2)`,
		orgID, id.TenantID).Scan(&orgOK); err != nil || !orgOK {
		httpx.Error(w, r, http.StatusNotFound, "ORGANIZATION_NOT_FOUND", "Organization not found")
		return
	}

	var userID string
	err = tx.QueryRow(r.Context(), `
		INSERT INTO users (tenant_id, organization_id, email, display_name)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (email) DO NOTHING
		RETURNING id`,
		id.TenantID, orgID, email, strings.TrimSpace(req.DisplayName)).Scan(&userID)
	if errors.Is(err, pgx.ErrNoRows) {
		httpx.Error(w, r, http.StatusConflict, "EMAIL_TAKEN", "A user with this email already exists")
		return
	}
	if err != nil {
		httpx.Internal(w, r)
		return
	}
	if _, err := tx.Exec(r.Context(),
		`INSERT INTO user_credentials (user_id, password_hash) VALUES ($1, $2)`, userID, hash); err != nil {
		httpx.Internal(w, r)
		return
	}
	if _, err := tx.Exec(r.Context(), `
		INSERT INTO user_roles (user_id, role_code, scope_type, scope_id)
		VALUES ($1, $2, 'organization', $3)`, userID, role, orgID); err != nil {
		httpx.Internal(w, r)
		return
	}

	mailboxAddress := ""
	if req.MailboxDomainID != "" {
		var domainName, domainOrgID, domainStatus string
		err = tx.QueryRow(r.Context(), `
			SELECT name, organization_id, status FROM domains WHERE id = $1 AND tenant_id = $2`,
			req.MailboxDomainID, id.TenantID).Scan(&domainName, &domainOrgID, &domainStatus)
		if errors.Is(err, pgx.ErrNoRows) {
			httpx.Error(w, r, http.StatusNotFound, "DOMAIN_NOT_FOUND", "Mailbox domain not found")
			return
		}
		if err != nil {
			httpx.Internal(w, r)
			return
		}
		if domainStatus != "verified" {
			httpx.Error(w, r, http.StatusConflict, "DOMAIN_NOT_VERIFIED", "Mailbox domain is not verified")
			return
		}
		local := req.MailboxLocalPart
		if local == "" {
			// default to the local part of the login email
			local = email[:strings.LastIndexByte(email, '@')]
		}
		if _, err := mailbox.Provision(r.Context(), tx, id.TenantID, orgID, req.MailboxDomainID, domainName, userID, local); err != nil {
			if errors.Is(err, mailbox.ErrAddressTaken) {
				httpx.Error(w, r, http.StatusConflict, "ADDRESS_TAKEN", "Mailbox address is already in use")
				return
			}
			httpx.Error(w, r, http.StatusBadRequest, "INVALID_LOCAL_PART", "Invalid mailbox local part")
			return
		}
		mailboxAddress = mailaddr.Join(local, domainName)
	}

	if err := tx.Commit(r.Context()); err != nil {
		httpx.Internal(w, r)
		return
	}
	h.Audit.Record(r.Context(), audit.Entry{
		TenantID: id.TenantID, ActorUserID: id.UserID, Action: "user.create",
		ResourceType: "user", ResourceID: userID,
		Detail: map[string]any{"email": email, "role": role, "mailbox": mailboxAddress},
	})
	httpx.JSON(w, http.StatusCreated, map[string]string{
		"id":              userID,
		"email":           email,
		"mailbox_address": mailboxAddress,
	})
}
