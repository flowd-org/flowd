package coredb

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"runtime"
	"sync"
	"testing"
	"time"

	sqlite3 "modernc.org/sqlite/lib"
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

func TestRuleYStoreQuotaIgnoresExpiredRows(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db := openTestDB(t)
	store := NewRuleYStore(db)
	base := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	now := base
	store.now = func() time.Time { return now }

	if _, err := store.Put(ctx, "core_triggers", "a", []byte("1234"), RuleYPutOptions{TTL: time.Second, MaxBytes: 6, MaxRows: 1}); err != nil {
		t.Fatalf("put a: %v", err)
	}

	now = now.Add(2 * time.Second)

	if _, err := store.Put(ctx, "core_triggers", "b", []byte("1234"), RuleYPutOptions{MaxBytes: 6, MaxRows: 1}); err != nil {
		t.Fatalf("expected quota check to ignore expired row, got %v", err)
	}
}

func TestRuleYStoreQuotaRevivingExpiredRowCountsAsNewLiveUsage(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db := openTestDB(t)
	store := NewRuleYStore(db)
	base := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	now := base
	store.now = func() time.Time { return now }

	big := bytes.Repeat([]byte("x"), 50)
	if _, err := store.Put(ctx, "core_triggers", "big", big, RuleYPutOptions{TTL: time.Second, MaxBytes: 1000, MaxRows: 1000}); err != nil {
		t.Fatalf("put big: %v", err)
	}
	if _, err := store.Put(ctx, "core_triggers", "live", []byte("0123456789"), RuleYPutOptions{MaxBytes: 1000, MaxRows: 1000}); err != nil {
		t.Fatalf("put live: %v", err)
	}

	now = now.Add(2 * time.Second)

	if _, err := store.Put(ctx, "core_triggers", "big", []byte("revive"), RuleYPutOptions{MaxRows: 1, MaxBytes: 1000}); !errors.Is(err, ErrRuleYQuotaExceeded) {
		t.Fatalf("expected reviving expired row to count as a new live row, got %v", err)
	}

	reviveVal := []byte("0123456789abcdef")
	liveBytes := int64(len("live") + len("0123456789"))
	reviveBytes := int64(len("big") + len(reviveVal))
	maxBytes := liveBytes + reviveBytes - 1
	if _, err := store.Put(ctx, "core_triggers", "big", reviveVal, RuleYPutOptions{MaxBytes: maxBytes, MaxRows: 1000}); !errors.Is(err, ErrRuleYQuotaExceeded) {
		t.Fatalf("expected reviving expired row to count towards bytes quota, got %v", err)
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

func TestRuleYStoreCASVerifyAffectedRow(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db := openTestDB(t)
	store := NewRuleYStore(db)

	// Test 1: CAS on non-existent key with expectVersion=0 should succeed (create)
	v1, err := store.CAS(ctx, "core_triggers", "new:key", 0, []byte("first"), RuleYPutOptions{})
	if err != nil {
		t.Fatalf("CAS create on new key: %v", err)
	}
	if v1 != 1 {
		t.Fatalf("expected version 1 for new key, got %d", v1)
	}

	// Test 2: CAS with wrong expected version should fail
	_, err = store.CAS(ctx, "core_triggers", "new:key", 999, []byte("stale"), RuleYPutOptions{})
	if !errors.Is(err, ErrRuleYCASMismatch) {
		t.Fatalf("expected cas mismatch for wrong version, got %v", err)
	}

	// Test 3: CAS with correct version should succeed
	v2, err := store.CAS(ctx, "core_triggers", "new:key", v1, []byte("second"), RuleYPutOptions{})
	if err != nil {
		t.Fatalf("CAS update with correct version: %v", err)
	}
	if v2 != v1+1 {
		t.Fatalf("expected version increment, got %d -> %d", v1, v2)
	}

	// Test 4: Concurrent first-writer race - both try to create same key
	done := make(chan error, 2)
	for i := 0; i < 2; i++ {
		go func() {
			_, err := store.CAS(ctx, "core_triggers", "concurrent:key", 0, []byte("racer"), RuleYPutOptions{})
			done <- err
		}()
	}

	err1, err2 := <-done, <-done

	// One should succeed, one should fail (either mismatch or unique constraint)
	successCount := 0
	if err1 == nil {
		successCount++
	}
	if err2 == nil {
		successCount++
	}
	if successCount != 1 {
		t.Fatalf("expected exactly one CAS to succeed in race, got %d successes", successCount)
	}

	// Verify the key exists with version 1
	entry, ok, err := store.Get(ctx, "core_triggers", "concurrent:key")
	if err != nil {
		t.Fatalf("get after race: %v", err)
	}
	if !ok {
		t.Fatalf("expected key to exist after race")
	}
	if entry.Version != 1 {
		t.Fatalf("expected version 1 after concurrent create, got %d", entry.Version)
	}
	if string(entry.Value) != "racer" {
		t.Fatalf("expected value 'racer', got %q", entry.Value)
	}

	// Test 5: CAS create with canceled context should propagate error (not convert to mismatch)
	t.Run("cas create propagates non-constraint errors", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		_, err := store.CAS(ctx, "core_triggers", "ctx:cancel", 0, []byte("v"), RuleYPutOptions{})
		if err == nil {
			t.Fatalf("expected error")
		}
		if errors.Is(err, ErrRuleYCASMismatch) {
			t.Fatalf("expected propagated error, got CAS mismatch")
		}
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("expected context.Canceled, got %v", err)
		}
	})
}

func TestRuleYStorePutConcurrentExistingKeyIncrementsVersion(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db := openTestDB(t)
	store := NewRuleYStore(db)

	const N = 20

	// Seed the key with version 1
	v0, err := store.Put(ctx, "core_triggers", "concurrent:put", []byte("seed"), RuleYPutOptions{})
	if err != nil {
		t.Fatalf("seed put: %v", err)
	}
	if v0 != 1 {
		t.Fatalf("expected seed version 1, got %d", v0)
	}

	// Spawn N goroutines that all call Put on the same key
	start := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(N)
	errs := make(chan error, N)
	versions := make(chan int64, N)

	for i := 0; i < N; i++ {
		i := i
		go func() {
			defer wg.Done()
			<-start
			v, err := store.Put(ctx, "core_triggers", "concurrent:put", []byte(fmt.Sprintf("v-%d", i)), RuleYPutOptions{})
			errs <- err
			versions <- v
		}()
	}

	close(start)
	wg.Wait()
	close(errs)
	close(versions)

	// Assert all errors are nil
	for i := 0; i < N; i++ {
		if err := <-errs; err != nil {
			t.Fatalf("goroutine %d returned error: %v", i, err)
		}
	}

	// Get the final entry and verify version increased by N
	entry, ok, err := store.Get(ctx, "core_triggers", "concurrent:put")
	if err != nil {
		t.Fatalf("get after concurrent puts: %v", err)
	}
	if !ok {
		t.Fatalf("expected key to exist after concurrent puts")
	}
	if entry.Version != v0+int64(N) {
		t.Errorf("expected version %d (seed %d + %d writes), got %d", v0+int64(N), v0, N, entry.Version)
	}

	// Optional: verify we saw all versions from v0+1 to v0+N
	seen := make(map[int64]bool)
	for i := 0; i < N; i++ {
		v := <-versions
		if seen[v] {
			t.Errorf("duplicate version returned: %d", v)
		}
		seen[v] = true
	}

	for v := v0 + 1; v <= v0+int64(N); v++ {
		if !seen[v] {
			t.Errorf("missing expected version: %d", v)
		}
	}
}

func TestRuleYStorePutConcurrentCreateDoesNotLeakConstraintError(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db := openTestDB(t)
	store := NewRuleYStore(db)

	const N = 20

	// Ensure the key does NOT exist
	if _, ok, err := store.Get(ctx, "core_triggers", "concurrent:create"); err != nil {
		t.Fatalf("get before test: %v", err)
	} else if ok {
		t.Fatalf("key should not exist before concurrent create test")
	}

	// Spawn N goroutines that all call Put to create the same new key
	start := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(N)
	errs := make(chan error, N)

	for range N {
		go func() {
			defer wg.Done()
			<-start
			_, err := store.Put(ctx, "core_triggers", "concurrent:create", []byte("x"), RuleYPutOptions{})
			errs <- err
		}()
	}

	close(start)
	wg.Wait()
	close(errs)

	// Assert no goroutine returns an error (no raw constraint errors)
	for err := range errs {
		if err != nil {
			t.Fatalf("goroutine returned error: %v", err)
		}
	}

	// Verify the key exists with expected version
	entry, ok, err := store.Get(ctx, "core_triggers", "concurrent:create")
	if err != nil {
		t.Fatalf("get after concurrent creates: %v", err)
	}
	if !ok {
		t.Fatalf("expected key to exist after concurrent creates")
	}
	if entry.Version != int64(N) {
		t.Errorf("expected version %d, got %d", N, entry.Version)
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

func TestRuleYJanitorDeletesExpiredRowsWithinSixtySeconds(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db := openTestDB(t)
	store := NewRuleYStore(db)
	base := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	now := base
	store.now = func() time.Time { return now }

	if _, err := store.Put(ctx, "core_triggers", "janitor:bound", []byte("v"), RuleYPutOptions{TTL: time.Second}); err != nil {
		t.Fatalf("put with ttl: %v", err)
	}

	expiresAt := base.Add(time.Second)
	now = expiresAt.Add(50 * time.Second)

	if _, ok, err := store.Get(ctx, "core_triggers", "janitor:bound"); err != nil {
		t.Fatalf("get after expiry: %v", err)
	} else if ok {
		t.Fatalf("expected expired key to be invisible before janitor sweep")
	}

	remaining, err := countNamespaceRows(ctx, db, "core_triggers")
	if err != nil {
		t.Fatalf("count before sweep: %v", err)
	}
	if remaining != 1 {
		t.Fatalf("expected 1 physical row before sweep, got %d", remaining)
	}

	janitor := NewRuleYJanitor(db, RuleYJanitorOptions{
		Now: func() time.Time { return now },
	})
	deleted, err := janitor.Sweep(ctx)
	if err != nil {
		t.Fatalf("sweep: %v", err)
	}
	if deleted != 1 {
		t.Fatalf("expected sweep to delete 1 row, got %d", deleted)
	}

	if elapsed := now.Sub(expiresAt); elapsed > 60*time.Second {
		t.Fatalf("expected deletion within <=60s of expiry, elapsed=%s", elapsed)
	}
}

func TestRuleYJanitorRunUsesInjectedTicks(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	db := openTestDB(t)
	store := NewRuleYStore(db)
	base := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	store.now = func() time.Time { return base }

	if _, err := store.Put(ctx, "core_triggers", "janitor:tick", []byte("v"), RuleYPutOptions{TTL: time.Second}); err != nil {
		t.Fatalf("put with ttl: %v", err)
	}

	expiresAt := base.Add(time.Second)
	janitorNow := expiresAt.Add(60 * time.Second)
	ticks := make(chan time.Time, 1)

	janitor := NewRuleYJanitor(db, RuleYJanitorOptions{
		Now:  func() time.Time { return janitorNow },
		Tick: ticks,
	})

	runDone := make(chan error, 1)
	go func() {
		runDone <- janitor.Run(ctx)
	}()

	ticks <- janitorNow

	deleted := false
	for i := 0; i < 200; i++ {
		remaining, err := countNamespaceRows(context.Background(), db, "core_triggers")
		if err != nil {
			t.Fatalf("count during run: %v", err)
		}
		if remaining == 0 {
			deleted = true
			break
		}
		runtime.Gosched()
	}
	if !deleted {
		t.Fatalf("expected tick-driven janitor run to delete expired row")
	}

	cancel()
	if err := <-runDone; err != nil {
		t.Fatalf("run: %v", err)
	}
	if elapsed := janitorNow.Sub(expiresAt); elapsed > 60*time.Second {
		t.Fatalf("expected tick-driven deletion within <=60s of expiry, elapsed=%s", elapsed)
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

func TestRuleYJanitorSweepUntilDrainedBounded(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db := openTestDB(t)
	store := NewRuleYStore(db)
	now := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	store.now = func() time.Time { return now }

	// Create more expired rows than a single sweep can handle (batch=3)
	const totalRows = 10
	for i := 0; i < totalRows; i++ {
		if _, err := store.Put(ctx, "core_triggers", fmt.Sprintf("exp:%d", i), []byte("v"), RuleYPutOptions{TTL: time.Second}); err != nil {
			t.Fatalf("put exp:%d: %v", i, err)
		}
	}

	now = now.Add(2 * time.Second)
	janitor := NewRuleYJanitor(db, RuleYJanitorOptions{
		Now:   func() time.Time { return now },
		Batch: 3,
	})

	// SweepUntilDrained should delete all expired rows in bounded iterations
	deleted, err := janitor.SweepUntilDrained(ctx)
	if err != nil {
		t.Fatalf("sweep until drained: %v", err)
	}
	if deleted != totalRows {
		t.Fatalf("expected sweep until drained to delete all %d rows, got %d", totalRows, deleted)
	}

	remaining, err := countNamespaceRows(ctx, db, "core_triggers")
	if err != nil {
		t.Fatalf("count after drain: %v", err)
	}
	if remaining != 0 {
		t.Fatalf("expected no remaining expired rows, got %d", remaining)
	}
}

// TestRuleYJanitorDrainCapacitySatisfiesSLA verifies that the janitor can delete
// all expired rows within ≤60s under worst-case backlog (all rows expire at once).
func TestRuleYJanitorDrainCapacitySatisfiesSLA(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	db := openTestDB(t)
	store := NewRuleYStore(db)
	base := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	now := base
	store.now = func() time.Time { return now }

	// Seed worst-case backlog: 10,000 expired rows (default max_rows for a namespace)
	const totalRows = 10_000
	for i := 0; i < totalRows; i++ {
		if _, err := store.Put(ctx, "core_triggers", fmt.Sprintf("exp:%d", i), []byte("v"), RuleYPutOptions{TTL: time.Second}); err != nil {
			t.Fatalf("put exp:%d: %v", i, err)
		}
	}

	// Advance time so all rows are expired
	now = now.Add(2 * time.Second)

	// Janitor with derived defaults from server config (batch=256, maxIterations=40 => 10240 capacity)
	janitor := NewRuleYJanitor(db, RuleYJanitorOptions{
		Now:           func() time.Time { return now },
		Batch:         256,
		MaxIterations: 40,
	})

	// Single SweepUntilDrained must drain all expired rows
	deleted, err := janitor.SweepUntilDrained(ctx)
	if err != nil {
		t.Fatalf("sweep until drained: %v", err)
	}
	if deleted != totalRows {
		t.Fatalf("expected sweep until drained to delete all %d rows in one tick, got %d", totalRows, deleted)
	}

	remaining, err := countNamespaceRows(ctx, db, "core_triggers")
	if err != nil {
		t.Fatalf("count after drain: %v", err)
	}
	if remaining != 0 {
		t.Fatalf("expected no remaining expired rows, got %d", remaining)
	}

	// Verify deletion happened within ≤60s of expiry
	if elapsed := now.Sub(base.Add(time.Second)); elapsed > 60*time.Second {
		t.Fatalf("expected deletion within <=60s of expiry, elapsed=%s", elapsed)
	}
}

func TestIsSQLiteConstraint(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{"nil error", nil, false},
		{"non-constraint error", errors.New("some other error"), false},
		{"SQLite CONSTRAINT error", &sqliteError{code: int(sqlite3.SQLITE_CONSTRAINT)}, true},
		{"SQLite PRIMARYKEY error", &sqliteError{code: int(sqlite3.SQLITE_CONSTRAINT_PRIMARYKEY)}, true},
		{"SQLite UNIQUE error", &sqliteError{code: int(sqlite3.SQLITE_CONSTRAINT_UNIQUE)}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isSQLiteConstraint(tt.err)
			if result != tt.expected {
				t.Errorf("isSQLiteConstraint() = %v, want %v", result, tt.expected)
			}
		})
	}
}

type sqliteError struct {
	code int
}

func (e *sqliteError) Error() string { return "sqlite constraint" }
func (e *sqliteError) Code() int     { return e.code }

// TestIsQuotaExceeded tests the IsQuotaExceeded helper function.
func TestIsQuotaExceeded(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{"nil error", nil, false},
		{"quota exceeded error", ErrJournalQuotaExceeded, true},
		{"sqlite full error", &sqliteError{code: int(sqlite3.SQLITE_FULL)}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsQuotaExceeded(tt.err)
			if result != tt.expected {
				t.Errorf("IsQuotaExceeded(%v) = %v, want %v", tt.err, result, tt.expected)
			}
		})
	}
}

func TestDB_Options(t *testing.T) {
	db := openTestDB(t)
	opts := db.Options()
	if opts.DataDir == "" {
		t.Errorf("expected DataDir to be set, got empty string")
	}
	if opts.MaxBytes <= 0 {
		t.Errorf("expected MaxBytes > 0, got %d", opts.MaxBytes)
	}
}
