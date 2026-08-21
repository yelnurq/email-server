// Command worker runs the Mail Platform background processor: the local
// delivery consumer (email.accepted → routing → mailbox delivery).
package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nats-io/nats.go"

	"github.com/yelnurq/email-server/internal/config"
	"github.com/yelnurq/email-server/internal/delivery"
	"github.com/yelnurq/email-server/internal/logging"
	"github.com/yelnurq/email-server/internal/mailservice"
	"github.com/yelnurq/email-server/internal/scanner"
	"github.com/yelnurq/email-server/internal/security"
	"github.com/yelnurq/email-server/internal/storage"
	"github.com/yelnurq/email-server/internal/webhooks"
)

func main() {
	if err := run(); err != nil {
		slog.Error("worker exited with error", slog.String("error", err.Error()))
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	log := logging.New("worker", cfg.LogLevel, cfg.LogFormat)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()

	nc, err := nats.Connect(cfg.NATSURL,
		nats.RetryOnFailedConnect(true),
		nats.MaxReconnects(-1),
		nats.ReconnectWait(2*time.Second),
	)
	if err != nil {
		return err
	}
	defer nc.Drain()

	// Heartbeat: the control plane reports worker liveness from this row.
	go heartbeat(ctx, pool, log)

	dispatcher := &webhooks.Dispatcher{Pool: pool, NATS: nc, Log: log}
	dispatchErr := make(chan error, 1)
	go func() {
		if err := dispatcher.Run(ctx); err != nil && ctx.Err() == nil {
			dispatchErr <- err
		}
	}()

	// Mail store access: the worker delivers into Stalwart mailboxes and
	// relays remote recipients through its submission queue (ADR-003).
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
	mailSvc := &mailservice.Service{
		JMAP: &mailservice.JMAP{
			BaseURL:      cfg.StalwartBaseURL,
			MasterUser:   cfg.StalwartMasterUser,
			MasterSecret: cfg.StalwartMasterPass,
		},
		SubmitAddr:        cfg.StalwartSubmitAddr,
		SubmitInsecureTLS: cfg.StalwartInsecureTLS,
	}

	w := &delivery.Worker{
		Pool: pool, NATS: nc, Log: log, Mail: mailSvc, Store: store,
		Scan: &security.Engine{
			Rspamd: &scanner.Rspamd{BaseURL: cfg.RspamdURL, Password: cfg.RspamdPassword},
			Clamd:  &scanner.Clamd{Addr: cfg.ClamAVAddr},
			Log:    log,
		},
	}
	log.Info("worker started")
	workerErr := make(chan error, 1)
	go func() {
		if err := w.Run(ctx); err != nil && ctx.Err() == nil {
			workerErr <- err
		}
	}()

	select {
	case err := <-dispatchErr:
		return err
	case err := <-workerErr:
		return err
	case <-ctx.Done():
	}
	log.Info("worker shutting down")
	return nil
}

// heartbeat upserts the worker liveness row every 10 seconds. started_at is
// reset on the first beat of each process so restarts are visible.
func heartbeat(ctx context.Context, pool *pgxpool.Pool, log *slog.Logger) {
	hostname, _ := os.Hostname()
	first := true
	beat := func() {
		var err error
		if first {
			_, err = pool.Exec(ctx, `
				INSERT INTO worker_heartbeats (name, hostname, beat_at, started_at)
				VALUES ('worker', $1, now(), now())
				ON CONFLICT (name) DO UPDATE
				SET beat_at = now(), started_at = now(), hostname = EXCLUDED.hostname`,
				hostname)
		} else {
			_, err = pool.Exec(ctx, `
				UPDATE worker_heartbeats SET beat_at = now() WHERE name = 'worker'`)
		}
		if err != nil {
			log.Warn("worker heartbeat failed", slog.String("error", err.Error()))
			return
		}
		first = false
	}
	beat()
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			beat()
		}
	}
}
