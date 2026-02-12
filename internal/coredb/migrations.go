// SPDX-License-Identifier: AGPL-3.0-or-later

package coredb

import (
	"context"
	"database/sql"
	"fmt"
	"regexp"
)

var baseMigrations = [...]string{
	`CREATE TABLE IF NOT EXISTS core_idempotency (
		key TEXT NOT NULL,
		endpoint TEXT NOT NULL,
		body_sha256 TEXT NOT NULL,
		status INTEGER NOT NULL,
		body BLOB NOT NULL,
		created_at INTEGER NOT NULL,
		ttl_expires_at INTEGER NOT NULL,
		PRIMARY KEY (key, endpoint)
	);`,
	`CREATE INDEX IF NOT EXISTS idx_core_idemp_ttl ON core_idempotency(ttl_expires_at);`,
	`CREATE TABLE IF NOT EXISTS core_runs (
		run_id TEXT PRIMARY KEY,
		job_id TEXT NOT NULL,
		status TEXT NOT NULL,
		started_at INTEGER NOT NULL,
		finished_at INTEGER,
		result BLOB,
		executor TEXT,
		runtime TEXT,
		security_profile TEXT,
		provenance BLOB,
		request_id TEXT
	);`,
	`CREATE INDEX IF NOT EXISTS idx_core_runs_started_at ON core_runs(started_at);`,
	`CREATE TABLE IF NOT EXISTS core_run_journal (
		seq INTEGER PRIMARY KEY AUTOINCREMENT,
		run_id TEXT NOT NULL,
		event_type TEXT NOT NULL,
		payload BLOB NOT NULL,
		ts INTEGER NOT NULL
	);`,
	`CREATE INDEX IF NOT EXISTS idx_core_journal_run_ts ON core_run_journal(run_id, ts);`,
	`CREATE TABLE IF NOT EXISTS kv (
		ns TEXT NOT NULL,
		k TEXT NOT NULL,
		v BLOB NOT NULL,
		content_type TEXT NOT NULL,
		version INTEGER NOT NULL,
		updated_at INTEGER NOT NULL,
		expires_at INTEGER,
		PRIMARY KEY (ns, k)
	);`,
	`CREATE INDEX IF NOT EXISTS idx_kv_expires_at ON kv(expires_at);`,
	`CREATE TABLE IF NOT EXISTS core_artifacts (
		artifact_id TEXT PRIMARY KEY,
		tenant TEXT NOT NULL,
		job_id TEXT NOT NULL,
		run_id TEXT NOT NULL,
		name TEXT NOT NULL,
		content_type TEXT,
		size_bytes INTEGER NOT NULL,
		created_at INTEGER NOT NULL
	);`,
	`CREATE INDEX IF NOT EXISTS idx_core_artifacts_tenant_job_run ON core_artifacts(tenant, job_id, run_id);`,
}

func applyMigrations(ctx context.Context, conn *sql.DB) error {
	for _, stmt := range baseMigrations {
		if _, err := conn.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("apply migration: %w", err)
		}
	}
	if err := ensureRunRequestIDColumn(ctx, conn); err != nil {
		return err
	}
	return nil
}

func ensureRunRequestIDColumn(ctx context.Context, conn *sql.DB) error {
	rows, err := conn.QueryContext(ctx, "PRAGMA table_info(core_runs);")
	if err != nil {
		return fmt.Errorf("inspect core_runs schema: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var cid int
		var name string
		var ctype string
		var notnull int
		var dflt sql.NullString
		var pk int
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			return fmt.Errorf("inspect core_runs schema: %w", err)
		}
		if name == "request_id" {
			return nil
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("inspect core_runs schema: %w", err)
	}
	if _, err := conn.ExecContext(ctx, "ALTER TABLE core_runs ADD COLUMN request_id TEXT;"); err != nil {
		return fmt.Errorf("add core_runs.request_id: %w", err)
	}
	return nil
}

var namespacePattern = regexp.MustCompile(`^[a-zA-Z0-9_]+$`)

// EnsureKVNamespace materialises the KV table for the provided namespace.
func EnsureKVNamespace(ctx context.Context, conn *sql.DB, namespace string) error {
	if !namespacePattern.MatchString(namespace) {
		return fmt.Errorf("invalid namespace %q", namespace)
	}
	stmt := fmt.Sprintf(`CREATE TABLE IF NOT EXISTS core_kv_%s (
		k BLOB PRIMARY KEY,
		v BLOB NOT NULL,
		ts INTEGER NOT NULL
	);`, namespace)
	if _, err := conn.ExecContext(ctx, stmt); err != nil {
		return fmt.Errorf("ensure kv namespace %q: %w", namespace, err)
	}
	return nil
}
