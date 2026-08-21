package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/yelnurq/email-server/internal/mailservice"
)

// fakeStore simulates the mail store: imports are recorded per
// mailbox/folder and become visible to List, exactly like Stalwart.
type fakeStore struct {
	// imports counts every Import call — the invariant under test is that
	// one logical message is imported exactly once across runs.
	imports  int
	byFolder map[string][]mailservice.ListItem // "address/folder" → items
	nextID   int
}

func newFakeStore() *fakeStore {
	return &fakeStore{byFolder: map[string][]mailservice.ListItem{}}
}

func (f *fakeStore) key(address, folder string) string { return address + "/" + folder }

func (f *fakeStore) List(_ context.Context, email string, q mailservice.ListQuery) ([]mailservice.ListItem, int, error) {
	items := f.byFolder[f.key(email, q.FolderType)]
	total := len(items)
	if q.Offset >= total {
		return nil, total, nil
	}
	end := q.Offset + q.Limit
	if q.Limit <= 0 || end > total {
		end = total
	}
	return items[q.Offset:end], total, nil
}

func (f *fakeStore) Import(_ context.Context, email, folderType string, raw []byte, _ bool) (string, error) {
	f.imports++
	f.nextID++
	// The raw bytes carry "Message-ID: <id>"; extract it the way JMAP
	// reports it (bare form).
	rfcID := ""
	for _, line := range strings.Split(string(raw), "\r\n") {
		if strings.HasPrefix(line, "Message-ID:") {
			rfcID = strings.Trim(strings.TrimSpace(strings.TrimPrefix(line, "Message-ID:")), "<>")
		}
	}
	k := f.key(email, folderType)
	f.byFolder[k] = append(f.byFolder[k], mailservice.ListItem{
		ID:        fmt.Sprintf("s%d", f.nextID),
		MessageID: rfcID,
		Date:      time.Now().UTC().Format(time.RFC3339),
	})
	return fmt.Sprintf("s%d", f.nextID), nil
}

func (f *fakeStore) Destroy(_ context.Context, email, id string) error {
	for k, items := range f.byFolder {
		if !strings.HasPrefix(k, email+"/") {
			continue
		}
		for i, it := range items {
			if it.ID == id {
				f.byFolder[k] = append(items[:i], items[i+1:]...)
				return nil
			}
		}
	}
	return errors.New("not found")
}

// fakeDB records migrated_at stamps and can simulate a crash: when
// failStamps > 0, the next stamps return an error the way a killed process
// or dead connection would — after the store import already succeeded.
type fakeDB struct {
	stamped    map[string]bool
	failStamps int
}

func (f *fakeDB) Exec(_ context.Context, _ string, args ...any) (pgconn.CommandTag, error) {
	if f.failStamps > 0 {
		f.failStamps--
		return pgconn.CommandTag{}, errors.New("simulated crash before status commit")
	}
	if f.stamped == nil {
		f.stamped = map[string]bool{}
	}
	f.stamped[args[0].(string)] = true
	return pgconn.NewCommandTag("UPDATE 1"), nil
}

func testRender(_ context.Context, messageID string) ([]byte, error) {
	return []byte("Message-ID: <" + messageID + "@company.test>\r\n\r\nbody"), nil
}

var testLog = slog.New(slog.NewTextHandler(&strings.Builder{}, nil))

// TestCrashBetweenImportAndStampProducesOneCopy is the V4 §9 / §188 gate at
// the unit level: import succeeds, the process dies before the PostgreSQL
// stamp, the worker restarts and retries — the result must be exactly one
// logical copy.
func TestCrashBetweenImportAndStampProducesOneCopy(t *testing.T) {
	ctx := context.Background()
	store := newFakeStore()
	pending := []copyRow{{
		id: "copy-1", messageID: "legacy-1", address: "user@company.test",
		folder: "inbox", rfcID: "<legacy-1@company.test>", receivedAt: time.Now(),
	}}

	// Run 1: the import lands in the store, then the stamp "crashes".
	db := &fakeDB{failStamps: 1}
	if _, err := migrateCopies(ctx, db, store, testRender, testLog, pending, 0); err == nil {
		t.Fatal("run 1 should fail at the stamp")
	}
	if store.imports != 1 {
		t.Fatalf("run 1 imports = %d, want 1", store.imports)
	}
	if db.stamped["copy-1"] {
		t.Fatal("run 1 must not have stamped the row (that was the crash)")
	}

	// Run 2 (restart): the row is still pending; the duplicate guard must
	// find the Message-ID in the store and stamp without importing again.
	db2 := &fakeDB{}
	res, err := migrateCopies(ctx, db2, store, testRender, testLog, pending, 0)
	if err != nil {
		t.Fatalf("run 2: %v", err)
	}
	if store.imports != 1 {
		t.Fatalf("after restart imports = %d, want exactly 1 (duplicate!)", store.imports)
	}
	if res.skipped != 1 || res.migrated != 0 {
		t.Fatalf("run 2 = %+v, want skipped=1 migrated=0", res)
	}
	if !db2.stamped["copy-1"] {
		t.Fatal("run 2 must stamp the recovered row")
	}
	if got := len(store.byFolder["user@company.test/inbox"]); got != 1 {
		t.Fatalf("store holds %d copies, want 1", got)
	}
}

// TestGuardSeesBeyondFirstPage covers the V3 regression: the duplicate
// guard must page through the whole folder, not only the newest 100.
func TestGuardSeesBeyondFirstPage(t *testing.T) {
	ctx := context.Background()
	store := newFakeStore()
	// Pre-fill 250 messages; the target Message-ID is the oldest (first).
	k := store.key("user@company.test", "inbox")
	for i := 0; i < 250; i++ {
		store.byFolder[k] = append(store.byFolder[k], mailservice.ListItem{
			ID:        fmt.Sprintf("pre%d", i),
			MessageID: fmt.Sprintf("old-%d@company.test", i),
			Date:      time.Now().Add(time.Duration(i) * time.Second).UTC().Format(time.RFC3339),
		})
	}

	pending := []copyRow{{
		id: "copy-old", messageID: "old-0", address: "user@company.test",
		folder: "inbox", rfcID: "<old-0@company.test>", receivedAt: time.Now(),
	}}
	db := &fakeDB{}
	res, err := migrateCopies(ctx, db, store, testRender, testLog, pending, 0)
	if err != nil {
		t.Fatal(err)
	}
	if store.imports != 0 {
		t.Fatalf("guard missed a message beyond page 1: %d imports", store.imports)
	}
	if res.skipped != 1 {
		t.Fatalf("res = %+v, want skipped=1", res)
	}
}

// TestRepeatedRunIsIdempotent: running the migration twice over the same
// pending set imports each copy once.
func TestRepeatedRunIsIdempotent(t *testing.T) {
	ctx := context.Background()
	store := newFakeStore()
	var pending []copyRow
	for i := 0; i < 3; i++ {
		pending = append(pending, copyRow{
			id: fmt.Sprintf("copy-%d", i), messageID: fmt.Sprintf("m-%d", i),
			address: "user@company.test", folder: "inbox",
			rfcID: fmt.Sprintf("<m-%d@company.test>", i), receivedAt: time.Now(),
		})
	}
	if _, err := migrateCopies(ctx, &fakeDB{}, store, testRender, testLog, pending, 0); err != nil {
		t.Fatal(err)
	}
	if store.imports != 3 {
		t.Fatalf("first run imports = %d, want 3", store.imports)
	}
	// Simulate lost stamps (the V3 incident): all rows pending again.
	res, err := migrateCopies(ctx, &fakeDB{}, store, testRender, testLog, pending, 0)
	if err != nil {
		t.Fatal(err)
	}
	if store.imports != 3 {
		t.Fatalf("second run duplicated mail: imports = %d, want 3", store.imports)
	}
	if res.skipped != 3 {
		t.Fatalf("second run = %+v, want skipped=3", res)
	}
}
