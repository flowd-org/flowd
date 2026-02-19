package coredb

import (
	"context"
	"fmt"
	"testing"
)

func TestMigrationsCreateKVSchema(t *testing.T) {
	t.Parallel()

	db := openTestDB(t)
	ctx := context.Background()

	assertTableColumns(t, db, ctx, "kv", []string{
		"ns",
		"k",
		"v",
		"content_type",
		"version",
		"updated_at",
		"expires_at",
	})
	assertIndexExists(t, db, ctx, "idx_kv_expires_at")
}

func TestMigrationsCreateArtifactMetadataSchema(t *testing.T) {
	t.Parallel()

	db := openTestDB(t)
	ctx := context.Background()

	assertTableColumns(t, db, ctx, "core_artifacts", []string{
		"artifact_id",
		"tenant",
		"job_id",
		"run_id",
		"name",
		"content_type",
		"size_bytes",
		"created_at",
	})
	assertIndexExists(t, db, ctx, "idx_core_artifacts_tenant_job_run")
}

func assertTableColumns(t *testing.T, db *DB, ctx context.Context, table string, expected []string) {
	t.Helper()

	rows, err := db.SQL().QueryContext(ctx, fmt.Sprintf("PRAGMA table_info(%s)", table))
	if err != nil {
		t.Fatalf("inspect table %s: %v", table, err)
	}
	defer rows.Close()

	present := make(map[string]struct{})
	for rows.Next() {
		var cid int
		var name string
		var ctype string
		var notnull int
		var dflt any
		var pk int
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			t.Fatalf("read table info for %s: %v", table, err)
		}
		present[name] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate table info for %s: %v", table, err)
	}

	for _, col := range expected {
		if _, ok := present[col]; !ok {
			t.Fatalf("expected column %s.%s to exist", table, col)
		}
	}
}

func assertIndexExists(t *testing.T, db *DB, ctx context.Context, indexName string) {
	t.Helper()

	row := db.SQL().QueryRowContext(ctx, "SELECT COUNT(*) FROM sqlite_master WHERE type = 'index' AND name = ?", indexName)
	var count int
	if err := row.Scan(&count); err != nil {
		t.Fatalf("check index %s: %v", indexName, err)
	}
	if count != 1 {
		t.Fatalf("expected index %s to exist", indexName)
	}
}
