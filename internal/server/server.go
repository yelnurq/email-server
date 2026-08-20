// Package server assembles the HTTP API: router, middleware and routes.
package server

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"

	"github.com/yelnurq/email-server/internal/auth"
)

// Deps carries everything the router needs. Handler sets are optional in
// tests; nil groups are simply not mounted.
type Deps struct {
	Log         *slog.Logger
	Health      *HealthChecker
	CORSOrigins []string

	AuthService *auth.Service
	Auth        *auth.Handlers
	APIRoutes   func(chi.Router) // v1 routes mounted by feature modules
}

// New builds the root HTTP handler.
func New(d Deps) http.Handler {
	r := chi.NewRouter()

	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(requestLogger(d.Log))
	r.Use(middleware.Recoverer)

	if len(d.CORSOrigins) > 0 {
		r.Use(cors.Handler(cors.Options{
			AllowedOrigins:   d.CORSOrigins,
			AllowedMethods:   []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
			AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-Request-ID", "Idempotency-Key"},
			AllowCredentials: true,
			MaxAge:           300,
		}))
	}

	r.Get("/health/live", d.Health.Live)
	r.Get("/health/ready", d.Health.Ready)

	r.Route("/api/v1", func(v1 chi.Router) {
		if d.AuthService != nil {
			v1.Use(d.AuthService.Middleware)
		}
		if d.Auth != nil {
			v1.Post("/auth/login", d.Auth.Login)
			v1.Post("/auth/logout", d.Auth.Logout)
			v1.With(auth.RequireAuth).Get("/me", d.Auth.Me)
		}
		if d.APIRoutes != nil {
			d.APIRoutes(v1)
		}
	})

	return r
}

// requestLogger emits one structured log line per request. It logs metadata
// only — never request bodies (mail content must not reach general logs).
func requestLogger(log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
			next.ServeHTTP(ww, r)
			log.Info("http_request",
				slog.String("request_id", middleware.GetReqID(r.Context())),
				slog.String("method", r.Method),
				slog.String("path", r.URL.Path),
				slog.Int("status", ww.Status()),
				slog.Int("bytes", ww.BytesWritten()),
				slog.Duration("duration", time.Since(start)),
			)
		})
	}
}
