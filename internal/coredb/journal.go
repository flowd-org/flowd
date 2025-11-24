// SPDX-License-Identifier: AGPL-3.0-or-later

package coredb

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/flowd-org/flowd/internal/metrics"
	"github.com/flowd-org/flowd/internal/observability/tracing"
)

// ErrJournalQuotaExceeded indicates the requested append cannot be satisfied
// because the payload is larger than the configured journal limit.
var ErrJournalQuotaExceeded = errors.New("coredb: journal quota exceeded")

// JournalEntry represents a persisted run event.
type JournalEntry struct {
	Seq       int64
	RunID     string
	EventType string
	Payload   []byte
	Timestamp time.Time
}

// Journal provides append-only persistence backed by the Core DB.
type Journal struct {
	db       *sql.DB
	maxBytes int64
	nowFn    func() time.Time
}

// NewJournal returns a Journal backed by the provided DB with the supplied
// maximum size budget. When maxBytes is zero or negative the default (64 MiB)
// is used.
func NewJournal(db *DB, maxBytes int64) *Journal {
	if db == nil {
		return nil
	}
	if maxBytes <= 0 {
		maxBytes = defaultJournalMaxBytes
	}
	return &Journal{
		db:       db.sql,
		maxBytes: maxBytes,
		nowFn: func() time.Time {
			return time.Now().UTC()
		},
	}
}

// Append stores an event for the provided run. It returns the persisted entry
// including the allocated sequence number. Appends are performed in a single
// transaction to ensure eviction and insertion remain atomic.
func (j *Journal) Append(ctx context.Context, runID, eventType string, payload []byte, ts time.Time) (entry JournalEntry, err error) {
	if j == nil {
		return entry, nil
	}
	ctx, span := tracing.Start(ctx, "coredb.journal.append",
		tracing.PersistDriver(sqliteDriverName),
		tracing.PersistOp("append"),
		tracing.PersistKeyspace("core_run_journal"),
		tracing.RunID(runID),
		tracing.Int("payload.bytes", len(payload)),
		tracing.String("journal.event_type", eventType),
	)
	defer tracing.End(span, &err)

	timer := metrics.StartPersistenceTimer(metrics.PersistenceOperationJournalAppend)
	outcome := metrics.PersistenceOutcomeError
	var evictedTotal int64
	defer func() {
		if timer != nil {
			timer.Observe(outcome)
		}
		if span != nil {
			attrs := []tracing.Attribute{tracing.String("journal.outcome", outcome)}
			if evictedTotal > 0 {
				attrs = append(attrs, tracing.Int64("journal.evicted_bytes_total", evictedTotal))
			}
			span.SetAttributes(attrs...)
		}
	}()

	if runID == "" {
		outcome = metrics.PersistenceOutcomeError
		err = fmt.Errorf("append journal: run id required")
		return entry, err
	}
	if len(payload) == 0 {
		outcome = metrics.PersistenceOutcomeError
		err = fmt.Errorf("append journal: payload required")
		return entry, err
	}
	payloadBytes := int64(len(payload))
	if payloadBytes > j.maxBytes {
		outcome = metrics.PersistenceOutcomeQuotaExceeded
		err = ErrJournalQuotaExceeded
		return entry, err
	}

	now := ts
	if now.IsZero() {
		now = j.nowFn()
	}

	var tx *sql.Tx
	tx, err = j.db.BeginTx(ctx, nil)
	if err != nil {
		err = fmt.Errorf("begin journal tx: %w", err)
		return entry, err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	var existingBytes int64
	if err = tx.QueryRowContext(ctx, `SELECT COALESCE(SUM(length(payload)), 0) FROM core_run_journal`).Scan(&existingBytes); err != nil {
		err = fmt.Errorf("journal size lookup: %w", err)
		return entry, err
	}

	for existingBytes+payloadBytes > j.maxBytes {
		var seq int64
		var size int64
		err = tx.QueryRowContext(ctx, `SELECT seq, length(payload) FROM core_run_journal ORDER BY seq ASC LIMIT 1`).Scan(&seq, &size)
		if errors.Is(err, sql.ErrNoRows) {
			outcome = metrics.PersistenceOutcomeError
			break
		}
		if err != nil {
			err = fmt.Errorf("journal eviction lookup: %w", err)
			return entry, err
		}
		if _, err = tx.ExecContext(ctx, `DELETE FROM core_run_journal WHERE seq = ?`, seq); err != nil {
			err = fmt.Errorf("journal eviction delete seq=%d: %w", seq, err)
			return entry, err
		}
		metrics.RecordPersistenceEviction(metrics.PersistenceKindJournal, size)
		evictedTotal += size
		if span != nil {
			span.SetAttributes(
				tracing.Int64("journal.evicted_seq", seq),
				tracing.Int64("journal.evicted_bytes", size),
			)
		}
		existingBytes -= size
		if existingBytes < 0 {
			existingBytes = 0
		}
	}

	var res sql.Result
	res, err = tx.ExecContext(ctx, `
INSERT INTO core_run_journal (run_id, event_type, payload, ts)
VALUES (?, ?, ?, ?)
`, runID, eventType, payload, now.UnixMilli())
	if err != nil {
		err = fmt.Errorf("journal insert: %w", err)
		return entry, err
	}
	var seq int64
	seq, err = res.LastInsertId()
	if err != nil {
		err = fmt.Errorf("journal last insert id: %w", err)
		return entry, err
	}

	if err = tx.Commit(); err != nil {
		err = fmt.Errorf("journal commit: %w", err)
		return entry, err
	}

	entry = JournalEntry{
		Seq:       seq,
		RunID:     runID,
		EventType: eventType,
		Payload:   append([]byte(nil), payload...),
		Timestamp: now,
	}
	outcome = metrics.PersistenceOutcomeOK
	if span != nil {
		span.SetAttributes(tracing.Int64("journal.seq", entry.Seq))
	}
	return entry, nil
}

// Bounds returns the earliest and latest sequence currently retained for the
// provided run. A zero earliest indicates no events are stored.
func (j *Journal) Bounds(ctx context.Context, runID string) (earliest, latest int64, err error) {
	if j == nil {
		return 0, 0, nil
	}
	if err = j.db.QueryRowContext(ctx, `
SELECT COALESCE(MIN(seq), 0), COALESCE(MAX(seq), 0)
FROM core_run_journal WHERE run_id = ?
`, runID).Scan(&earliest, &latest); err != nil {
		return 0, 0, fmt.Errorf("journal bounds: %w", err)
	}
	return earliest, latest, nil
}

// ForEach streams events for the supplied run strictly after the provided
// sequence (i.e. seq > afterSeq) in ascending order. Iteration halts if the
// callback returns an error.
func (j *Journal) ForEach(ctx context.Context, runID string, afterSeq int64, fn func(JournalEntry) error) (err error) {
	if j == nil || fn == nil {
		return nil
	}
	ctx, span := tracing.Start(ctx, "coredb.journal.read",
		tracing.PersistDriver(sqliteDriverName),
		tracing.PersistOp("read"),
		tracing.PersistKeyspace("core_run_journal"),
		tracing.RunID(runID),
		tracing.Int64("journal.after_seq", afterSeq),
	)
	defer tracing.End(span, &err)

	timer := metrics.StartPersistenceTimer(metrics.PersistenceOperationJournalRead)
	outcome := metrics.PersistenceOutcomeError
	entries := 0
	defer func() {
		if timer != nil {
			timer.Observe(outcome)
		}
		if span != nil {
			span.SetAttributes(
				tracing.String("journal.outcome", outcome),
				tracing.Int("journal.entries", entries),
			)
		}
	}()

	var rows *sql.Rows
	rows, err = j.db.QueryContext(ctx, `
SELECT seq, event_type, payload, ts
FROM core_run_journal
WHERE run_id = ? AND seq > ?
ORDER BY seq ASC
`, runID, afterSeq)
	if err != nil {
		err = fmt.Errorf("journal query: %w", err)
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var seq int64
		var eventType string
		var payload []byte
		var tsMillis int64
		if scanErr := rows.Scan(&seq, &eventType, &payload, &tsMillis); scanErr != nil {
			err = fmt.Errorf("journal scan: %w", scanErr)
			return err
		}
		entry := JournalEntry{
			Seq:       seq,
			RunID:     runID,
			EventType: eventType,
			Payload:   append([]byte(nil), payload...),
			Timestamp: time.UnixMilli(tsMillis).UTC(),
		}
		entries++
		if span != nil {
			span.SetAttributes(tracing.Int64("journal.last_seq", entry.Seq))
		}
		if fnErr := fn(entry); fnErr != nil {
			err = fnErr
			return err
		}
	}
	if rowsErr := rows.Err(); rowsErr != nil {
		err = fmt.Errorf("journal rows: %w", rowsErr)
		return err
	}
	outcome = metrics.PersistenceOutcomeOK
	return nil
}

// ParseEventID converts an SSE event ID into a sequence integer. It returns
// zero when the ID is empty.
func ParseEventID(id string) (int64, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return 0, nil
	}
	seq, err := strconv.ParseInt(id, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid event id %q: %w", id, err)
	}
	return seq, nil
}
