package runstore

import (
	"testing"
	"time"
)

func TestStoreCreateGetList(t *testing.T) {
	store := New()
	now := time.Now()
	run1 := Run{ID: "r1", JobID: "jobA", Status: "queued", StartedAt: now}
	run2 := Run{ID: "r2", JobID: "jobB", Status: "queued", StartedAt: now.Add(1 * time.Minute)}

	store.Create(run1)
	store.Create(run2)

	if got, ok := store.Get("r1"); !ok || got.JobID != "jobA" {
		t.Fatalf("expected run r1 jobA, got %+v, ok=%v", got, ok)
	}

	list := store.List()
	if len(list) != 2 {
		t.Fatalf("expected 2 runs, got %d", len(list))
	}
	if list[0].ID != "r2" {
		t.Fatalf("expected newest run first, got %s", list[0].ID)
	}
}

func TestStoreUpdate(t *testing.T) {
	store := New()
	now := time.Now()
	run1 := Run{ID: "r1", JobID: "jobA", Status: "queued", StartedAt: now}

	store.Create(run1)

	// Update the run
	run1.Status = "running"
	store.Update(run1)

	if got, ok := store.Get("r1"); !ok || got.Status != "running" {
		t.Fatalf("expected run r1 status running after update, got %+v, ok=%v", got, ok)
	}

	// Update with a new run (should replace if ID matches)
	run2 := Run{ID: "r2", JobID: "jobB", Status: "pending", StartedAt: now.Add(1 * time.Minute)}
	store.Update(run2)

	if got, ok := store.Get("r2"); !ok || got.JobID != "jobB" {
		t.Fatalf("expected run r2 jobB after update, got %+v, ok=%v", got, ok)
	}
}

func TestStoreGetMissing(t *testing.T) {
	store := New()

	run, ok := store.Get("nonexistent")
	if ok {
		t.Fatalf("expected ok=false for missing key, got ok=true")
	}
	if run.ID != "" {
		t.Fatalf("expected empty Run for missing key, got %+v", run)
	}
}

func TestStoreListSnapshot(t *testing.T) {
	store := New()
	now := time.Now()
	run1 := Run{ID: "r1", JobID: "jobA", Status: "queued", StartedAt: now}
	run2 := Run{ID: "r2", JobID: "jobB", Status: "pending", StartedAt: now.Add(1 * time.Minute)}

	store.Create(run1)
	store.Create(run2)

	list := store.List()
	if len(list) != 2 {
		t.Fatalf("expected 2 runs, got %d", len(list))
	}

	// Mutate the returned slice
	mutatedRun := list[0]
	originalJobID := mutatedRun.JobID
	mutatedRun.JobID = "modified"

	// Verify original store is unchanged for the mutated run
	run, ok := store.Get(mutatedRun.ID)
	if !ok || run.ID != mutatedRun.ID || run.JobID != originalJobID {
		t.Fatalf("expected %s to remain unchanged, got %+v, ok=%v", mutatedRun.ID, run, ok)
	}
}

func TestStoreGetAfterUpdate(t *testing.T) {
	store := New()
	now := time.Now()

	run := Run{ID: "r1", JobID: "jobA", Status: "queued", StartedAt: now}
	store.Create(run)

	// Verify initial state
	if got, ok := store.Get("r1"); !ok || got.Status != "queued" {
		t.Fatalf("expected run r1 status queued, got %+v, ok=%v", got, ok)
	}

	// Update and verify
	run.Status = "running"
	store.Update(run)

	if got, ok := store.Get("r1"); !ok || got.Status != "running" {
		t.Fatalf("expected run r1 status running after update, got %+v, ok=%v", got, ok)
	}
}

func TestStoreListEmpty(t *testing.T) {
	store := New()

	list := store.List()
	if len(list) != 0 {
		t.Fatalf("expected empty list, got %d items", len(list))
	}
}

func TestStoreMultipleUpdates(t *testing.T) {
	store := New()
	now := time.Now()

	run := Run{ID: "r1", JobID: "jobA", Status: "queued", StartedAt: now}
	store.Create(run)

	// First update
	run.Status = "running"
	store.Update(run)

	// Second update
	run.Status = "completed"
	store.Update(run)

	if got, ok := store.Get("r1"); !ok || got.Status != "completed" {
		t.Fatalf("expected run r1 status completed after multiple updates, got %+v, ok=%v", got, ok)
	}
}
