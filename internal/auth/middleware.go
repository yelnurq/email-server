package auth

import (
	"context"
	"net/http"
	"strings"

	"github.com/yelnurq/email-server/internal/httpx"
)

type contextKey struct{}

// IdentityFrom returns the authenticated identity from the request context,
// or nil when the request is unauthenticated.
func IdentityFrom(ctx context.Context) *Identity {
	id, _ := ctx.Value(contextKey{}).(*Identity)
	return id
}

// TokenFromRequest extracts the session token from the Authorization header
// (Bearer) or the session cookie.
func TokenFromRequest(r *http.Request) string {
	if h := r.Header.Get("Authorization"); strings.HasPrefix(h, "Bearer ") {
		return strings.TrimPrefix(h, "Bearer ")
	}
	if c, err := r.Cookie(CookieName); err == nil {
		return c.Value
	}
	return ""
}

// Middleware resolves the session (when present) and stores the identity in
// the request context. It does not reject unauthenticated requests.
func (s *Service) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := TokenFromRequest(r)
		if token != "" {
			if id, err := s.Resolve(r.Context(), token); err == nil {
				r = r.WithContext(context.WithValue(r.Context(), contextKey{}, id))
			}
		}
		next.ServeHTTP(w, r)
	})
}

// RequireAuth rejects unauthenticated requests with 401.
func RequireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if IdentityFrom(r.Context()) == nil {
			httpx.Error(w, r, http.StatusUnauthorized, "UNAUTHENTICATED", "Authentication required")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// RequirePermission rejects requests whose identity lacks the permission.
func RequirePermission(perm string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			id := IdentityFrom(r.Context())
			if id == nil {
				httpx.Error(w, r, http.StatusUnauthorized, "UNAUTHENTICATED", "Authentication required")
				return
			}
			if !id.HasPermission(perm) {
				httpx.Error(w, r, http.StatusForbidden, "FORBIDDEN", "Insufficient permissions")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
