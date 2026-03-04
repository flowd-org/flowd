// SPDX-License-Identifier: AGPL-3.0-or-later

package coredb

import (
	"context"
	"testing"
	"time"
)

func TestNewRunStore_NilDB(t *testing.T) {
	store := NewRunStore(nil)
	if store != nil {
		t.Fatalf("expected nil store for nil DB, got %v", store)
	}
}

func TestRunStore_Get_NotFound(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	s := NewRunStore(db)

	_, ok, err := s.Get(ctx, "missing")
	if ok {
		t.Fatalf("expected ok==false for missing run, got true")
	}
	if err != nil {
		t.Fatalf("expected err==nil for missing run, got %v", err)
	}
}

func TestRunStore_CreateGetRoundTrip_WithMaps(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	s := NewRunStore(db)

	fixedTime := time.Unix(1704067200, 0) // 2024-01-01 00:00:00 UTC

	record := RunRecord{
		ID:              "run-001",
		JobID:           "job-001",
		Status:          "running",
		StartedAt:       fixedTime,
		Result:          map[string]any{"foo": "bar", "num": 42},
		Executor:        "local",
		Runtime:         "python3",
		SecurityProfile: "default",
		Provenance:      map[string]any{"source": "api"},
		RequestID:       "req-001",
	}

	if err := s.Create(ctx, record); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	got, ok, err := s.Get(ctx, "run-001")
	if !ok {
		t.Fatalf("expected ok==true after Create, got false")
	}
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	if got.ID != record.ID {
		t.Errorf("ID mismatch: got %q, want %q", got.ID, record.ID)
	}
	if got.JobID != record.JobID {
		t.Errorf("JobID mismatch: got %q, want %q", got.JobID, record.JobID)
	}
	if got.Status != record.Status {
		t.Errorf("Status mismatch: got %q, want %q", got.Status, record.Status)
	}
	if !got.StartedAt.Equal(record.StartedAt) {
		t.Errorf("StartedAt mismatch: got %v, want %v", got.StartedAt, record.StartedAt)
	}
	if got.Executor != record.Executor {
		t.Errorf("Executor mismatch: got %q, want %q", got.Executor, record.Executor)
	}
	if got.Runtime != record.Runtime {
		t.Errorf("Runtime mismatch: got %q, want %q", got.Runtime, record.Runtime)
	}
	if got.SecurityProfile != record.SecurityProfile {
		t.Errorf("SecurityProfile mismatch: got %q, want %q", got.SecurityProfile, record.SecurityProfile)
	}
	if got.RequestID != record.RequestID {
		t.Errorf("RequestID mismatch: got %q, want %q", got.RequestID, record.RequestID)
	}

	// Check Result map
	if got.Result == nil {
		t.Fatalf("Result is nil, expected populated map")
	}
	if v, ok := got.Result["foo"]; !ok || v != "bar" {
		t.Errorf("Result[foo] mismatch: got %v (type %T), want bar", v, v)
	}
	// JSON unmarshaling converts numbers to float64
	if v, ok := got.Result["num"]; !ok || v != 42.0 {
		t.Errorf("Result[num] mismatch: got %v (type %T), want 42.0", v, v)
	}

	// Check Provenance map
	if got.Provenance == nil {
		t.Fatalf("Provenance is nil, expected populated map")
	}
	if v, ok := got.Provenance["source"]; !ok || v != "api" {
		t.Errorf("Provenance[source] mismatch: got %v (type %T), want api", v, v)
	}
}

func TestRunStore_Update_OverwritesFields(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	s := NewRunStore(db)

	fixedTime1 := time.Unix(1704067200, 0) // 2024-01-01 00:00:00 UTC
	fixedTime2 := time.Unix(1704153600, 0) // 2024-01-02 00:00:00 UTC

	// Create initial record
	record := RunRecord{
		ID:              "run-002",
		JobID:           "job-002",
		Status:          "running",
		StartedAt:       fixedTime1,
		Result:          map[string]any{"initial": true},
		Executor:        "local",
		Runtime:         "python3",
		SecurityProfile: "default",
		Provenance:      map[string]any{"source": "api"},
		RequestID:       "req-002",
	}

	if err := s.Create(ctx, record); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// Update the record
	updatedRecord := RunRecord{
		ID:              "run-002",
		JobID:           "job-002-updated",
		Status:          "completed",
		StartedAt:       fixedTime1,
		FinishedAt:      &fixedTime2,
		Result:          map[string]any{"updated": true, "new_field": 123},
		Executor:        "k8s",
		Runtime:         "python3.11",
		SecurityProfile: "restricted",
		Provenance:      map[string]any{"source": "scheduler"},
		RequestID:       "req-002-updated",
	}

	if err := s.Update(ctx, updatedRecord); err != nil {
		t.Fatalf("Update failed: %v", err)
	}

	got, ok, err := s.Get(ctx, "run-002")
	if !ok {
		t.Fatalf("expected ok==true after Update, got false")
	}
	if err != nil {
		t.Fatalf("Get after update failed: %v", err)
	}

	// Verify all fields were overwritten
	if got.JobID != updatedRecord.JobID {
		t.Errorf("JobID mismatch after update: got %q, want %q", got.JobID, updatedRecord.JobID)
	}
	if got.Status != updatedRecord.Status {
		t.Errorf("Status mismatch after update: got %q, want %q", got.Status, updatedRecord.Status)
	}
	if got.Executor != updatedRecord.Executor {
		t.Errorf("Executor mismatch after update: got %q, want %q", got.Executor, updatedRecord.Executor)
	}
	if got.Runtime != updatedRecord.Runtime {
		t.Errorf("Runtime mismatch after update: got %q, want %q", got.Runtime, updatedRecord.Runtime)
	}
	if got.SecurityProfile != updatedRecord.SecurityProfile {
		t.Errorf("SecurityProfile mismatch after update: got %q, want %q", got.SecurityProfile, updatedRecord.SecurityProfile)
	}
	if got.RequestID != updatedRecord.RequestID {
		t.Errorf("RequestID mismatch after update: got %q, want %q", got.RequestID, updatedRecord.RequestID)
	}

	// Verify FinishedAt was set
	if got.FinishedAt == nil {
		t.Fatalf("FinishedAt is nil after update, expected non-nil")
	}
	if !got.FinishedAt.Equal(*updatedRecord.FinishedAt) {
		t.Errorf("FinishedAt mismatch: got %v, want %v", *got.FinishedAt, *updatedRecord.FinishedAt)
	}

	// Verify Result map was overwritten
	if v, ok := got.Result["initial"]; ok && v == true {
		t.Errorf("Result still contains old key 'initial', expected to be overwritten")
	}
	// JSON unmarshaling converts numbers to float64
	if v, ok := got.Result["new_field"]; !ok || v != 123.0 {
		t.Errorf("Result[new_field] mismatch: got %v (type %T), want 123.0", v, v)
	}

	// Verify Provenance map was overwritten
	if v, ok := got.Provenance["source"]; !ok || v != "scheduler" {
		t.Errorf("Provenance[source] mismatch: got %v (type %T), want scheduler", v, v)
	}
}

func TestRunStore_List_ReturnsAll(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	s := NewRunStore(db)

	fixedTime1 := time.Unix(1704067200, 0) // 2024-01-01 00:00:00 UTC
	fixedTime2 := time.Unix(1704153600, 0) // 2024-01-02 00:00:00 UTC

	// Create two records
	record1 := RunRecord{
		ID:        "run-003",
		JobID:     "job-003",
		Status:    "running",
		StartedAt: fixedTime1,
		Result:    map[string]any{"idx": 1},
		Executor:  "local",
	}

	record2 := RunRecord{
		ID:        "run-004",
		JobID:     "job-004",
		Status:    "completed",
		StartedAt: fixedTime2,
		Result:    map[string]any{"idx": 2},
		Executor:  "k8s",
	}

	if err := s.Create(ctx, record1); err != nil {
		t.Fatalf("Create record1 failed: %v", err)
	}
	if err := s.Create(ctx, record2); err != nil {
		t.Fatalf("Create record2 failed: %v", err)
	}

	// List all runs
	listed, err := s.List(ctx)
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}

	if len(listed) != 2 {
		t.Fatalf("expected 2 runs, got %d", len(listed))
	}

	// Verify IDs (List returns sorted by started_at DESC)
	ids := make([]string, len(listed))
	for i, rec := range listed {
		ids[i] = rec.ID
	}

	// run-004 has later timestamp, should come first
	if ids[0] != "run-004" || ids[1] != "run-003" {
		t.Errorf("List order mismatch: got %v, want [run-004, run-003]", ids)
	}

	// Verify we can find both records
	found := make(map[string]bool)
	for _, rec := range listed {
		if rec.ID == "run-003" || rec.ID == "run-004" {
			found[rec.ID] = true
		}
	}
	if !found["run-003"] {
		t.Errorf("run-003 not found in List result")
	}
	if !found["run-004"] {
		t.Errorf("run-004 not found in List result")
	}
}

func TestRunStore_List_WithEmptyMaps(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	s := NewRunStore(db)

	fixedTime1 := time.Unix(1704067200, 0) // 2024-01-01 00:00:00 UTC

	// Create a record with empty Result and Provenance maps
	record := RunRecord{
		ID:              "run-005",
		JobID:           "job-005",
		Status:          "running",
		StartedAt:       fixedTime1,
		Result:          map[string]any{}, // Empty map
		Executor:        "local",
		Runtime:         "python3",
		SecurityProfile: "default",
		Provenance:      map[string]any{}, // Empty map
		RequestID:       "req-005",
	}

	if err := s.Create(ctx, record); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// List should handle empty maps correctly
	listed, err := s.List(ctx)
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}

	if len(listed) != 1 {
		t.Fatalf("expected 1 run, got %d", len(listed))
	}

	got := listed[0]
	if got.ID != "run-005" {
		t.Errorf("ID mismatch: got %q, want run-005", got.ID)
	}
	if got.Executor != "local" {
		t.Errorf("Executor mismatch: got %q, want local", got.Executor)
	}
	// Empty maps should be nil after round-trip (encodeJSONMap returns nil for empty maps)
	if got.Result != nil {
		t.Errorf("Result should be nil for empty map, got %v", got.Result)
	}
	if got.Provenance != nil {
		t.Errorf("Provenance should be nil for empty map, got %v", got.Provenance)
	}
}

func TestRunStore_List_WithNilValues(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	s := NewRunStore(db)

	fixedTime1 := time.Unix(1704067200, 0) // 2024-01-01 00:00:00 UTC

	// Create a record with nil Result and Provenance
	record := RunRecord{
		ID:        "run-006",
		JobID:     "job-006",
		Status:    "running",
		StartedAt: fixedTime1,
		Result:    nil, // Explicitly nil
		Executor:  "local",
	}

	if err := s.Create(ctx, record); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	// List should handle nil maps correctly
	listed, err := s.List(ctx)
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}

	if len(listed) != 1 {
		t.Fatalf("expected 1 run, got %d", len(listed))
	}

	got := listed[0]
	if got.ID != "run-006" {
		t.Errorf("ID mismatch: got %q, want run-006", got.ID)
	}
	// Nil maps should remain nil after round-trip
	if got.Result != nil {
		t.Errorf("Result should be nil for nil input, got %v", got.Result)
	}
	if got.Provenance != nil {
		t.Errorf("Provenance should be nil for nil input, got %v", got.Provenance)
	}
}

func TestRunStore_List_EmptyTable(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	s := NewRunStore(db)

	// List from empty table
	listed, err := s.List(ctx)
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}

	if listed != nil {
		t.Fatalf("expected nil for empty table, got %v", listed)
	}
}

// TestRunStore_NilStoreMethods tests that methods on nil RunStore return safe defaults.
func TestRunStore_NilStoreMethods(t *testing.T) {
	var s *RunStore
	ctx := context.Background()

	// Get on nil should return (Record{}, false, nil)
	_, ok, err := s.Get(ctx, "any")
	if ok || err != nil {
		t.Errorf("Get on nil store: expected (record{}, false, nil), got (%v, %v)", ok, err)
	}

	// List on nil should return (nil, nil)
	listed, err := s.List(ctx)
	if listed != nil || err != nil {
		t.Errorf("List on nil store: expected (nil, nil), got (%v, %v)", listed, err)
	}
}

// TestRunStore_NilStoreUpsert tests that upsert on nil RunStore returns nil error.
func TestRunStore_NilStoreUpsert(t *testing.T) {
	var s *RunStore
	ctx := context.Background()

	fixedTime := time.Unix(1704067200, 0)
	record := RunRecord{
		ID:        "run-nil",
		JobID:     "job-nil",
		Status:    "running",
		StartedAt: fixedTime,
		Result:    map[string]any{"test": true},
		Executor:  "local",
	}

	err := s.upsert(ctx, record)
	if err != nil {
		t.Errorf("upsert on nil store should return nil error, got %v", err)
	}
}
