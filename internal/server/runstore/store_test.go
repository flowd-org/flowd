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
