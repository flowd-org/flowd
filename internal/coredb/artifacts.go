// SPDX-License-Identifier: AGPL-3.0-or-later

package coredb

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
)

var (
	// ErrArtifactStoreUnavailable indicates the backing DB has not been initialised.
	ErrArtifactStoreUnavailable = errors.New("coredb: artifact store unavailable")
	// ErrArtifactInvalidID indicates an artifact ID that is not a UUIDv7 with RFC4122 variant.
	ErrArtifactInvalidID = errors.New("coredb: artifact_id must be uuidv7 with rfc4122 variant")
	// ErrArtifactInvalidMetadata indicates required metadata is missing.
	ErrArtifactInvalidMetadata = errors.New("coredb: artifact metadata invalid")
)

// ArtifactRecord represents a persisted artifact metadata row.
type ArtifactRecord struct {
	ArtifactID  string
	Tenant      string
	JobID       string
	RunID       string
	Name        string
	ContentType string
	SizeBytes   int64
	CreatedAt   time.Time
}

// ArtifactStore provides CoreDB-backed artifact metadata helpers.
type ArtifactStore struct {
	db  *sql.DB
	now func() time.Time
}

// NewArtifactStore returns an artifact metadata store backed by the provided DB.
func NewArtifactStore(db *DB) *ArtifactStore {
	if db == nil {
		return nil
	}
	return &ArtifactStore{db: db.sql, now: func() time.Time { return time.Now().UTC() }}
}

// Create inserts immutable artifact metadata.
func (s *ArtifactStore) Create(ctx context.Context, record ArtifactRecord) error {
	if s == nil || s.db == nil {
		return ErrArtifactStoreUnavailable
	}
	normalized, err := normalizeArtifactRecord(record, s.now)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx, `
INSERT INTO core_artifacts (artifact_id, tenant, job_id, run_id, name, content_type, size_bytes, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?);
`, normalized.ArtifactID, normalized.Tenant, normalized.JobID, normalized.RunID, normalized.Name, normalized.ContentType, normalized.SizeBytes, normalized.CreatedAt.UnixMilli())
	return err
}

// Get retrieves artifact metadata by artifact ID.
func (s *ArtifactStore) Get(ctx context.Context, artifactID string) (ArtifactRecord, bool, error) {
	if s == nil || s.db == nil {
		return ArtifactRecord{}, false, ErrArtifactStoreUnavailable
	}
	normalizedID, err := normalizeArtifactID(artifactID)
	if err != nil {
		return ArtifactRecord{}, false, err
	}
	row := s.db.QueryRowContext(ctx, `
SELECT artifact_id, tenant, job_id, run_id, name, content_type, size_bytes, created_at
FROM core_artifacts
WHERE artifact_id = ?;
`, normalizedID)

	var out ArtifactRecord
	var contentType sql.NullString
	var createdAtMillis int64
	if err := row.Scan(&out.ArtifactID, &out.Tenant, &out.JobID, &out.RunID, &out.Name, &contentType, &out.SizeBytes, &createdAtMillis); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ArtifactRecord{}, false, nil
		}
		return ArtifactRecord{}, false, err
	}
	out.CreatedAt = time.UnixMilli(createdAtMillis).UTC()
	if contentType.Valid {
		out.ContentType = contentType.String
	}
	return out, true, nil
}

func normalizeArtifactRecord(record ArtifactRecord, nowFn func() time.Time) (ArtifactRecord, error) {
	artifactID, err := normalizeArtifactID(record.ArtifactID)
	if err != nil {
		return ArtifactRecord{}, err
	}
	tenant := strings.TrimSpace(record.Tenant)
	jobID := strings.TrimSpace(record.JobID)
	runID := strings.TrimSpace(record.RunID)
	name := strings.TrimSpace(record.Name)
	if tenant == "" || jobID == "" || runID == "" || name == "" {
		return ArtifactRecord{}, ErrArtifactInvalidMetadata
	}
	if record.SizeBytes < 0 {
		return ArtifactRecord{}, ErrArtifactInvalidMetadata
	}
	createdAt := record.CreatedAt.UTC()
	if createdAt.IsZero() {
		if nowFn == nil {
			nowFn = func() time.Time { return time.Now().UTC() }
		}
		createdAt = nowFn().UTC()
	}
	contentType := strings.TrimSpace(record.ContentType)
	return ArtifactRecord{
		ArtifactID:  artifactID,
		Tenant:      tenant,
		JobID:       jobID,
		RunID:       runID,
		Name:        name,
		ContentType: contentType,
		SizeBytes:   record.SizeBytes,
		CreatedAt:   createdAt,
	}, nil
}

func normalizeArtifactID(input string) (string, error) {
	trimmed := strings.TrimSpace(input)
	id, err := uuid.Parse(trimmed)
	if err != nil {
		return "", ErrArtifactInvalidID
	}
	if id.Version() != 7 {
		return "", ErrArtifactInvalidID
	}
	// RFC4122 variant is 1 (0b01xxxx in the variant octet)
	if id.Variant() != uuid.RFC4122 {
		return "", ErrArtifactInvalidID
	}
	return strings.ToLower(id.String()), nil
}

// NormalizeArtifactIDForStorage validates and canonicalizes artifact IDs for cross-package usage.
func NormalizeArtifactIDForStorage(input string) (string, error) {
	return normalizeArtifactID(input)
}
