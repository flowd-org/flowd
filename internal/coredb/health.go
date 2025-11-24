// SPDX-License-Identifier: AGPL-3.0-or-later

package coredb

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// StorageStats captures high-level information about the Core DB.
type StorageStats struct {
	Driver          string `json:"driver"`
	OK              bool   `json:"ok"`
	BytesUsed       int64  `json:"bytes_used"`
	MaxBytes        int64  `json:"max_bytes"`
	JournalBytes    int64  `json:"journal_bytes"`
	JournalMaxBytes int64  `json:"journal_max_bytes"`
	EvictionActive  bool   `json:"eviction_active"`
	SchemaVersion   int64  `json:"schema_version"`
}

// CollectStorageStats inspects the backing SQLite database and returns
// aggregate storage statistics suitable for health monitoring.
func CollectStorageStats(ctx context.Context, db *DB) (StorageStats, error) {
	if db == nil || db.sql == nil {
		return StorageStats{}, errors.New("coredb: database not initialised")
	}
	conn := db.SQL()
	stats := StorageStats{Driver: sqliteDriverName}

	pageSize, err := querySingleInt(ctx, conn, "PRAGMA page_size;")
	if err != nil {
		return stats, fmt.Errorf("coredb: lookup page_size: %w", err)
	}
	pageCount, err := querySingleInt(ctx, conn, "PRAGMA page_count;")
	if err != nil {
		return stats, fmt.Errorf("coredb: lookup page_count: %w", err)
	}
	maxPageCount, err := querySingleInt(ctx, conn, "PRAGMA max_page_count;")
	if err != nil {
		return stats, fmt.Errorf("coredb: lookup max_page_count: %w", err)
	}

	userVersion, err := querySingleInt(ctx, conn, "PRAGMA user_version;")
	if err != nil {
		return stats, fmt.Errorf("coredb: lookup user_version: %w", err)
	}
	stats.SchemaVersion = userVersion

	journalLimit, err := querySingleInt(ctx, conn, "PRAGMA journal_size_limit;")
	if err != nil {
		return stats, fmt.Errorf("coredb: lookup journal_size_limit: %w", err)
	}
	stats.JournalMaxBytes = journalLimit

	var journalBytes sql.NullInt64
	if err := conn.QueryRowContext(ctx, `SELECT COALESCE(SUM(length(payload)),0) FROM core_run_journal`).Scan(&journalBytes); err != nil {
		return stats, fmt.Errorf("coredb: journal payload inspection: %w", err)
	}
	stats.JournalBytes = journalBytes.Int64

	stats.BytesUsed = pageCount * pageSize
	stats.MaxBytes = maxPageCount * pageSize

	maxBytes := stats.MaxBytes
	if maxBytes <= 0 {
		maxBytes = int64(db.opts.MaxBytes)
		stats.MaxBytes = maxBytes
	}
	stats.OK = maxBytes == 0 || stats.BytesUsed < maxBytes

	if stats.JournalMaxBytes > 0 && stats.JournalBytes >= stats.JournalMaxBytes {
		stats.EvictionActive = true
	}
	if stats.MaxBytes > 0 && stats.BytesUsed >= (stats.MaxBytes*9)/10 {
		stats.EvictionActive = true
	}

	return stats, nil
}

func querySingleInt(ctx context.Context, conn *sql.DB, stmt string) (int64, error) {
	var out sql.NullInt64
	if err := conn.QueryRowContext(ctx, stmt).Scan(&out); err != nil {
		return 0, err
	}
	return out.Int64, nil
}
