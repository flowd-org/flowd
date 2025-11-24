package coredb

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

const (
	ruleYDefaultNamespaceLimit = 32 << 20 // 32 MiB
	ruleYMaxKeyBytes           = 256
	ruleYMaxValueBytes         = 8 << 10 // 8 KiB
)

// RuleYStore provides a constrained key/value surface backed by the Core DB.
type RuleYStore struct {
	db             *DB
	namespaceLimit int64
	maxKeyBytes    int
	maxValueBytes  int
	now            func() time.Time
}

// NewRuleYStore constructs a Rule-Y store backed by the provided DB.
func NewRuleYStore(db *DB) *RuleYStore {
	return &RuleYStore{
		db:             db,
		namespaceLimit: ruleYDefaultNamespaceLimit,
		maxKeyBytes:    ruleYMaxKeyBytes,
		maxValueBytes:  ruleYMaxValueBytes,
		now:            func() time.Time { return time.Now().UTC() },
	}
}

// ErrRuleYUnavailable indicates the backing DB has not been initialised.
var ErrRuleYUnavailable = errors.New("coredb: ruley store unavailable")

// ErrRuleYNamespaceQuota indicates the namespace would exceed its byte budget.
var ErrRuleYNamespaceQuota = errors.New("coredb: ruley namespace quota exceeded")

// ErrRuleYKeyTooLarge indicates the supplied key exceeds the configured limit.
var ErrRuleYKeyTooLarge = errors.New("coredb: ruley key exceeds maximum length")

// ErrRuleYValueTooLarge indicates the supplied value exceeds the configured limit.
var ErrRuleYValueTooLarge = errors.New("coredb: ruley value exceeds maximum length")

// RuleYItem represents a key/value pair returned from a prefix scan.
type RuleYItem struct {
	Key       []byte
	Value     []byte
	Timestamp time.Time
}

// Put stores the provided key/value pair within the namespace, applying the
// per-namespace quota. When limitBytes <= 0 the store's default limit applies.
func (s *RuleYStore) Put(ctx context.Context, namespace string, key, value []byte, limitBytes int64) error {
	if s == nil || s.db == nil || s.db.sql == nil {
		return ErrRuleYUnavailable
	}
	if err := s.validateKeyValue(key, value); err != nil {
		return err
	}
	limit := s.resolveLimit(limitBytes)

	conn := s.db.SQL()
	if err := EnsureKVNamespace(ctx, conn, namespace); err != nil {
		return err
	}

	table, err := tableName(namespace)
	if err != nil {
		return err
	}

	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	current, err := namespaceSize(ctx, tx, table)
	if err != nil {
		return err
	}

	delta, err := sizeDelta(ctx, tx, table, key, len(key)+len(value))
	if err != nil {
		return err
	}

	if limit > 0 && current+delta > limit {
		return ErrRuleYNamespaceQuota
	}

	ts := s.now().UnixMilli()
	if _, err := tx.ExecContext(ctx,
		fmt.Sprintf(`INSERT INTO %s (k, v, ts) VALUES (?, ?, ?)
ON CONFLICT(k) DO UPDATE SET v=excluded.v, ts=excluded.ts;`, table),
		key, value, ts,
	); err != nil {
		return err
	}

	return tx.Commit()
}

// Get returns the value for key within namespace.
func (s *RuleYStore) Get(ctx context.Context, namespace string, key []byte) ([]byte, time.Time, bool, error) {
	if s == nil || s.db == nil || s.db.sql == nil {
		return nil, time.Time{}, false, ErrRuleYUnavailable
	}
	conn := s.db.SQL()
	if err := EnsureKVNamespace(ctx, conn, namespace); err != nil {
		return nil, time.Time{}, false, err
	}
	table, err := tableName(namespace)
	if err != nil {
		return nil, time.Time{}, false, err
	}

	row := conn.QueryRowContext(ctx,
		fmt.Sprintf("SELECT v, ts FROM %s WHERE k = ?", table), key)

	var val []byte
	var ts int64
	if err := row.Scan(&val, &ts); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, time.Time{}, false, nil
		}
		return nil, time.Time{}, false, err
	}
	valueCopy := append([]byte(nil), val...)
	return valueCopy, time.UnixMilli(ts).UTC(), true, nil
}

// Delete removes the key from namespace, returning true when a row was deleted.
func (s *RuleYStore) Delete(ctx context.Context, namespace string, key []byte) (bool, error) {
	if s == nil || s.db == nil || s.db.sql == nil {
		return false, ErrRuleYUnavailable
	}
	conn := s.db.SQL()
	if err := EnsureKVNamespace(ctx, conn, namespace); err != nil {
		return false, err
	}
	table, err := tableName(namespace)
	if err != nil {
		return false, err
	}

	res, err := conn.ExecContext(ctx,
		fmt.Sprintf("DELETE FROM %s WHERE k = ?", table),
		key,
	)
	if err != nil {
		return false, err
	}
	affected, _ := res.RowsAffected()
	return affected > 0, nil
}

// Scan performs a lexicographic prefix scan. The cursor (if provided) must be
// the last key from a previous page and is treated as exclusive. The method
// returns up to limit items plus a cursor for the next page when more items are
// available.
func (s *RuleYStore) Scan(ctx context.Context, namespace string, prefix, cursor []byte, limit int) ([]RuleYItem, []byte, error) {
	if s == nil || s.db == nil || s.db.sql == nil {
		return nil, nil, ErrRuleYUnavailable
	}
	if limit <= 0 || limit > 256 {
		limit = 256
	}

	conn := s.db.SQL()
	if err := EnsureKVNamespace(ctx, conn, namespace); err != nil {
		return nil, nil, err
	}
	table, err := tableName(namespace)
	if err != nil {
		return nil, nil, err
	}

	query, args := buildScanQuery(table, prefix, cursor, limit+1)
	rows, err := conn.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()

	var items []RuleYItem
	for rows.Next() {
		var k, v []byte
		var ts int64
		if err := rows.Scan(&k, &v, &ts); err != nil {
			return nil, nil, err
		}
		item := RuleYItem{
			Key:       append([]byte(nil), k...),
			Value:     append([]byte(nil), v...),
			Timestamp: time.UnixMilli(ts).UTC(),
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}

	var nextCursor []byte
	if len(items) > limit {
		nextCursor = append([]byte(nil), items[limit-1].Key...)
		items = items[:limit]
	}

	return items, nextCursor, nil
}

// NamespaceSize returns the current byte footprint for namespace.
func (s *RuleYStore) NamespaceSize(ctx context.Context, namespace string) (int64, error) {
	if s == nil || s.db == nil || s.db.sql == nil {
		return 0, ErrRuleYUnavailable
	}
	conn := s.db.SQL()
	if err := EnsureKVNamespace(ctx, conn, namespace); err != nil {
		return 0, err
	}
	table, err := tableName(namespace)
	if err != nil {
		return 0, err
	}
	return namespaceSize(ctx, conn, table)
}

func (s *RuleYStore) resolveLimit(limit int64) int64 {
	if limit > 0 {
		return limit
	}
	return s.namespaceLimit
}

func (s *RuleYStore) validateKeyValue(key, value []byte) error {
	if len(key) > s.maxKeyBytes {
		return ErrRuleYKeyTooLarge
	}
	if len(value) > s.maxValueBytes {
		return ErrRuleYValueTooLarge
	}
	return nil
}

func tableName(namespace string) (string, error) {
	if !namespacePattern.MatchString(namespace) {
		return "", fmt.Errorf("invalid namespace %q", namespace)
	}
	return fmt.Sprintf("core_kv_%s", namespace), nil
}

func namespaceSize(ctx context.Context, exec interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, table string) (int64, error) {
	var total sql.NullInt64
	row := exec.QueryRowContext(ctx,
		fmt.Sprintf("SELECT COALESCE(SUM(length(k) + length(v)), 0) FROM %s", table),
	)
	if err := row.Scan(&total); err != nil {
		return 0, err
	}
	return total.Int64, nil
}

func sizeDelta(ctx context.Context, tx *sql.Tx, table string, key []byte, newSize int) (int64, error) {
	var existing sql.NullInt64
	row := tx.QueryRowContext(ctx,
		fmt.Sprintf("SELECT length(k) + length(v) FROM %s WHERE k = ?", table),
		key,
	)
	if err := row.Scan(&existing); err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			return 0, err
		}
	}
	if !existing.Valid {
		return int64(newSize), nil
	}
	return int64(newSize) - existing.Int64, nil
}

func buildScanQuery(table string, prefix, cursor []byte, limit int) (string, []any) {
	query := fmt.Sprintf("SELECT k, v, ts FROM %s WHERE 1=1", table)
	args := make([]any, 0, 4)

	if len(prefix) > 0 {
		query += " AND k >= ?"
		args = append(args, prefix)
		if upper := nextPrefix(prefix); upper != nil {
			query += " AND k < ?"
			args = append(args, upper)
		}
	}

	if len(cursor) > 0 {
		query += " AND k > ?"
		args = append(args, cursor)
	}

	query += " ORDER BY k ASC LIMIT ?"
	args = append(args, limit)

	return query, args
}

func nextPrefix(prefix []byte) []byte {
	out := append([]byte(nil), prefix...)
	for i := len(out) - 1; i >= 0; i-- {
		if out[i] != 0xFF {
			out[i]++
			return out[:i+1]
		}
	}
	return nil
}
