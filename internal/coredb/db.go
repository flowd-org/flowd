// SPDX-License-Identifier: AGPL-3.0-or-later

package coredb

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/flowd-org/flowd/internal/paths"
	_ "modernc.org/sqlite"
)

const (
	sqliteDriverName = "sqlite"

	defaultBusyTimeout       = 5 * time.Second
	defaultWalAutoCheckpoint = 1000
	defaultJournalMode       = "WAL"
	defaultSynchronous       = "FULL"

	defaultGlobalMaxBytes  = 256 << 20 // 256 MiB
	defaultJournalMaxBytes = 64 << 20  // 64 MiB
)

// Options controls how the Core DB is opened.
type Options struct {
	// DataDir is the base directory where the DB file lives. If empty the
	// platform-default flowd data directory is used.
	DataDir string
	// MaxBytes places an upper bound on total DB size. Zero uses defaults.
	MaxBytes int64
	// JournalMaxBytes places an upper bound on the run journal table footprint.
	// Zero uses defaults.
	JournalMaxBytes int64
}

// DB wraps the SQLite connection used by flowd Core.
type DB struct {
	sql  *sql.DB
	opts Options
}

// Open initialises the Core DB with required pragmas and schema.
func Open(ctx context.Context, opts Options) (*DB, error) {
	dir := opts.DataDir
	if dir == "" {
		dir = paths.DataDir()
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("ensure data dir: %w", err)
	}

	dbPath := filepath.Join(dir, "flowd.db")
	dsn := fmt.Sprintf("file:%s?_pragma=busy_timeout(%d)", filepath.ToSlash(dbPath), int(defaultBusyTimeout/time.Millisecond))

	conn, err := sql.Open(sqliteDriverName, dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}

	resolvedOpts := opts
	resolvedOpts.DataDir = dir
	if resolvedOpts.MaxBytes <= 0 {
		resolvedOpts.MaxBytes = defaultGlobalMaxBytes
	}
	if resolvedOpts.JournalMaxBytes <= 0 {
		resolvedOpts.JournalMaxBytes = defaultJournalMaxBytes
	}

	if err := configureConnection(ctx, conn, resolvedOpts); err != nil {
		_ = conn.Close()
		return nil, err
	}

	if err := applyMigrations(ctx, conn); err != nil {
		_ = conn.Close()
		return nil, err
	}

	db := &DB{sql: conn, opts: resolvedOpts}
	return db, nil
}

// Close shuts down the underlying SQLite connection.
func (db *DB) Close() error {
	if db == nil || db.sql == nil {
		return nil
	}
	return db.sql.Close()
}

// SQL exposes the raw connection for internal packages that need direct access.
func (db *DB) SQL() *sql.DB {
	if db == nil {
		return nil
	}
	return db.sql
}

// Options returns the resolved options used when opening the DB.
func (db *DB) Options() Options {
	if db == nil {
		return Options{}
	}
	return db.opts
}

func configureConnection(ctx context.Context, conn *sql.DB, opts Options) error {
	conn.SetMaxOpenConns(1)
	conn.SetMaxIdleConns(1)
	conn.SetConnMaxLifetime(0)

	statements := []string{
		fmt.Sprintf("PRAGMA journal_mode=%s;", defaultJournalMode),
		fmt.Sprintf("PRAGMA synchronous=%s;", defaultSynchronous),
		"PRAGMA foreign_keys=ON;",
		"PRAGMA locking_mode=EXCLUSIVE;",
		fmt.Sprintf("PRAGMA wal_autocheckpoint=%d;", defaultWalAutoCheckpoint),
	}

	maxBytes := opts.MaxBytes
	if maxBytes <= 0 {
		maxBytes = defaultGlobalMaxBytes
	}
	pageSizeStmt := "PRAGMA page_size;"
	var pageSize int64 = 4096
	if err := conn.QueryRowContext(ctx, pageSizeStmt).Scan(&pageSize); err != nil {
		// default to 4 KiB if pragma unsupported
		pageSize = 4096
	}
	maxPages := maxBytes / pageSize
	if maxPages <= 0 {
		maxPages = defaultGlobalMaxBytes / 4096
	}
	statements = append(statements,
		fmt.Sprintf("PRAGMA max_page_count=%d;", maxPages),
	)

	for _, stmt := range statements {
		if _, err := conn.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("execute pragma %q: %w", stmt, err)
		}
	}

	journalLimit := opts.JournalMaxBytes
	if journalLimit <= 0 {
		journalLimit = defaultJournalMaxBytes
	}
	if _, err := conn.ExecContext(ctx, fmt.Sprintf("PRAGMA journal_size_limit=%d;", journalLimit)); err != nil {
		return fmt.Errorf("set journal size: %w", err)
	}

	return nil
}
