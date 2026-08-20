// Package server assembles the HTTP API: router, middleware and routes.
package server

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
)

// New builds the root HTTP handler. Versioned API routes will mount under
// /api/v1 in later milestones; health endpoints live at the root.
func New(log *slog.Logger, health *HealthChecker, corsOrigins []string) http.Handler {
	r := chi.NewRouter()

	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(requestLogger(log))
	r.Use(middleware.Recoverer)

	if len(corsOrigins) > 0 {
		r.Use(cors.Handler(cors.Options{
			AllowedOrigins:   corsOrigins,
			AllowedMethods:   []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
			AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-Request-ID"},
			AllowCredentials: true,
			MaxAge:           300,
		}))
	}

	r.Get("/health/live", health.Live)
	r.Get("/health/ready", health.Ready)

	r.Route("/api/v1", func(r chi.Router) {
		// Milestone 2+: /auth, /me, /organizations, /domains, /users,
		// /mailboxes, /folders, /messages, /attachments.
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
