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
	"github.com/yelnurq/email-server/internal/bulkmail"
	"github.com/yelnurq/email-server/internal/collab"
	"github.com/yelnurq/email-server/internal/config"
	"github.com/yelnurq/email-server/internal/departments"
	"github.com/yelnurq/email-server/internal/domains"
	"github.com/yelnurq/email-server/internal/emailapi"
	"github.com/yelnurq/email-server/internal/events"
	"github.com/yelnurq/email-server/internal/groups"
	"github.com/yelnurq/email-server/internal/logging"
	"github.com/yelnurq/email-server/internal/mailbox"
	"github.com/yelnurq/email-server/internal/mailcore"
	"github.com/yelnurq/email-server/internal/mailservice"
	"github.com/yelnurq/email-server/internal/messages"
	"github.com/yelnurq/email-server/internal/official"
	"github.com/yelnurq/email-server/internal/organization"
	"github.com/yelnurq/email-server/internal/security"
	"github.com/yelnurq/email-server/internal/server"
	"github.com/yelnurq/email-server/internal/smtpcreds"
	"github.com/yelnurq/email-server/internal/storage"
	"github.com/yelnurq/email-server/internal/tenant"
	"github.com/yelnurq/email-server/internal/trace"
	"github.com/yelnurq/email-server/internal/users"
	"github.com/yelnurq/email-server/internal/webhooks"
	"github.com/yelnurq/email-server/internal/webmail"
	"github.com/yelnurq/email-server/internal/worklist"

	"github.com/yelnurq/email-server/internal/httpx"
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
	authService := &auth.Service{Pool: pool, Log: log}
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

	// Mail core: product logic goes through the provider abstraction only.
	var mailCore mailcore.Provider = mailcore.Disabled{}
	if cfg.MailCoreProvider == "stalwart" {
		mailCore = &mailcore.Stalwart{
			BaseURL:   cfg.StalwartBaseURL,
			AdminUser: cfg.StalwartAdminUser,
			AdminPass: cfg.StalwartAdminPass,
		}
	}
	log.Info("mail core provider", slog.String("provider", mailCore.Name()))
	infra := &server.InfraMonitor{Health: health, Provider: mailCore, TTL: 10 * time.Second}

	// Unified mail store access (ADR-003): webmail reads/writes mailboxes
	// through JMAP with master-user auth. Without a mail core the JMAP base
	// URL is empty and every mailbox call fails as "store unavailable",
	// which webmail surfaces as a friendly status.
	mailSvc := &mailservice.Service{
		JMAP: &mailservice.JMAP{
			BaseURL:      cfg.StalwartBaseURL,
			MasterUser:   cfg.StalwartMasterUser,
			MasterSecret: cfg.StalwartMasterPass,
		},
		SubmitAddr:        cfg.StalwartSubmitAddr,
		SubmitInsecureTLS: cfg.StalwartInsecureTLS,
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
	webmailHandlers := &webmail.Handlers{Pool: pool, Mail: mailSvc, Accept: messageService, Log: log}
	attachmentHandlers := &attachments.Handlers{Pool: pool, Store: store, Log: log}

	orgHandlers := &organization.Handlers{Pool: pool, Audit: auditLog, Log: log}
	officialHandlers := &official.Handlers{Pool: pool, Audit: auditLog}
	worklistHandlers := &worklist.Handlers{Pool: pool, Audit: auditLog}
	go (&worklist.Processor{Pool: pool, NATS: nc, Log: log}).Run(ctx)
	bulkHandlers := &bulkmail.Handlers{Pool: pool, Audit: auditLog}
	go (&bulkmail.Processor{Pool: pool, Messages: messageService, Audit: auditLog, Log: log}).Run(ctx)
	provisioner := &mailcore.Provisioner{Pool: pool, Provider: mailCore, Audit: auditLog, Log: log}
	// Provisioning jobs run in-process here: the control plane owns the
	// desired state and the API is the only writer of these rows.
	go provisioner.RunJobs(ctx)
	domainHandlers := &domains.Handlers{Pool: pool, Audit: auditLog, Log: log, Provisioner: provisioner}
	departmentHandlers := &departments.Handlers{Pool: pool, Audit: auditLog, Log: log}
	collabHandlers := &collab.Handlers{Pool: pool, NATS: nc, Log: log}
	userHandlers := &users.Handlers{Pool: pool, Audit: auditLog, Log: log, Provisioner: provisioner}
	mailboxHandlers := &mailbox.Handlers{Pool: pool, Audit: auditLog, Log: log, Provider: mailCore, Provisioner: provisioner}
	aliasHandlers := &aliases.Handlers{Pool: pool, Audit: auditLog, Log: log}
	groupHandlers := &groups.Handlers{Pool: pool, Audit: auditLog, Log: log}
	apikeyHandlers := &apikeys.Handlers{Pool: pool, Audit: auditLog, Log: log}
	webhookHandlers := &webhooks.Handlers{Pool: pool, Audit: auditLog, Log: log}
	securityHandlers := &security.Handlers{Pool: pool, Audit: auditLog, Log: log, Mail: mailSvc, Store: store}
	smtpCredHandlers := &smtpcreds.Handlers{Pool: pool, Audit: auditLog, Log: log, Provider: mailCore}
	auditHandlers := &auditapi.Handlers{Pool: pool}
	traceHandlers := &trace.Handlers{Pool: pool, Log: log}
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
				admin.With(auth.RequirePermission("domains.manage")).
					Post("/domains/{id}/provision", domainHandlers.Provision)

				admin.With(auth.RequirePermission("users.manage")).
					Get("/users", userHandlers.List)
				admin.With(auth.RequirePermission("users.manage")).
					Post("/users", userHandlers.Create)
				admin.With(auth.RequirePermission("users.manage")).
					Patch("/users/{id}", userHandlers.Patch)

				admin.With(auth.RequirePermission("departments.read")).
					Get("/departments", departmentHandlers.List)
				admin.With(auth.RequirePermission("departments.read")).
					Get("/directory/users", departmentHandlers.Directory)

				admin.Group(func(departments chi.Router) {
					departments.Use(auth.RequirePermission("departments.manage"))
					departments.Post("/departments", departmentHandlers.Create)
					departments.Patch("/departments/{id}", departmentHandlers.Patch)
					departments.Delete("/departments/{id}", departmentHandlers.Delete)
					departments.Put("/departments/{id}/members", departmentHandlers.UpdateMembers)
				})

				admin.With(auth.RequirePermission("mailboxes.manage")).
					Get("/mailboxes", mailboxHandlers.List)
				admin.With(auth.RequirePermission("mailboxes.manage")).
					Post("/mailboxes", mailboxHandlers.Create)
				admin.With(auth.RequirePermission("mailboxes.manage")).
					Patch("/mailboxes/{id}", mailboxHandlers.Patch)
				admin.With(auth.RequirePermission("mailboxes.manage")).
					Post("/mailboxes/{id}/provision", mailboxHandlers.Provision)

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

				// Operational tools: Message Trace and infrastructure health.
				admin.Group(func(ops chi.Router) {
					ops.Use(auth.RequirePermission("security.manage"))
					ops.Get("/admin/messages", traceHandlers.List)
					ops.Get("/admin/messages/{publicID}/trace", traceHandlers.Get)
				})
				admin.With(auth.RequirePermission("users.manage")).
					Get("/system/infrastructure", infra.Infrastructure)

				admin.Group(func(chat chi.Router) {
					chat.Use(auth.RequirePermission("messages.send"))
					chat.Get("/chat/conversations", collabHandlers.List)
					chat.Post("/chat/conversations", collabHandlers.Create)
					chat.Get("/chat/conversations/{id}/messages", collabHandlers.Messages)
					chat.Post("/chat/conversations/{id}/messages", collabHandlers.Send)
					chat.Patch("/chat/messages/{messageID}", collabHandlers.Edit)
					chat.Delete("/chat/messages/{messageID}", collabHandlers.Delete)
					chat.Post("/chat/messages/{messageID}/reactions", collabHandlers.React)
					chat.Post("/chat/conversations/{id}/read", collabHandlers.MarkRead)
					chat.Post("/chat/conversations/{id}/typing", collabHandlers.Typing)
				})
				admin.Get("/realtime", collabHandlers.Realtime)
				admin.Get("/notifications", collabHandlers.Notifications)
				admin.Post("/notifications/read-all", collabHandlers.ReadAllNotifications)
				admin.Post("/notifications/{id}/read", collabHandlers.ReadNotification)
				admin.Group(func(official chi.Router) {
					official.Use(auth.RequirePermission("official.read"))
					official.Get("/official", officialHandlers.List)
					official.Post("/official", officialHandlers.Create)
					official.Post("/official/{id}/read", officialHandlers.Read)
					official.Post("/official/{id}/acknowledge", officialHandlers.Acknowledge)
					official.Get("/official/{id}/stats", officialHandlers.Stats)
				})
				admin.Group(func(tasks chi.Router) {
					tasks.Use(auth.RequirePermission("tasks.manage.self"))
					tasks.Get("/tasks", worklistHandlers.List)
					tasks.Post("/tasks", worklistHandlers.Create)
					tasks.Patch("/tasks/{id}", worklistHandlers.Patch)
					tasks.Delete("/tasks/{id}", worklistHandlers.Delete)
				})
				admin.With(auth.RequirePermission("bulk_email.view_analytics")).Get("/bulk-email", bulkHandlers.List)
				admin.With(auth.RequirePermission("bulk_email.create")).Post("/bulk-email", bulkHandlers.Create)
				admin.With(auth.RequirePermission("bulk_email.send")).Post("/bulk-email/{id}/send", bulkHandlers.Send)
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

				mail.Get("/mail/summary", webmailHandlers.Summary)
				mail.Get("/mail/messages", webmailHandlers.List)
				mail.Get("/mail/messages/{id}", webmailHandlers.Get)
				mail.Get("/mail/messages/{id}/events", webmailHandlers.Events)
				mail.Patch("/mail/messages/{id}", webmailHandlers.Patch)
				mail.Delete("/mail/messages/{id}", webmailHandlers.Delete)
				mail.Get("/mail/blob/{blobID}", webmailHandlers.Blob)

				// Mail client connection parameters (Settings → Mail clients).
				// Static configuration; passwords are never included.
				mail.Get("/mail/client-config", func(w http.ResponseWriter, r *http.Request) {
					httpx.JSON(w, http.StatusOK, map[string]any{
						"enabled": mailCore.Enabled(),
						"imap": map[string]string{
							"host": cfg.MailClientHost, "port": cfg.MailClientIMAPPort,
							"encryption": "SSL/TLS (self-signed in development)",
						},
						"smtp": map[string]string{
							"host": cfg.MailClientHost, "port": cfg.MailClientSMTPPort,
							"encryption": "STARTTLS (self-signed in development)",
						},
						"login_hint": "Sign in with your mailbox address and an SMTP credential password issued by your administrator.",
					})
				})

				mail.With(auth.RequirePermission("mail.send")).
					Post("/mail/send", webmailHandlers.Send)
				mail.With(auth.RequirePermission("mail.send")).
					Post("/mail/drafts", webmailHandlers.CreateDraft)
				mail.With(auth.RequirePermission("mail.send")).
					Put("/mail/drafts/{id}", webmailHandlers.UpdateDraft)
				mail.With(auth.RequirePermission("mail.send")).
					Post("/mail/drafts/{id}/send", webmailHandlers.SendDraft)

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
