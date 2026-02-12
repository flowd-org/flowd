package coredb

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"
)

func TestRuleYStorePutGetDelete(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db := openTestDB(t)
	store := NewRuleYStore(db)
	store.now = func() time.Time { return time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC) }

	version, err := store.Put(ctx, "core_triggers", "Foo", []byte("bar"), RuleYPutOptions{})
	if err != nil {
		t.Fatalf("put: %v", err)
	}
	if version != 1 {
		t.Fatalf("expected version 1, got %d", version)
	}

	entry, ok, err := store.Get(ctx, "core_triggers", "foo")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if !ok {
		t.Fatalf("expected key to exist")
	}
	if string(entry.Value) != "bar" {
		t.Fatalf("unexpected value: %q", entry.Value)
	}
	if entry.Version != 1 {
		t.Fatalf("expected version 1, got %d", entry.Version)
	}
	if !entry.UpdatedAt.Equal(store.now()) {
		t.Fatalf("unexpected updated_at: %v", entry.UpdatedAt)
	}

	deleted, err := store.Del(ctx, "core_triggers", "foo")
	if err != nil {
		t.Fatalf("delete: %v", err)
	}
	if !deleted {
		t.Fatalf("expected delete=true")
	}
}

func TestRuleYStoreQuotaExceeded(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db := openTestDB(t)
	store := NewRuleYStore(db)

	if _, err := store.Put(ctx, "core_triggers", "a", []byte("1234"), RuleYPutOptions{MaxBytes: 6}); err != nil {
		t.Fatalf("initial put: %v", err)
	}
	if _, err := store.Put(ctx, "core_triggers", "b", []byte("1234"), RuleYPutOptions{MaxBytes: 6}); !errors.Is(err, ErrRuleYQuotaExceeded) {
		t.Fatalf("expected quota error, got %v", err)
	}

	if _, err := store.Put(ctx, "core_invocation_state", "row:a", []byte("x"), RuleYPutOptions{MaxRows: 1, MaxBytes: 1024}); err != nil {
		t.Fatalf("row quota initial put: %v", err)
	}
	if _, err := store.Put(ctx, "core_invocation_state", "row:b", []byte("x"), RuleYPutOptions{MaxRows: 1, MaxBytes: 1024}); !errors.Is(err, ErrRuleYQuotaExceeded) {
		t.Fatalf("expected row quota error, got %v", err)
	}
}

func TestRuleYStoreScanPrefix(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db := openTestDB(t)
	store := NewRuleYStore(db)

	testData := map[string]string{
		"app:one":   "v1",
		"app:two":   "v2",
		"app:three": "v3",
		"bee:one":   "v4",
	}
	for k, v := range testData {
		if _, err := store.Put(ctx, "core_triggers", k, []byte(v), RuleYPutOptions{}); err != nil {
			t.Fatalf("put %s: %v", k, err)
		}
	}

	items, cursor, err := store.Scan(ctx, "core_triggers", "app:", "", 2)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}
	if cursor == "" {
		t.Fatalf("expected next cursor")
	}

	items2, cursor2, err := store.Scan(ctx, "core_triggers", "app:", cursor, 2)
	if err != nil {
		t.Fatalf("scan page 2: %v", err)
	}
	if len(items2) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items2))
	}
	if cursor2 != "" {
		t.Fatalf("expected empty cursor at end, got %q", cursor2)
	}
}

func TestRuleYStoreValidationAndCAS(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db := openTestDB(t)
	store := NewRuleYStore(db)

	if _, err := store.Put(ctx, "core_triggers", "UPPER", []byte("ok"), RuleYPutOptions{}); err != nil {
		t.Fatalf("put uppercase key: %v", err)
	}
	if _, err := store.Put(ctx, "core_triggers", "bad key", []byte("x"), RuleYPutOptions{}); !errors.Is(err, ErrRuleYInvalidKey) {
		t.Fatalf("expected invalid key error, got %v", err)
	}
	tooBig := make([]byte, ruleYMaxValueBytes+1)
	if _, err := store.Put(ctx, "core_triggers", "k", tooBig, RuleYPutOptions{}); !errors.Is(err, ErrRuleYValueTooLarge) {
		t.Fatalf("expected value-too-large error, got %v", err)
	}

	v1, err := store.CAS(ctx, "core_triggers", "cas:key", 0, []byte("one"), RuleYPutOptions{})
	if err != nil {
		t.Fatalf("cas create: %v", err)
	}
	v2, err := store.CAS(ctx, "core_triggers", "cas:key", v1, []byte("two"), RuleYPutOptions{})
	if err != nil {
		t.Fatalf("cas update: %v", err)
	}
	if v2 != v1+1 {
		t.Fatalf("expected version increment, got %d -> %d", v1, v2)
	}
	if _, err := store.CAS(ctx, "core_triggers", "cas:key", v1, []byte("stale"), RuleYPutOptions{}); !errors.Is(err, ErrRuleYCASMismatch) {
		t.Fatalf("expected cas mismatch, got %v", err)
	}
}

func TestRuleYStoreTTLInvisibleOnReadAndScan(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db := openTestDB(t)
	store := NewRuleYStore(db)
	now := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	store.now = func() time.Time { return now }

	if _, err := store.Put(ctx, "core_triggers", "ttl:key", []byte("v"), RuleYPutOptions{TTL: time.Second}); err != nil {
		t.Fatalf("put with ttl: %v", err)
	}

	if _, ok, err := store.Get(ctx, "core_triggers", "ttl:key"); err != nil {
		t.Fatalf("get before expiry: %v", err)
	} else if !ok {
		t.Fatalf("expected key before expiry")
	}

	now = now.Add(2 * time.Second)

	if _, ok, err := store.Get(ctx, "core_triggers", "ttl:key"); err != nil {
		t.Fatalf("get after expiry: %v", err)
	} else if ok {
		t.Fatalf("expected key to be invisible after expiry")
	}

	items, _, err := store.Scan(ctx, "core_triggers", "ttl:", "", 10)
	if err != nil {
		t.Fatalf("scan after expiry: %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("expected expired key excluded from scan, got %d items", len(items))
	}
}

func TestRuleYStoreScanLimitIsCappedAt1000(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db := openTestDB(t)
	store := NewRuleYStore(db)

	for i := 0; i < 1005; i++ {
		key := fmt.Sprintf("cap:%04d", i)
		if _, err := store.Put(ctx, "core_triggers", key, []byte("v"), RuleYPutOptions{}); err != nil {
			t.Fatalf("put %q: %v", key, err)
		}
	}

	items, cursor, err := store.Scan(ctx, "core_triggers", "cap:", "", 5000)
	if err != nil {
		t.Fatalf("scan with over-limit request: %v", err)
	}
	if len(items) != 1000 {
		t.Fatalf("expected 1000 items due to cap, got %d", len(items))
	}
	if cursor == "" {
		t.Fatalf("expected non-empty cursor when over cap")
	}
}

func TestRuleYJanitorSweepDeletesExpiredRowsInBatches(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db := openTestDB(t)
	store := NewRuleYStore(db)
	now := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	store.now = func() time.Time { return now }

	for i := 0; i < 3; i++ {
		if _, err := store.Put(ctx, "core_triggers", fmt.Sprintf("exp:%d", i), []byte("v"), RuleYPutOptions{TTL: time.Second}); err != nil {
			t.Fatalf("put exp:%d: %v", i, err)
		}
	}

	now = now.Add(2 * time.Second)
	janitor := NewRuleYJanitor(db, RuleYJanitorOptions{
		Now:   func() time.Time { return now },
		Batch: 2,
	})

	deleted, err := janitor.Sweep(ctx)
	if err != nil {
		t.Fatalf("sweep 1: %v", err)
	}
	if deleted != 2 {
		t.Fatalf("expected first sweep to delete 2 rows, got %d", deleted)
	}

	remaining, err := countNamespaceRows(ctx, db, "core_triggers")
	if err != nil {
		t.Fatalf("count after first sweep: %v", err)
	}
	if remaining != 1 {
		t.Fatalf("expected 1 remaining row, got %d", remaining)
	}

	deleted, err = janitor.Sweep(ctx)
	if err != nil {
		t.Fatalf("sweep 2: %v", err)
	}
	if deleted != 1 {
		t.Fatalf("expected second sweep to delete 1 row, got %d", deleted)
	}
}

func countNamespaceRows(ctx context.Context, db *DB, namespace string) (int64, error) {
	var count int64
	err := db.SQL().QueryRowContext(ctx, `SELECT COUNT(*) FROM kv WHERE ns = ?`, namespace).Scan(&count)
	return count, err
}

func openTestDB(t *testing.T) *DB {
	t.Helper()
	dir := t.TempDir()
	db, err := Open(context.Background(), Options{DataDir: dir})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}
