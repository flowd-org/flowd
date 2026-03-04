package sourcestore

import (
	"sort"
	"testing"
)

func TestStoreUpsertAndGet(t *testing.T) {
	store := New()
	src := Source{Name: "demo", Type: "local"}
	created := store.Upsert(src)
	if !created {
		t.Fatalf("expected first upsert to report created")
	}
	got, ok := store.Get("demo")
	if !ok {
		t.Fatalf("expected source to exist")
	}
	if got.Name != "demo" || got.Type != "local" {
		t.Fatalf("unexpected source: %#v", got)
	}

	srcUpdated := Source{Name: "demo", Type: "git", Ref: "https://example/repo.git"}
	if created := store.Upsert(srcUpdated); created {
		t.Fatalf("expected update to report existing")
	}
	got, ok = store.Get("demo")
	if !ok {
		t.Fatalf("expected updated source to exist")
	}
	if got.Type != "git" {
		t.Fatalf("expected updated type git, got %s", got.Type)
	}
}

func TestStoreDelete(t *testing.T) {
	store := New()
	store.Upsert(Source{Name: "demo", Type: "local"})
	if deleted := store.Delete("demo"); !deleted {
		t.Fatalf("expected delete to report removal")
	}
	if _, ok := store.Get("demo"); ok {
		t.Fatalf("expected source to be removed")
	}
	if deleted := store.Delete("demo"); deleted {
		t.Fatalf("expected deleting non-existent source to return false")
	}
}

func TestStoreList(t *testing.T) {
	store := New()

	// Insert sources in non-alphabetical order
	srcs := []Source{
		{Name: "zebra", Type: "local"},
		{Name: "alpha", Type: "git"},
		{Name: "beta", Type: "http"},
	}

	for _, src := range srcs {
		store.Upsert(src)
	}

	listed := store.List()
	if len(listed) != len(srcs) {
		t.Fatalf("expected %d sources, got %d", len(srcs), len(listed))
	}

	// Verify ordering by name
	keys := make([]string, 0, len(listed))
	for _, src := range listed {
		keys = append(keys, src.Name)
	}
	sort.Strings(keys)

	for i, src := range listed {
		if src.Name != keys[i] {
			t.Fatalf("expected source %d to be %s, got %s", i, keys[i], src.Name)
		}
	}

	// Verify content matches by comparing fields individually (maps are not comparable)
	expected := map[string]Source{
		"alpha": {Name: "alpha", Type: "git"},
		"beta":  {Name: "beta", Type: "http"},
		"zebra": {Name: "zebra", Type: "local"},
	}
	for i, src := range listed {
		exp := expected[src.Name]
		if src.Name != exp.Name || src.Type != exp.Type {
			t.Fatalf("source %d mismatch: got (%s,%s), expected (%s,%s)", i, src.Name, src.Type, exp.Name, exp.Type)
		}
	}
}

func TestStoreUpsertCreateFlag(t *testing.T) {
	store := New()

	// First upsert should return true (created)
	if created := store.Upsert(Source{Name: "src1", Type: "local"}); !created {
		t.Fatalf("expected first upsert to report created")
	}

	// Second upsert of same key should return false (updated)
	if created := store.Upsert(Source{Name: "src1", Type: "git"}); created {
		t.Fatalf("expected second upsert to report existing")
	}
}

func TestStoreDeleteMissingKey(t *testing.T) {
	store := New()

	// Delete on empty store should return false
	if deleted := store.Delete("nonexistent"); deleted {
		t.Fatalf("expected delete of missing key to return false")
	}

	// After inserting and deleting, delete again should still return false
	store.Upsert(Source{Name: "temp", Type: "local"})
	store.Delete("temp")
	if deleted := store.Delete("temp"); deleted {
		t.Fatalf("expected delete after removal to return false")
	}
}
