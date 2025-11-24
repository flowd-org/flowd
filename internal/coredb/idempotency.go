// SPDX-License-Identifier: AGPL-3.0-or-later

package coredb

import (
	context "context"
	"database/sql"
	"errors"
	"time"

	"github.com/flowd-org/flowd/internal/metrics"
	"github.com/flowd-org/flowd/internal/observability/tracing"
)

// IdempotencyStore provides helpers for persisting idempotent responses.
type IdempotencyStore struct {
	db *sql.DB
}

// NewIdempotencyStore returns a store backed by the provided DB.
func NewIdempotencyStore(db *DB) *IdempotencyStore {
	if db == nil {
		return nil
	}
	return &IdempotencyStore{db: db.sql}
}

// Lookup returns the stored response payload, HTTP status code, and body hash for the given key/endpoint combination.
func (s *IdempotencyStore) Lookup(ctx context.Context, key, endpoint string, now time.Time) (body []byte, status int, bodyHash string, ok bool, err error) {
	if s == nil {
		return nil, 0, "", false, nil
	}
	ctx, span := tracing.Start(ctx, "coredb.idempotency.lookup",
		tracing.PersistDriver(sqliteDriverName),
		tracing.PersistOp("lookup"),
		tracing.PersistKeyspace("core_idempotency"),
		tracing.String("idempotency.key", key),
		tracing.String("idempotency.endpoint", endpoint),
	)
	defer tracing.End(span, &err)

	timer := metrics.StartPersistenceTimer(metrics.PersistenceOperationIdempotencyLookup)
	outcome := metrics.PersistenceOutcomeError
	defer func() {
		if timer != nil {
			timer.Observe(outcome)
		}
		if span != nil {
			span.SetAttributes(tracing.String("idempotency.outcome", outcome))
		}
	}()

	row := s.db.QueryRowContext(ctx, `SELECT body, status, body_sha256, ttl_expires_at FROM core_idempotency WHERE key = ? AND endpoint = ?`, key, endpoint)
	var expires int64
	if scanErr := row.Scan(&body, &status, &bodyHash, &expires); errors.Is(scanErr, sql.ErrNoRows) {
		outcome = metrics.PersistenceOutcomeMiss
		err = nil
		return nil, 0, "", false, nil
	} else if scanErr != nil {
		err = scanErr
		return nil, 0, "", false, err
	}
	if expires > 0 && now.UnixMilli() > expires {
		_, _ = s.db.ExecContext(ctx, `DELETE FROM core_idempotency WHERE key = ? AND endpoint = ?`, key, endpoint)
		metrics.RecordPersistenceEviction(metrics.PersistenceKindIdempotency, int64(len(body)))
		outcome = metrics.PersistenceOutcomeExpired
		return nil, 0, "", false, nil
	}
	outcome = metrics.PersistenceOutcomeHit
	ok = true
	return body, status, bodyHash, ok, nil
}

// Store persists the response payload for the supplied idempotency key.
func (s *IdempotencyStore) Store(ctx context.Context, key, endpoint, bodyHash string, status int, payload []byte, expiresAt time.Time) (err error) {
	if s == nil {
		return nil
	}
	ctx, span := tracing.Start(ctx, "coredb.idempotency.store",
		tracing.PersistDriver(sqliteDriverName),
		tracing.PersistOp("store"),
		tracing.PersistKeyspace("core_idempotency"),
		tracing.String("idempotency.key", key),
		tracing.String("idempotency.endpoint", endpoint),
		tracing.Int("response.status", status),
		tracing.Int("payload.bytes", len(payload)),
	)
	defer tracing.End(span, &err)

	timer := metrics.StartPersistenceTimer(metrics.PersistenceOperationIdempotencyStore)
	outcome := metrics.PersistenceOutcomeError
	defer func() {
		if timer != nil {
			timer.Observe(outcome)
		}
	}()
	now := time.Now().UTC().UnixMilli()
	expires := expiresAt.UnixMilli()
	_, err = s.db.ExecContext(ctx, `
INSERT INTO core_idempotency (key, endpoint, body_sha256, status, body, created_at, ttl_expires_at)
VALUES (?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(key, endpoint) DO UPDATE SET
  body_sha256 = excluded.body_sha256,
  status = excluded.status,
  body = excluded.body,
  created_at = excluded.created_at,
  ttl_expires_at = excluded.ttl_expires_at;
`, key, endpoint, bodyHash, status, payload, now, expires)
	if err != nil {
		return err
	}
	outcome = metrics.PersistenceOutcomeOK
	if span != nil {
		span.SetAttributes(tracing.String("idempotency.outcome", outcome))
	}
	return nil
}
