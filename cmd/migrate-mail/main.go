// Command migrate-mail backfills pre-V3 mailbox contents from the legacy
// PostgreSQL data plane into the authoritative mail store (ADR-003).
//
// For every legacy mailbox_messages row it renders the canonical RFC822
// message (headers, bodies and attachments from MinIO) and imports it into
// the owning mailbox's folder in Stalwart, then stamps migrated_at.
//
// Properties:
//
//   - idempotent: rows already stamped are skipped, and each mailbox's
//     existing Message-IDs are read from the store first, so re-running
//     never duplicates a message;
//
//   - resumable: progress is committed per row, so an interrupted run
//     continues where it stopped;
//
//   - non-destructive: legacy rows are only stamped, never deleted.
//
//     go run ./cmd/migrate-mail            # migrate everything pending
//     go run ./cmd/migrate-mail -dry-run   # report what would be migrated
//     go run ./cmd/migrate-mail -mailbox user@example.test
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"sort"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/yelnurq/email-server/internal/config"
	"github.com/yelnurq/email-server/internal/logging"
	"github.com/yelnurq/email-server/internal/mailservice"
	"github.com/yelnurq/email-server/internal/storage"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "migrate-mail:", err)
		os.Exit(1)
	}
}

// runDedupe reconciles mailboxes against their own contents: within one
// folder, a Message-ID must appear once. Extra copies (from an interrupted
// or repeated import) are destroyed, keeping the oldest. Deliberate copies
// of the same message in *different* folders — Sent plus Inbox for
// self-addressed mail — are preserved because each folder is scanned on its
// own.
func runDedupe(ctx context.Context, pool *pgxpool.Pool, mail *mailservice.Service, only string, dryRun bool) error {
	query := `SELECT address::text FROM mailboxes WHERE status = 'active'`
	args := []any{}
	if only != "" {
		query += ` AND address = $1`
		args = append(args, only)
	}
	query += ` ORDER BY address`
	rows, err := pool.Query(ctx, query, args...)
	if err != nil {
		return err
	}
	var addresses []string
	for rows.Next() {
		var a string
		if err := rows.Scan(&a); err != nil {
			rows.Close()
			return err
		}
		addresses = append(addresses, a)
	}
	rows.Close()

	totalDupes := 0
	for _, address := range addresses {
		for _, folder := range []string{"inbox", "sent", "spam", "trash", "drafts"} {
			items, _, err := mail.List(ctx, address, mailservice.ListQuery{
				FolderType: folder, Limit: 100,
			})
			if err != nil {
				fmt.Printf("  %s/%s: unreadable (%v)\n", address, folder, err)
				continue
			}
			// Oldest first: keep the first occurrence of each Message-ID.
			sort.Slice(items, func(i, j int) bool { return items[i].Date < items[j].Date })
			seen := map[string]bool{}
			for _, it := range items {
				key := mailservice.NormalizeMessageID(it.MessageID)
				if key == "" {
					continue // no identity: never auto-remove
				}
				if !seen[key] {
					seen[key] = true
					continue
				}
				totalDupes++
				if dryRun {
					fmt.Printf("  would remove duplicate %s/%s %q\n", address, folder, it.Subject)
					continue
				}
				if err := mail.Destroy(ctx, address, it.ID); err != nil {
					fmt.Printf("  failed to remove duplicate %s/%s: %v\n", address, folder, err)
					continue
				}
				fmt.Printf("  removed duplicate %s/%s %q\n", address, folder, it.Subject)
			}
		}
	}
	fmt.Printf("duplicates found=%d (dry-run=%v)\n", totalDupes, dryRun)
	return nil
}

func run() error {
	dryRun := flag.Bool("dry-run", false, "report what would be migrated without writing")
	only := flag.String("mailbox", "", "restrict the run to one mailbox address")
	limit := flag.Int("limit", 0, "stop after N copies (0 = all)")
	dedupe := flag.Bool("dedupe", false, "reconcile instead of migrating: remove duplicate Message-IDs within each folder")
	flag.Parse()

	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if cfg.MailCoreProvider != "stalwart" {
		return errors.New("MAIL_CORE_PROVIDER must be stalwart to migrate mail")
	}
	log := logging.New("migrate-mail", cfg.LogLevel, cfg.LogFormat)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()

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
	mail := &mailservice.Service{
		JMAP: &mailservice.JMAP{
			BaseURL:      cfg.StalwartBaseURL,
			MasterUser:   cfg.StalwartMasterUser,
			MasterSecret: cfg.StalwartMasterPass,
		},
	}

	if *dedupe {
		return runDedupe(ctx, pool, mail, *only, *dryRun)
	}

	query := `
		SELECT mm.id, mm.message_id, mb.address::text, f.type, mm.is_read, mm.is_starred,
		       m.rfc_message_id, m.subject, mm.created_at
		FROM mailbox_messages mm
		JOIN mailboxes mb ON mb.id = mm.mailbox_id
		JOIN folders f ON f.id = mm.folder_id
		JOIN messages m ON m.id = mm.message_id
		WHERE mm.migrated_at IS NULL AND m.status <> 'draft'`
	args := []any{}
	if *only != "" {
		query += ` AND mb.address = $1`
		args = append(args, *only)
	}
	query += ` ORDER BY mb.address, mm.created_at`

	rows, err := pool.Query(ctx, query, args...)
	if err != nil {
		return err
	}
	type copyRow struct {
		id, messageID, address, folder string
		isRead, isStarred              bool
		rfcID, subject                 string
		receivedAt                     time.Time
	}
	var pending []copyRow
	for rows.Next() {
		var c copyRow
		if err := rows.Scan(&c.id, &c.messageID, &c.address, &c.folder,
			&c.isRead, &c.isStarred, &c.rfcID, &c.subject, &c.receivedAt); err != nil {
			rows.Close()
			return err
		}
		pending = append(pending, c)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}

	fmt.Printf("legacy copies pending: %d\n", len(pending))
	if *dryRun {
		for _, c := range pending {
			fmt.Printf("  would migrate %-28s %-7s %s\n", c.address, c.folder, c.subject)
		}
		return nil
	}

	// Per-mailbox set of Message-IDs already in the store: the duplicate
	// guard that makes re-runs safe even if migrated_at was lost.
	seen := map[string]map[string]bool{}
	loadSeen := func(address string) (map[string]bool, error) {
		if s, ok := seen[address]; ok {
			return s, nil
		}
		s := map[string]bool{}
		for _, folder := range []string{"inbox", "sent", "spam", "trash", "drafts"} {
			items, _, err := mail.List(ctx, address, mailservice.ListQuery{
				FolderType: folder, Limit: 100,
			})
			if err != nil {
				return nil, err
			}
			for _, it := range items {
				if it.MessageID != "" {
					// Both sides are normalized: the control plane stores
					// "<id@domain>", JMAP reports "id@domain".
					s[mailservice.NormalizeMessageID(it.MessageID)] = true
				}
			}
		}
		seen[address] = s
		return s, nil
	}

	var migrated, skipped, failed int
	for i, c := range pending {
		if *limit > 0 && migrated+skipped >= *limit {
			break
		}
		if ctx.Err() != nil {
			fmt.Println("interrupted; progress is saved")
			break
		}
		known, err := loadSeen(c.address)
		if err != nil {
			return fmt.Errorf("reading mailbox %s: %w", c.address, err)
		}
		// The RFC Message-ID is the identity across both worlds.
		rfc := mailservice.NormalizeMessageID(c.rfcID)
		if rfc != "" && known[rfc] {
			if _, err := pool.Exec(ctx,
				`UPDATE mailbox_messages SET migrated_at = now() WHERE id = $1`, c.id); err != nil {
				return err
			}
			skipped++
			continue
		}
		raw, err := mailservice.RenderMessage(ctx, pool, store, c.messageID)
		if err != nil {
			log.Error("render failed", "copy", c.id, "address", c.address, "error", err.Error())
			failed++
			continue
		}
		// Keep the original arrival time so migrated mail sorts correctly.
		importCtx := mailservice.WithReceivedAt(ctx, c.receivedAt)
		if _, err := mail.Import(importCtx, c.address, c.folder, raw, c.isRead); err != nil {
			log.Error("import failed", "copy", c.id, "address", c.address, "error", err.Error())
			failed++
			continue
		}
		if rfc != "" {
			known[rfc] = true
		}
		if _, err := pool.Exec(ctx,
			`UPDATE mailbox_messages SET migrated_at = now() WHERE id = $1`, c.id); err != nil {
			return err
		}
		migrated++
		if (i+1)%25 == 0 {
			fmt.Printf("  ... %d/%d\n", i+1, len(pending))
		}
	}

	fmt.Printf("migrated=%d skipped(already present)=%d failed=%d\n", migrated, skipped, failed)
	if failed > 0 {
		return fmt.Errorf("%d copies could not be migrated; legacy rows are unchanged for those", failed)
	}
	return nil
}
