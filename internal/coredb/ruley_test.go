package coredb

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestRuleYStorePutGet(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db := openTestDB(t)
	store := NewRuleYStore(db)
	store.now = func() time.Time { return time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC) }

	key := []byte("foo")
	val := []byte("bar")
	if err := store.Put(ctx, "core_triggers", key, val, 0); err != nil {
		t.Fatalf("put: %v", err)
	}

	got, ts, ok, err := store.Get(ctx, "core_triggers", key)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if !ok {
		t.Fatalf("expected value present")
	}
	if string(got) != "bar" {
		t.Fatalf("unexpected value %q", got)
	}
	if !ts.Equal(store.now()) {
		t.Fatalf("unexpected timestamp %v", ts)
	}

	// Ensure NamespaceSize tracks the stored payload (key + value).
	size, err := store.NamespaceSize(ctx, "core_triggers")
	if err != nil {
		t.Fatalf("namespace size: %v", err)
	}
	expected := int64(len(key) + len(val))
	if size != expected {
		t.Fatalf("expected namespace size %d, got %d", expected, size)
	}
}

func TestRuleYStoreQuotaExceeded(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db := openTestDB(t)
	store := NewRuleYStore(db)

	limit := int64(20)
	value := make([]byte, 10)
	if err := store.Put(ctx, "core_triggers", []byte("a"), value, limit); err != nil {
		t.Fatalf("initial put: %v", err)
	}
	if err := store.Put(ctx, "core_triggers", []byte("b"), value, limit); !errors.Is(err, ErrRuleYNamespaceQuota) {
		t.Fatalf("expected quota error, got %v", err)
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
		if err := store.Put(ctx, "core_triggers", []byte(k), []byte(v), 0); err != nil {
			t.Fatalf("put %s: %v", k, err)
		}
	}

	items, cursor, err := store.Scan(ctx, "core_triggers", []byte("app:"), nil, 2)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}
	if cursor == nil {
		t.Fatalf("expected cursor for next page")
	}
	if string(items[0].Key) != "app:one" || string(items[1].Key) != "app:three" {
		t.Fatalf("unexpected first page keys: %q, %q", items[0].Key, items[1].Key)
	}

	// Fetch next page
	items2, cursor2, err := store.Scan(ctx, "core_triggers", []byte("app:"), cursor, 2)
	if err != nil {
		t.Fatalf("scan page 2: %v", err)
	}
	if len(items2) != 1 {
		t.Fatalf("expected final item, got %d", len(items2))
	}
	if cursor2 != nil {
		t.Fatalf("expected cursor to be nil on last page")
	}
	if string(items2[0].Key) != "app:two" {
		t.Fatalf("unexpected second page key %q", items2[0].Key)
	}
}

func TestRuleYStoreDelete(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db := openTestDB(t)
	store := NewRuleYStore(db)
	if err := store.Put(ctx, "core_triggers", []byte("del"), []byte("value"), 0); err != nil {
		t.Fatalf("put: %v", err)
	}
	deleted, err := store.Delete(ctx, "core_triggers", []byte("del"))
	if err != nil {
		t.Fatalf("delete: %v", err)
	}
	if !deleted {
		t.Fatalf("expected delete to return true")
	}
	_, _, ok, err := store.Get(ctx, "core_triggers", []byte("del"))
	if err != nil {
		t.Fatalf("get after delete: %v", err)
	}
	if ok {
		t.Fatalf("expected key to be removed")
	}
}

func TestRuleYStoreKeyValueLimits(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db := openTestDB(t)
	store := NewRuleYStore(db)

	bigKey := make([]byte, ruleYMaxKeyBytes+1)
	if err := store.Put(ctx, "core_triggers", bigKey, []byte("x"), 0); !errors.Is(err, ErrRuleYKeyTooLarge) {
		t.Fatalf("expected key too large error, got %v", err)
	}
	bigValue := make([]byte, ruleYMaxValueBytes+1)
	if err := store.Put(ctx, "core_triggers", []byte("k"), bigValue, 0); !errors.Is(err, ErrRuleYValueTooLarge) {
		t.Fatalf("expected value too large error, got %v", err)
	}
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
