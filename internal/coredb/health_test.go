package coredb

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestCollectStorageStats(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	dir := t.TempDir()
	db, err := Open(ctx, Options{DataDir: dir})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	stats, err := CollectStorageStats(ctx, db)
	if err != nil {
		t.Fatalf("collect stats: %v", err)
	}
	if stats.Driver != sqliteDriverName {
		t.Fatalf("expected driver %q, got %q", sqliteDriverName, stats.Driver)
	}
	if stats.MaxBytes <= 0 {
		t.Fatalf("expected max bytes > 0, got %d", stats.MaxBytes)
	}
	if stats.BytesUsed < 0 {
		t.Fatalf("expected bytes used >= 0, got %d", stats.BytesUsed)
	}
	if stats.SchemaVersion < 0 {
		t.Fatalf("expected non-negative schema version, got %d", stats.SchemaVersion)
	}
}

func TestCollectStorageStatsNoDB(t *testing.T) {
	t.Parallel()
	_, err := CollectStorageStats(context.Background(), nil)
	if err == nil {
		t.Fatalf("expected error when db nil")
	}
}

func TestCollectStorageStatsHandlesMissingJournalTable(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "flowd.db")
	if err := os.WriteFile(dbPath, []byte{}, 0o600); err != nil {
		t.Fatalf("seed db file: %v", err)
	}
	db, err := Open(ctx, Options{DataDir: dir})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	if _, err := CollectStorageStats(ctx, db); err != nil {
		t.Fatalf("collect stats on fresh db: %v", err)
	}
}
