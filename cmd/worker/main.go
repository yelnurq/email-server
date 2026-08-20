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

	w := &delivery.Worker{Pool: pool, NATS: nc, Log: log}
	log.Info("worker started")
	if err := w.Run(ctx); err != nil {
		return err
	}
	log.Info("worker shutting down")
	return nil
}
