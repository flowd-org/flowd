package sourcestore

import "testing"

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
