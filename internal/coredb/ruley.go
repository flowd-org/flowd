package coredb

import (
	"context"
	"database/sql"
	"errors"
	"regexp"
	"strings"
	"time"
)

const (
	ruleYDefaultMaxRows  = 10_000
	ruleYDefaultMaxBytes = 10 << 20 // 10 MiB
	ruleYMaxValueBytes   = 1 << 20  // 1 MiB
	ruleYMaxScanLimit    = 1000
)

var ruleYKeyPattern = regexp.MustCompile(`^[a-z0-9._:/-]{1,128}$`)

var defaultRuleYNamespaces = map[string]RuleYNamespaceQuota{
	"core_triggers":         {MaxRows: ruleYDefaultMaxRows, MaxBytes: ruleYDefaultMaxBytes},
	"core_invocation_state": {MaxRows: ruleYDefaultMaxRows, MaxBytes: ruleYDefaultMaxBytes},
}

// RuleYStore provides a constrained key/value surface backed by the Core DB.
type RuleYStore struct {
	db        *DB
	now       func() time.Time
	allowlist map[string]RuleYNamespaceQuota
}

// NewRuleYStore constructs a Rule-Y store backed by the provided DB.
func NewRuleYStore(db *DB) *RuleYStore {
	allow := make(map[string]RuleYNamespaceQuota, len(defaultRuleYNamespaces))
	for ns, q := range defaultRuleYNamespaces {
		allow[ns] = q
	}
	return &RuleYStore{
		db:        db,
		now:       func() time.Time { return time.Now().UTC() },
		allowlist: allow,
	}
}

// RuleYNamespaceQuota defines per-namespace row/bytes limits.
type RuleYNamespaceQuota struct {
	MaxRows  int64
	MaxBytes int64
}

// SetAllowlist replaces the namespace allowlist used by the store.
func (s *RuleYStore) SetAllowlist(allowlist map[string]RuleYNamespaceQuota) {
	if s == nil {
		return
	}
	next := make(map[string]RuleYNamespaceQuota, len(allowlist))
	for ns, q := range allowlist {
		normalized := strings.ToLower(strings.TrimSpace(ns))
		if normalized == "" {
			continue
		}
		if q.MaxRows <= 0 {
			q.MaxRows = ruleYDefaultMaxRows
		}
		if q.MaxBytes <= 0 {
			q.MaxBytes = ruleYDefaultMaxBytes
		}
		next[normalized] = q
	}
	s.allowlist = next
}

// ErrRuleYUnavailable indicates the backing DB has not been initialised.
var ErrRuleYUnavailable = errors.New("coredb: ruley store unavailable")

// ErrRuleYNamespaceForbidden indicates namespace is not in the explicit allowlist.
var ErrRuleYNamespaceForbidden = errors.New("coredb: ruley namespace forbidden")

// ErrRuleYQuotaExceeded indicates the namespace would exceed configured limits.
var ErrRuleYQuotaExceeded = errors.New("coredb: kv/quota-exceeded")

// ErrRuleYInvalidKey indicates the key does not satisfy Rule-Y constraints.
var ErrRuleYInvalidKey = errors.New("coredb: ruley key invalid")

// ErrRuleYValueTooLarge indicates the supplied value exceeds the configured limit.
var ErrRuleYValueTooLarge = errors.New("coredb: ruley value exceeds maximum length")

// ErrRuleYCASMismatch indicates the expected version did not match current value.
var ErrRuleYCASMismatch = errors.New("coredb: ruley cas mismatch")

// RuleYPutOptions controls write behavior.
type RuleYPutOptions struct {
	ContentType string
	TTL         time.Duration
	MaxRows     int64
	MaxBytes    int64
}

// RuleYGetResult contains metadata for a successful key lookup.
type RuleYGetResult struct {
	Value       []byte
	ContentType string
	Version     int64
	UpdatedAt   time.Time
}

// RuleYItem represents a key/value pair returned from a prefix scan.
type RuleYItem struct {
	Key         string
	Value       []byte
	ContentType string
	Version     int64
	UpdatedAt   time.Time
}

// Put stores key/value and returns the new per-key version.
func (s *RuleYStore) Put(ctx context.Context, namespace, key string, value []byte, opts RuleYPutOptions) (int64, error) {
	if s == nil || s.db == nil || s.db.sql == nil {
		return 0, ErrRuleYUnavailable
	}
	ns, quota, err := s.normalizeNamespace(namespace)
	if err != nil {
		return 0, err
	}
	normalizedKey, err := normalizeRuleYKey(key)
	if err != nil {
		return 0, err
	}
	if len(value) > ruleYMaxValueBytes {
		return 0, ErrRuleYValueTooLarge
	}

	conn := s.db.SQL()
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()

	nowMillis := s.now().UnixMilli()
	existingVersion, existed, existedCounted, oldBytes, err := readKVExisting(ctx, tx, ns, normalizedKey, nowMillis)
	if err != nil {
		return 0, err
	}
	effectiveQuota := quota
	if opts.MaxRows > 0 {
		effectiveQuota.MaxRows = opts.MaxRows
	}
	if opts.MaxBytes > 0 {
		effectiveQuota.MaxBytes = opts.MaxBytes
	}
	newEntryBytes := len(normalizedKey) + len(value)
	if err := enforceRuleYQuota(ctx, tx, ns, nowMillis, effectiveQuota, existed, existedCounted, oldBytes, newEntryBytes); err != nil {
		return 0, err
	}

	updatedAt := nowMillis
	expiresAt := nullableExpiryMillis(updatedAt, opts.TTL)
	contentType := strings.TrimSpace(opts.ContentType)
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	newVersion := int64(1)
	if existed {
		newVersion = existingVersion + 1
		_, err = tx.ExecContext(ctx,
			`UPDATE kv
			SET v = ?, content_type = ?, version = ?, updated_at = ?, expires_at = ?
			WHERE ns = ? AND k = ?`,
			value, contentType, newVersion, updatedAt, expiresAt, ns, normalizedKey,
		)
	} else {
		_, err = tx.ExecContext(ctx,
			`INSERT INTO kv (ns, k, v, content_type, version, updated_at, expires_at)
			VALUES (?, ?, ?, ?, ?, ?, ?)`,
			ns, normalizedKey, value, contentType, newVersion, updatedAt, expiresAt,
		)
	}
	if err != nil {
		return 0, err
	}

	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return newVersion, nil
}

// Get returns the value for key within namespace.
func (s *RuleYStore) Get(ctx context.Context, namespace, key string) (RuleYGetResult, bool, error) {
	if s == nil || s.db == nil || s.db.sql == nil {
		return RuleYGetResult{}, false, ErrRuleYUnavailable
	}
	ns, _, err := s.normalizeNamespace(namespace)
	if err != nil {
		return RuleYGetResult{}, false, err
	}
	normalizedKey, err := normalizeRuleYKey(key)
	if err != nil {
		return RuleYGetResult{}, false, err
	}

	conn := s.db.SQL()
	nowMillis := s.now().UnixMilli()
	row := conn.QueryRowContext(ctx,
		`SELECT v, content_type, version, updated_at
		 FROM kv
		 WHERE ns = ? AND k = ?
		   AND (expires_at IS NULL OR expires_at > ?)`,
		ns, normalizedKey, nowMillis,
	)

	var val []byte
	var contentType string
	var version int64
	var updatedAt int64
	if err := row.Scan(&val, &contentType, &version, &updatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return RuleYGetResult{}, false, nil
		}
		return RuleYGetResult{}, false, err
	}

	valueCopy := append([]byte(nil), val...)
	return RuleYGetResult{
		Value:       valueCopy,
		ContentType: contentType,
		Version:     version,
		UpdatedAt:   time.UnixMilli(updatedAt).UTC(),
	}, true, nil
}

// Del removes the key from namespace, returning true when a row was deleted.
func (s *RuleYStore) Del(ctx context.Context, namespace, key string) (bool, error) {
	if s == nil || s.db == nil || s.db.sql == nil {
		return false, ErrRuleYUnavailable
	}
	ns, _, err := s.normalizeNamespace(namespace)
	if err != nil {
		return false, err
	}
	normalizedKey, err := normalizeRuleYKey(key)
	if err != nil {
		return false, err
	}

	conn := s.db.SQL()
	res, err := conn.ExecContext(ctx,
		`DELETE FROM kv WHERE ns = ? AND k = ?`,
		ns, normalizedKey,
	)
	if err != nil {
		return false, err
	}
	affected, _ := res.RowsAffected()
	return affected > 0, nil
}

// Scan performs a lexicographic prefix scan and returns an exclusive next cursor.
func (s *RuleYStore) Scan(ctx context.Context, namespace, prefix, cursor string, limit int) ([]RuleYItem, string, error) {
	if s == nil || s.db == nil || s.db.sql == nil {
		return nil, "", ErrRuleYUnavailable
	}
	ns, _, err := s.normalizeNamespace(namespace)
	if err != nil {
		return nil, "", err
	}
	normalizedPrefix := strings.ToLower(strings.TrimSpace(prefix))
	if normalizedPrefix != "" {
		if err := validateRuleYKeyPrefix(normalizedPrefix); err != nil {
			return nil, "", err
		}
	}
	normalizedCursor := strings.ToLower(strings.TrimSpace(cursor))
	if normalizedCursor != "" {
		if err := validateRuleYKeyPrefix(normalizedCursor); err != nil {
			return nil, "", err
		}
	}
	if limit <= 0 {
		limit = 100
	}
	if limit > ruleYMaxScanLimit {
		limit = ruleYMaxScanLimit
	}

	conn := s.db.SQL()
	query, args := buildScanQuery(ns, normalizedPrefix, normalizedCursor, limit+1, s.now().UnixMilli())
	rows, err := conn.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, "", err
	}
	defer rows.Close()

	var items []RuleYItem
	for rows.Next() {
		var k string
		var v []byte
		var contentType string
		var version int64
		var updatedAt int64
		if err := rows.Scan(&k, &v, &contentType, &version, &updatedAt); err != nil {
			return nil, "", err
		}
		item := RuleYItem{
			Key:         k,
			Value:       append([]byte(nil), v...),
			ContentType: contentType,
			Version:     version,
			UpdatedAt:   time.UnixMilli(updatedAt).UTC(),
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, "", err
	}

	var nextCursor string
	if len(items) > limit {
		nextCursor = items[limit-1].Key
		items = items[:limit]
	}

	return items, nextCursor, nil
}

// CAS updates key/value only when expectVersion matches. expectVersion=0
// creates the key when missing.
func (s *RuleYStore) CAS(ctx context.Context, namespace, key string, expectVersion int64, value []byte, opts RuleYPutOptions) (int64, error) {
	if s == nil || s.db == nil || s.db.sql == nil {
		return 0, ErrRuleYUnavailable
	}
	ns, quota, err := s.normalizeNamespace(namespace)
	if err != nil {
		return 0, err
	}
	normalizedKey, err := normalizeRuleYKey(key)
	if err != nil {
		return 0, err
	}
	if len(value) > ruleYMaxValueBytes {
		return 0, ErrRuleYValueTooLarge
	}

	conn := s.db.SQL()
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()

	nowMillis := s.now().UnixMilli()
	existingVersion, existed, existedCounted, oldBytes, err := readKVExisting(ctx, tx, ns, normalizedKey, nowMillis)
	if err != nil {
		return 0, err
	}
	if !existed {
		if expectVersion != 0 {
			return 0, ErrRuleYCASMismatch
		}
	} else if existingVersion != expectVersion {
		return 0, ErrRuleYCASMismatch
	}
	effectiveQuota := quota
	if opts.MaxRows > 0 {
		effectiveQuota.MaxRows = opts.MaxRows
	}
	if opts.MaxBytes > 0 {
		effectiveQuota.MaxBytes = opts.MaxBytes
	}
	newEntryBytes := len(normalizedKey) + len(value)
	if err := enforceRuleYQuota(ctx, tx, ns, nowMillis, effectiveQuota, existed, existedCounted, oldBytes, newEntryBytes); err != nil {
		return 0, err
	}

	updatedAt := nowMillis
	expiresAt := nullableExpiryMillis(updatedAt, opts.TTL)
	contentType := strings.TrimSpace(opts.ContentType)
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	newVersion := expectVersion + 1
	var affected int64
	if !existed {
		newVersion = 1
		res, err := tx.ExecContext(ctx,
			`INSERT INTO kv (ns, k, v, content_type, version, updated_at, expires_at)
			VALUES (?, ?, ?, ?, ?, ?, ?)`,
			ns, normalizedKey, value, contentType, newVersion, updatedAt, expiresAt,
		)
		if err != nil {
			return 0, err
		}
		affected, _ = res.RowsAffected()
	} else {
		res, err := tx.ExecContext(ctx,
			`UPDATE kv
			SET v = ?, content_type = ?, version = ?, updated_at = ?, expires_at = ?
			WHERE ns = ? AND k = ? AND version = ?`,
			value, contentType, newVersion, updatedAt, expiresAt, ns, normalizedKey, expectVersion,
		)
		if err != nil {
			return 0, err
		}
		affected, _ = res.RowsAffected()
		if affected == 0 {
			return 0, ErrRuleYCASMismatch
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return newVersion, nil
}

func (s *RuleYStore) normalizeNamespace(namespace string) (string, RuleYNamespaceQuota, error) {
	ns := strings.ToLower(strings.TrimSpace(namespace))
	if ns == "" {
		return "", RuleYNamespaceQuota{}, ErrRuleYNamespaceForbidden
	}
	quota, ok := s.allowlist[ns]
	if !ok {
		return "", RuleYNamespaceQuota{}, ErrRuleYNamespaceForbidden
	}
	if quota.MaxRows <= 0 {
		quota.MaxRows = ruleYDefaultMaxRows
	}
	if quota.MaxBytes <= 0 {
		quota.MaxBytes = ruleYDefaultMaxBytes
	}
	return ns, quota, nil
}

func normalizeRuleYKey(key string) (string, error) {
	normalized := strings.ToLower(strings.TrimSpace(key))
	if !ruleYKeyPattern.MatchString(normalized) {
		return "", ErrRuleYInvalidKey
	}
	return normalized, nil
}

// NormalizeRuleYKey validates and canonicalizes a Rule-Y key for cross-package use.
func NormalizeRuleYKey(key string) (string, error) {
	return normalizeRuleYKey(key)
}

func validateRuleYKeyPrefix(prefix string) error {
	if len(prefix) > 128 {
		return ErrRuleYInvalidKey
	}
	for _, r := range prefix {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '.' || r == '_' || r == ':' || r == '/' || r == '-' {
			continue
		}
		return ErrRuleYInvalidKey
	}
	return nil
}

func readKVExisting(ctx context.Context, tx *sql.Tx, namespace, key string, nowMillis int64) (version int64, exists bool, counted bool, bytes int64, err error) {
	row := tx.QueryRowContext(ctx, `SELECT version, length(k) + length(v), expires_at FROM kv WHERE ns = ? AND k = ?`, namespace, key)
	var storedVersion int64
	var storedBytes int64
	var expiresAt sql.NullInt64
	if err := row.Scan(&storedVersion, &storedBytes, &expiresAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, false, false, 0, nil
		}
		return 0, false, false, 0, err
	}
	counted = !expiresAt.Valid || expiresAt.Int64 > nowMillis
	return storedVersion, true, counted, storedBytes, nil
}

func enforceRuleYQuota(ctx context.Context, tx *sql.Tx, namespace string, nowMillis int64, quota RuleYNamespaceQuota, exists bool, existedCounted bool, oldBytes int64, newBytes int) error {
	if quota.MaxRows <= 0 && quota.MaxBytes <= 0 {
		return nil
	}

	var rows int64
	var bytes int64
	row := tx.QueryRowContext(ctx,
		`SELECT COUNT(*), COALESCE(SUM(length(k) + length(v)), 0)
		 FROM kv
		 WHERE ns = ?
		   AND (expires_at IS NULL OR expires_at > ?)`,
		namespace, nowMillis,
	)
	if err := row.Scan(&rows, &bytes); err != nil {
		return err
	}
	newRows := rows
	if !exists || !existedCounted {
		newRows++
	}
	if quota.MaxRows > 0 && newRows > quota.MaxRows {
		return ErrRuleYQuotaExceeded
	}

	totalBytes := bytes
	if existedCounted {
		totalBytes -= oldBytes
	}
	totalBytes += int64(newBytes)
	if quota.MaxBytes > 0 && totalBytes > quota.MaxBytes {
		return ErrRuleYQuotaExceeded
	}
	return nil
}

func nullableExpiryMillis(updatedAtMillis int64, ttl time.Duration) sql.NullInt64 {
	if ttl <= 0 {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: updatedAtMillis + ttl.Milliseconds(), Valid: true}
}

func buildScanQuery(namespace, prefix, cursor string, limit int, nowMillis int64) (string, []any) {
	query := `SELECT k, v, content_type, version, updated_at
		FROM kv
		WHERE ns = ?
		  AND (expires_at IS NULL OR expires_at > ?)`
	args := make([]any, 0, 4)
	args = append(args, namespace, nowMillis)

	if prefix != "" {
		query += " AND k >= ?"
		args = append(args, prefix)
		if upper := nextPrefix(prefix); upper != "" {
			query += " AND k < ?"
			args = append(args, upper)
		}
	}

	if cursor != "" {
		query += " AND k > ?"
		args = append(args, cursor)
	}

	query += " ORDER BY k ASC LIMIT ?"
	args = append(args, limit)

	return query, args
}

func nextPrefix(prefix string) string {
	out := []byte(prefix)
	for i := len(out) - 1; i >= 0; i-- {
		if out[i] != 0xFF {
			out[i]++
			return string(out[:i+1])
		}
	}
	return ""
}
