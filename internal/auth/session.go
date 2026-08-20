package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// SessionTTL is how long a session stays valid. Configurable later if needed.
const SessionTTL = 7 * 24 * time.Hour

// CookieName carries the session token for browser clients.
const CookieName = "mp_session"

// Identity is the authenticated principal attached to a request context.
type Identity struct {
	UserID         string
	TenantID       string
	OrganizationID string // empty when the user is tenant-scoped only
	Email          string
	DisplayName    string
	SessionID      string
	Permissions    map[string]bool
	Roles          []RoleGrant
}

// RoleGrant is one role assignment with its scope.
type RoleGrant struct {
	Role      string `json:"role"`
	ScopeType string `json:"scope_type"`
	ScopeID   string `json:"scope_id,omitempty"`
}

// TenantWide reports whether the identity holds any platform- or
// tenant-scoped role, i.e. may administer every organization in its tenant.
// Organization-scoped admins are limited to their own organization.
func (id *Identity) TenantWide() bool {
	for _, g := range id.Roles {
		if g.ScopeType == "platform" || g.ScopeType == "tenant" {
			return true
		}
	}
	return false
}

// HasPermission reports whether the identity holds a permission at any scope.
// Scope-narrowing (tenant/organization ownership of the target resource) is
// enforced by handlers via tenant-scoped queries.
func (id *Identity) HasPermission(perm string) bool {
	return id.Permissions[perm]
}

// Service owns session persistence.
type Service struct {
	Pool *pgxpool.Pool
	Log  *slog.Logger
}

func hashToken(token string) []byte {
	sum := sha256.Sum256([]byte(token))
	return sum[:]
}

// CreateSession issues a new opaque token for the user and stores its hash.
func (s *Service) CreateSession(ctx context.Context, userID, ip, userAgent string) (token string, err error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	token = base64.RawURLEncoding.EncodeToString(raw)
	expiresAt := time.Now().UTC().Add(SessionTTL)
	_, err = s.Pool.Exec(ctx, `
		INSERT INTO sessions (user_id, token_hash, expires_at, ip, user_agent)
		VALUES ($1, $2, $3, $4, $5)`,
		userID, hashToken(token), expiresAt, ip, userAgent)
	if err != nil {
		return "", err
	}
	return token, nil
}

// ErrNoSession means the token is unknown, expired, revoked, or the user is
// not active.
var ErrNoSession = errors.New("no valid session")

// Resolve validates a raw token and loads the full identity with permissions.
func (s *Service) Resolve(ctx context.Context, token string) (*Identity, error) {
	if token == "" {
		return nil, ErrNoSession
	}
	id := &Identity{Permissions: map[string]bool{}}
	var orgID *string
	err := s.Pool.QueryRow(ctx, `
		SELECT s.id, u.id, u.tenant_id, u.organization_id, u.email, u.display_name
		FROM sessions s
		JOIN users u ON u.id = s.user_id
		WHERE s.token_hash = $1
		  AND s.revoked_at IS NULL
		  AND s.expires_at > now()
		  AND u.status = 'active'`,
		hashToken(token)).
		Scan(&id.SessionID, &id.UserID, &id.TenantID, &orgID, &id.Email, &id.DisplayName)
	if errors.Is(err, pgx.ErrNoRows) {
		if s.Log != nil {
			s.Log.Warn("session not found", slog.String("token_hash", base64.RawURLEncoding.EncodeToString(hashToken(token)[:8])))
		}
		return nil, ErrNoSession
	}
	if err != nil {
		if s.Log != nil {
			s.Log.Error("session query failed", slog.String("error", err.Error()))
		}
		return nil, err
	}
	if orgID != nil {
		id.OrganizationID = *orgID
	}

	rows, err := s.Pool.Query(ctx, `
		SELECT ur.role_code, ur.scope_type, ur.scope_id, rp.permission_code
		FROM user_roles ur
		JOIN role_permissions rp ON rp.role_code = ur.role_code
		WHERE ur.user_id = $1`, id.UserID)
	if err != nil {
		if s.Log != nil {
			s.Log.Error("roles query failed", slog.String("userID", id.UserID), slog.String("error", err.Error()))
		}
		return nil, err
	}
	defer rows.Close()
	seenRole := map[string]bool{}
	for rows.Next() {
		var role, scopeType, perm string
		var scopeID *string
		if err := rows.Scan(&role, &scopeType, &scopeID, &perm); err != nil {
			if s.Log != nil {
				s.Log.Error("role scan failed", slog.String("userID", id.UserID), slog.String("error", err.Error()))
			}
			return nil, err
		}
		id.Permissions[perm] = true
		key := role + "|" + scopeType
		if scopeID != nil {
			key += "|" + *scopeID
		}
		if !seenRole[key] {
			seenRole[key] = true
			grant := RoleGrant{Role: role, ScopeType: scopeType}
			if scopeID != nil {
				grant.ScopeID = *scopeID
			}
			id.Roles = append(id.Roles, grant)
		}
	}
	if err := rows.Err(); err != nil {
		if s.Log != nil {
			s.Log.Error("roles iteration failed", slog.String("userID", id.UserID), slog.String("error", err.Error()))
		}
		return nil, err
	}
	return id, nil
}

// RevokeSession invalidates one session by its id.
func (s *Service) RevokeSession(ctx context.Context, sessionID string) error {
	_, err := s.Pool.Exec(ctx,
		`UPDATE sessions SET revoked_at = now() WHERE id = $1 AND revoked_at IS NULL`, sessionID)
	return err
}
