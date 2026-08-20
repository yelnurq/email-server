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

	"github.com/yelnurq/email-server/internal/aliases"
	"github.com/yelnurq/email-server/internal/apikeys"
	"github.com/yelnurq/email-server/internal/attachments"
	"github.com/yelnurq/email-server/internal/audit"
	"github.com/yelnurq/email-server/internal/auditapi"
	"github.com/yelnurq/email-server/internal/auth"
	"github.com/yelnurq/email-server/internal/config"
	"github.com/yelnurq/email-server/internal/domains"
	"github.com/yelnurq/email-server/internal/emailapi"
	"github.com/yelnurq/email-server/internal/events"
	"github.com/yelnurq/email-server/internal/groups"
	"github.com/yelnurq/email-server/internal/logging"
	"github.com/yelnurq/email-server/internal/mailbox"
	"github.com/yelnurq/email-server/internal/messages"
	"github.com/yelnurq/email-server/internal/organization"
	"github.com/yelnurq/email-server/internal/security"
	"github.com/yelnurq/email-server/internal/server"
	"github.com/yelnurq/email-server/internal/smtpcreds"
	"github.com/yelnurq/email-server/internal/storage"
	"github.com/yelnurq/email-server/internal/tenant"
	"github.com/yelnurq/email-server/internal/users"
	"github.com/yelnurq/email-server/internal/webhooks"
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

	// Mail-plane topology and the transactional outbox publisher.
	if err := events.EnsureStream(nc); err != nil {
		log.Warn("could not ensure NATS stream at startup; publisher will retry",
			slog.String("error", err.Error()))
	}
	publisher := &events.Publisher{Pool: pool, NATS: nc, Log: log}
	go publisher.Run(ctx)

	store, err := storage.NewS3Store(ctx, storage.Config{
		Endpoint:  cfg.S3Endpoint,
		AccessKey: cfg.S3AccessKey,
		SecretKey: cfg.S3SecretKey,
		Region:    cfg.S3Region,
		Bucket:    cfg.S3BucketAttachments,
	})
	if err != nil {
		return err
	}

	messageService := &messages.Service{Pool: pool}
	webmail := &messages.WebmailHandlers{Svc: messageService, Log: log}
	attachmentHandlers := &attachments.Handlers{Pool: pool, Store: store, Log: log}

	orgHandlers := &organization.Handlers{Pool: pool, Audit: auditLog, Log: log}
	domainHandlers := &domains.Handlers{Pool: pool, Audit: auditLog, Log: log}
	userHandlers := &users.Handlers{Pool: pool, Audit: auditLog, Log: log}
	mailboxHandlers := &mailbox.Handlers{Pool: pool, Audit: auditLog, Log: log}
	aliasHandlers := &aliases.Handlers{Pool: pool, Audit: auditLog, Log: log}
	groupHandlers := &groups.Handlers{Pool: pool, Audit: auditLog, Log: log}
	apikeyHandlers := &apikeys.Handlers{Pool: pool, Audit: auditLog, Log: log}
	webhookHandlers := &webhooks.Handlers{Pool: pool, Audit: auditLog, Log: log}
	securityHandlers := &security.Handlers{Pool: pool, Audit: auditLog, Log: log}
	smtpCredHandlers := &smtpcreds.Handlers{Pool: pool, Audit: auditLog, Log: log}
	auditHandlers := &auditapi.Handlers{Pool: pool}
	emailAPI := &emailapi.Handlers{
		Pool: pool,
		Keys: &apikeys.Service{Pool: pool},
		Svc:  messageService,
		Log:  log,
	}

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

				admin.Group(func(mbadmin chi.Router) {
					mbadmin.Use(auth.RequirePermission("mailboxes.manage"))
					mbadmin.Get("/aliases", aliasHandlers.List)
					mbadmin.Post("/aliases", aliasHandlers.Create)
					mbadmin.Patch("/aliases/{id}", aliasHandlers.Patch)
					mbadmin.Delete("/aliases/{id}", aliasHandlers.Delete)

					mbadmin.Get("/groups", groupHandlers.List)
					mbadmin.Post("/groups", groupHandlers.Create)
					mbadmin.Post("/groups/{id}/members", groupHandlers.UpdateMembers)
					mbadmin.Delete("/groups/{id}", groupHandlers.Delete)
				})

				admin.Group(func(keys chi.Router) {
					keys.Use(auth.RequirePermission("apikeys.manage"))
					keys.Get("/api-keys", apikeyHandlers.List)
					keys.Post("/api-keys", apikeyHandlers.Create)
					keys.Delete("/api-keys/{id}", apikeyHandlers.Revoke)

					keys.Get("/smtp-credentials", smtpCredHandlers.List)
					keys.Post("/smtp-credentials", smtpCredHandlers.Create)
					keys.Delete("/smtp-credentials/{id}", smtpCredHandlers.Revoke)
				})

				admin.Group(func(hooks chi.Router) {
					hooks.Use(auth.RequirePermission("webhooks.manage"))
					hooks.Get("/webhooks", webhookHandlers.List)
					hooks.Post("/webhooks", webhookHandlers.Create)
					hooks.Patch("/webhooks/{id}", webhookHandlers.Patch)
					hooks.Delete("/webhooks/{id}", webhookHandlers.Delete)
					hooks.Get("/webhooks/{id}/deliveries", webhookHandlers.Deliveries)
					hooks.Post("/webhooks/{id}/deliveries/{deliveryID}/retry", webhookHandlers.Retry)
				})

				admin.Group(func(sec chi.Router) {
					sec.Use(auth.RequirePermission("security.manage"))
					sec.Get("/quarantine", securityHandlers.Quarantine)
					sec.Post("/quarantine/{id}/release", securityHandlers.Release)
					sec.Post("/quarantine/{id}/delete", securityHandlers.DeleteItem)
					sec.Get("/security/blocks", securityHandlers.Blocks)
					sec.Post("/security/blocks", securityHandlers.AddBlock)
					sec.Delete("/security/blocks/{id}", securityHandlers.RemoveBlock)
				})

				admin.With(auth.RequirePermission("audit.read")).
					Get("/audit", auditHandlers.List)
			})

			// Developer Email API: authenticated by API key, not session.
			v1.Group(func(emails chi.Router) {
				emails.Use(emailAPI.RequireAPIKey)
				emails.With(emailapi.RequireScope("emails.send")).
					Post("/emails", emailAPI.Send)
				emails.With(emailapi.RequireScope("emails.send")).
					Post("/emails/batch", emailAPI.SendBatch)
				emails.With(emailapi.RequireScope("emails.read")).
					Get("/emails/{id}", emailAPI.Get)
				emails.With(emailapi.RequireScope("emails.read")).
					Get("/emails/{id}/events", emailAPI.Events)
			})

			v1.Group(func(mail chi.Router) {
				mail.Use(auth.RequireAuth)
				mail.Use(auth.RequirePermission("mail.read"))

				mail.Get("/mail/summary", webmail.Summary)
				mail.Get("/mail/messages", webmail.List)
				mail.Get("/mail/messages/{id}", webmail.Get)
				mail.Patch("/mail/messages/{id}", webmail.Patch)
				mail.Delete("/mail/messages/{id}", webmail.Delete)

				mail.With(auth.RequirePermission("mail.send")).
					Post("/mail/send", webmail.Send)
				mail.With(auth.RequirePermission("mail.send")).
					Post("/mail/drafts", webmail.CreateDraft)
				mail.With(auth.RequirePermission("mail.send")).
					Put("/mail/drafts/{id}", webmail.UpdateDraft)
				mail.With(auth.RequirePermission("mail.send")).
					Post("/mail/drafts/{id}/send", webmail.SendDraft)

				mail.With(auth.RequirePermission("mail.send")).
					Post("/mail/attachments", attachmentHandlers.Upload)
				mail.Get("/mail/attachments/{id}", attachmentHandlers.Download)
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
