package auth

import (
	"errors"
	"log/slog"
	"net"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/yelnurq/email-server/internal/audit"
	"github.com/yelnurq/email-server/internal/httpx"
)

// Handlers exposes /api/v1/auth/* and /api/v1/me.
type Handlers struct {
	Service      *Service
	Limiter      *LoginLimiter
	Audit        *audit.Logger
	Log          *slog.Logger
	CookieSecure bool
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type userResponse struct {
	ID             string      `json:"id"`
	Email          string      `json:"email"`
	DisplayName    string      `json:"display_name"`
	TenantID       string      `json:"tenant_id"`
	OrganizationID string      `json:"organization_id,omitempty"`
	Roles          []RoleGrant `json:"roles"`
	Permissions    []string    `json:"permissions"`
}

func identityResponse(id *Identity) userResponse {
	perms := make([]string, 0, len(id.Permissions))
	for p := range id.Permissions {
		perms = append(perms, p)
	}
	roles := id.Roles
	if roles == nil {
		roles = []RoleGrant{}
	}
	return userResponse{
		ID:             id.UserID,
		Email:          id.Email,
		DisplayName:    id.DisplayName,
		TenantID:       id.TenantID,
		OrganizationID: id.OrganizationID,
		Roles:          roles,
		Permissions:    perms,
	}
}

func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// Login verifies credentials and issues a session (cookie + bearer token).
func (h *Handlers) Login(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := httpx.Decode(w, r, &req); err != nil {
		httpx.Error(w, r, http.StatusBadRequest, "INVALID_BODY", err.Error())
		return
	}
	req.Email = strings.TrimSpace(strings.ToLower(req.Email))
	if req.Email == "" || req.Password == "" {
		httpx.Error(w, r, http.StatusBadRequest, "INVALID_CREDENTIALS_FORMAT", "Email and password are required")
		return
	}

	ip := clientIP(r)
	if !h.Limiter.Allow(r.Context(), "ip:"+ip) || !h.Limiter.Allow(r.Context(), "email:"+req.Email) {
		httpx.Error(w, r, http.StatusTooManyRequests, "RATE_LIMITED", "Too many login attempts, try again later")
		return
	}

	var userID, tenantID, status, passwordHash string
	err := h.Service.Pool.QueryRow(r.Context(), `
		SELECT u.id, u.tenant_id, u.status, c.password_hash
		FROM users u
		JOIN user_credentials c ON c.user_id = u.id
		WHERE u.email = $1`, req.Email).
		Scan(&userID, &tenantID, &status, &passwordHash)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		// Equalize timing between unknown-user and wrong-password paths.
		_ = VerifyPassword(req.Password, dummyHash)
		httpx.Error(w, r, http.StatusUnauthorized, "INVALID_CREDENTIALS", "Invalid email or password")
		return
	case err != nil:
		h.Log.Error("login query failed", slog.String("error", err.Error()))
		httpx.Internal(w, r)
		return
	}

	if err := VerifyPassword(req.Password, passwordHash); err != nil {
		h.Audit.Record(r.Context(), audit.Entry{
			TenantID: tenantID, Action: "auth.login_failed",
			ResourceType: "user", ResourceID: userID, IP: ip,
		})
		httpx.Error(w, r, http.StatusUnauthorized, "INVALID_CREDENTIALS", "Invalid email or password")
		return
	}
	if status != "active" {
		httpx.Error(w, r, http.StatusForbidden, "USER_DISABLED", "This account is disabled")
		return
	}

	token, err := h.Service.CreateSession(r.Context(), userID, ip, r.UserAgent())
	if err != nil {
		h.Log.Error("session create failed", slog.String("error", err.Error()))
		httpx.Internal(w, r)
		return
	}
	id, err := h.Service.Resolve(r.Context(), token)
	if err != nil {
		h.Log.Error("session resolve failed", slog.String("error", err.Error()))
		httpx.Internal(w, r)
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     CookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   h.CookieSecure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(SessionTTL.Seconds()),
	})
	h.Audit.Record(r.Context(), audit.Entry{
		TenantID: tenantID, ActorUserID: userID, Action: "auth.login",
		ResourceType: "user", ResourceID: userID, IP: ip,
	})
	httpx.JSON(w, http.StatusOK, map[string]any{
		"token": token,
		"user":  identityResponse(id),
	})
}

// dummyHash is verified on unknown-user logins so response timing does not
// reveal whether an email exists. Generated once with HashPassword.
const dummyHash = "$argon2id$v=19$m=65536,t=3,p=4$AAAAAAAAAAAAAAAAAAAAAA$L9tOEsZoWvU5DUZOtSJ9EU8Wgxo7mYXqjjjyBnnUAAc"

// Logout revokes the current session and clears the cookie.
func (h *Handlers) Logout(w http.ResponseWriter, r *http.Request) {
	id := IdentityFrom(r.Context())
	if id != nil {
		if err := h.Service.RevokeSession(r.Context(), id.SessionID); err != nil {
			h.Log.Error("session revoke failed", slog.String("error", err.Error()))
			httpx.Internal(w, r)
			return
		}
		h.Audit.Record(r.Context(), audit.Entry{
			TenantID: id.TenantID, ActorUserID: id.UserID, Action: "auth.logout",
			ResourceType: "session", ResourceID: id.SessionID, IP: clientIP(r),
		})
	}
	http.SetCookie(w, &http.Cookie{
		Name: CookieName, Value: "", Path: "/", HttpOnly: true,
		Secure: h.CookieSecure, SameSite: http.SameSiteLaxMode, MaxAge: -1,
	})
	httpx.JSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// Me returns the current authenticated identity.
func (h *Handlers) Me(w http.ResponseWriter, r *http.Request) {
	id := IdentityFrom(r.Context())
	httpx.JSON(w, http.StatusOK, identityResponse(id))
}
