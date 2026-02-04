// SPDX-License-Identifier: AGPL-3.0-or-later

package coredb

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"
)

// RunRecord represents persisted run metadata.
type RunRecord struct {
	ID              string
	JobID           string
	Status          string
	StartedAt       time.Time
	FinishedAt      *time.Time
	Result          map[string]any
	Executor        string
	Runtime         string
	SecurityProfile string
	Provenance      map[string]any
	RequestID       string
}

// RunStore provides CoreDB-backed run record persistence.
type RunStore struct {
	db *sql.DB
}

// NewRunStore returns a run store backed by the provided DB.
func NewRunStore(db *DB) *RunStore {
	if db == nil {
		return nil
	}
	return &RunStore{db: db.sql}
}

// Create inserts or replaces a run record.
func (s *RunStore) Create(ctx context.Context, record RunRecord) error {
	return s.upsert(ctx, record)
}

// Update replaces a run record by ID.
func (s *RunStore) Update(ctx context.Context, record RunRecord) error {
	return s.upsert(ctx, record)
}

// Get retrieves a run record by ID.
func (s *RunStore) Get(ctx context.Context, id string) (RunRecord, bool, error) {
	if s == nil {
		return RunRecord{}, false, nil
	}
	row := s.db.QueryRowContext(ctx, `SELECT run_id, job_id, status, started_at, finished_at, result, executor, runtime, security_profile, provenance, request_id FROM core_runs WHERE run_id = ?`, id)
	var rec RunRecord
	var startedAt int64
	var finishedAt sql.NullInt64
	var resultBytes []byte
	var provenanceBytes []byte
	var executor sql.NullString
	var runtime sql.NullString
	var securityProfile sql.NullString
	var requestID sql.NullString
	if err := row.Scan(&rec.ID, &rec.JobID, &rec.Status, &startedAt, &finishedAt, &resultBytes, &executor, &runtime, &securityProfile, &provenanceBytes, &requestID); errors.Is(err, sql.ErrNoRows) {
		return RunRecord{}, false, nil
	} else if err != nil {
		return RunRecord{}, false, err
	}
	if startedAt > 0 {
		rec.StartedAt = time.UnixMilli(startedAt)
	}
	if finishedAt.Valid {
		stamp := time.UnixMilli(finishedAt.Int64)
		rec.FinishedAt = &stamp
	}
	if executor.Valid {
		rec.Executor = executor.String
	}
	if runtime.Valid {
		rec.Runtime = runtime.String
	}
	if securityProfile.Valid {
		rec.SecurityProfile = securityProfile.String
	}
	if requestID.Valid {
		rec.RequestID = requestID.String
	}
	if len(resultBytes) > 0 {
		var result map[string]any
		if err := json.Unmarshal(resultBytes, &result); err == nil {
			rec.Result = result
		}
	}
	if len(provenanceBytes) > 0 {
		var provenance map[string]any
		if err := json.Unmarshal(provenanceBytes, &provenance); err == nil {
			rec.Provenance = provenance
		}
	}
	return rec, true, nil
}

// List returns runs sorted by started_at descending.
func (s *RunStore) List(ctx context.Context) ([]RunRecord, error) {
	if s == nil {
		return nil, nil
	}
	rows, err := s.db.QueryContext(ctx, `SELECT run_id, job_id, status, started_at, finished_at, result, executor, runtime, security_profile, provenance, request_id FROM core_runs ORDER BY started_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []RunRecord
	for rows.Next() {
		var rec RunRecord
		var startedAt int64
		var finishedAt sql.NullInt64
		var resultBytes []byte
		var provenanceBytes []byte
		var executor sql.NullString
		var runtime sql.NullString
		var securityProfile sql.NullString
		var requestID sql.NullString
		if err := rows.Scan(&rec.ID, &rec.JobID, &rec.Status, &startedAt, &finishedAt, &resultBytes, &executor, &runtime, &securityProfile, &provenanceBytes, &requestID); err != nil {
			return nil, err
		}
		if startedAt > 0 {
			rec.StartedAt = time.UnixMilli(startedAt)
		}
		if finishedAt.Valid {
			stamp := time.UnixMilli(finishedAt.Int64)
			rec.FinishedAt = &stamp
		}
		if executor.Valid {
			rec.Executor = executor.String
		}
		if runtime.Valid {
			rec.Runtime = runtime.String
		}
		if securityProfile.Valid {
			rec.SecurityProfile = securityProfile.String
		}
		if requestID.Valid {
			rec.RequestID = requestID.String
		}
		if len(resultBytes) > 0 {
			var result map[string]any
			if err := json.Unmarshal(resultBytes, &result); err == nil {
				rec.Result = result
			}
		}
		if len(provenanceBytes) > 0 {
			var provenance map[string]any
			if err := json.Unmarshal(provenanceBytes, &provenance); err == nil {
				rec.Provenance = provenance
			}
		}
		out = append(out, rec)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *RunStore) upsert(ctx context.Context, record RunRecord) error {
	if s == nil {
		return nil
	}
	resultBytes, err := encodeJSONMap(record.Result)
	if err != nil {
		return err
	}
	provenanceBytes, err := encodeJSONMap(record.Provenance)
	if err != nil {
		return err
	}
	var finishedAt any
	if record.FinishedAt != nil {
		finishedAt = record.FinishedAt.UnixMilli()
	}
	_, err = s.db.ExecContext(ctx, `
INSERT INTO core_runs (run_id, job_id, status, started_at, finished_at, result, executor, runtime, security_profile, provenance, request_id)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(run_id) DO UPDATE SET
  job_id = excluded.job_id,
  status = excluded.status,
  started_at = excluded.started_at,
  finished_at = excluded.finished_at,
  result = excluded.result,
  executor = excluded.executor,
  runtime = excluded.runtime,
  security_profile = excluded.security_profile,
  provenance = excluded.provenance,
  request_id = excluded.request_id;
`, record.ID, record.JobID, record.Status, record.StartedAt.UnixMilli(), finishedAt, resultBytes, record.Executor, record.Runtime, record.SecurityProfile, provenanceBytes, record.RequestID)
	return err
}

func encodeJSONMap(value map[string]any) ([]byte, error) {
	if len(value) == 0 {
		return nil, nil
	}
	return json.Marshal(value)
}
