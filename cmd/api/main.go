// Command api runs the Mail Platform HTTP API.
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nats-io/nats.go"
	"github.com/redis/go-redis/v9"

	"github.com/go-chi/chi/v5"

	"github.com/yelnurq/email-server/internal/audit"
	"github.com/yelnurq/email-server/internal/auth"
	"github.com/yelnurq/email-server/internal/config"
	"github.com/yelnurq/email-server/internal/domains"
	"github.com/yelnurq/email-server/internal/logging"
	"github.com/yelnurq/email-server/internal/mailbox"
	"github.com/yelnurq/email-server/internal/organization"
	"github.com/yelnurq/email-server/internal/server"
	"github.com/yelnurq/email-server/internal/tenant"
	"github.com/yelnurq/email-server/internal/users"
)

func main() {
	if err := run(); err != nil {
		slog.Error("api exited with error", slog.String("error", err.Error()))
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	log := logging.New("api", cfg.LogLevel, cfg.LogFormat)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// PostgreSQL: pool construction is lazy; readiness reports actual state.
	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()

	// Redis: lazy client, checked by readiness.
	redisOpts, err := redis.ParseURL(cfg.RedisURL)
	if err != nil {
		return err
	}
	rdb := redis.NewClient(redisOpts)
	defer rdb.Close()

	// NATS: keep retrying in the background so a temporarily missing broker
	// degrades readiness instead of crashing the API.
	nc, err := nats.Connect(cfg.NATSURL,
		nats.RetryOnFailedConnect(true),
		nats.MaxReconnects(-1),
		nats.ReconnectWait(2*time.Second),
	)
	if err != nil {
		return err
	}
	defer nc.Drain()

	// One-time bootstrap of the first tenant/org/admin on an empty database.
	if err := tenant.Bootstrap(ctx, pool, log, tenant.BootstrapConfig{
		AdminEmail:    cfg.BootstrapAdminEmail,
		AdminPassword: cfg.BootstrapAdminPassword,
	}); err != nil {
		return err
	}

	auditLog := &audit.Logger{Pool: pool, Log: log}
	authService := &auth.Service{Pool: pool}
	authHandlers := &auth.Handlers{
		Service:      authService,
		Limiter:      auth.NewLoginLimiter(rdb),
		Audit:        auditLog,
		Log:          log,
		CookieSecure: cfg.CookieSecure,
	}

	health := &server.HealthChecker{
		Pool:        pool,
		Redis:       rdb,
		NATS:        nc,
		S3HealthURL: strings.TrimRight(cfg.S3Endpoint, "/") + "/minio/health/live",
	}

	orgHandlers := &organization.Handlers{Pool: pool, Audit: auditLog, Log: log}
	domainHandlers := &domains.Handlers{Pool: pool, Audit: auditLog, Log: log}
	userHandlers := &users.Handlers{Pool: pool, Audit: auditLog, Log: log}
	mailboxHandlers := &mailbox.Handlers{Pool: pool, Audit: auditLog, Log: log}

	deps := server.Deps{
		Log:         log,
		Health:      health,
		CORSOrigins: cfg.CORSAllowedOrigins,
		AuthService: authService,
		Auth:        authHandlers,
		APIRoutes: func(v1 chi.Router) {
			v1.Group(func(admin chi.Router) {
				admin.Use(auth.RequireAuth)

				admin.With(auth.RequirePermission("organizations.manage")).
					Get("/organizations", orgHandlers.List)
				admin.With(auth.RequirePermission("organizations.manage")).
					Post("/organizations", orgHandlers.Create)

				admin.With(auth.RequirePermission("domains.manage")).
					Get("/domains", domainHandlers.List)
				admin.With(auth.RequirePermission("domains.manage")).
					Post("/domains", domainHandlers.Create)

				admin.With(auth.RequirePermission("users.manage")).
					Get("/users", userHandlers.List)
				admin.With(auth.RequirePermission("users.manage")).
					Post("/users", userHandlers.Create)

				admin.With(auth.RequirePermission("mailboxes.manage")).
					Get("/mailboxes", mailboxHandlers.List)
				admin.With(auth.RequirePermission("mailboxes.manage")).
					Post("/mailboxes", mailboxHandlers.Create)
			})
		},
	}

	srv := &http.Server{
		Addr:              cfg.APIAddr,
		Handler:           server.New(deps),
		ReadHeaderTimeout: 10 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		log.Info("api listening", slog.String("addr", cfg.APIAddr))
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
	}

	log.Info("shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return srv.Shutdown(shutdownCtx)
}
