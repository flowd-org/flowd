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
	`CREATE TABLE IF NOT EXISTS core_run_journal (
		seq INTEGER PRIMARY KEY AUTOINCREMENT,
		run_id TEXT NOT NULL,
		event_type TEXT NOT NULL,
		payload BLOB NOT NULL,
		ts INTEGER NOT NULL
	);`,
	`CREATE INDEX IF NOT EXISTS idx_core_journal_run_ts ON core_run_journal(run_id, ts);`,
}

func applyMigrations(ctx context.Context, conn *sql.DB) error {
	for _, stmt := range baseMigrations {
		if _, err := conn.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("apply migration: %w", err)
		}
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
