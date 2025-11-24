// SC-REF: SC701 (Phase 7 — Alias usability in listings)
// SC-REF: SC704 (Phase 7 — Policy-gated alias visibility)
// Non-functional traceability tags for reviewer mapping.
package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/flowd-org/flowd/internal/indexer"
	"github.com/flowd-org/flowd/internal/server/headers"
	"github.com/flowd-org/flowd/internal/server/sourcestore"
)

func TestJobsHandlerIncludesOCIJobs(t *testing.T) {
	store := sourcestore.New()
	tempDir := t.TempDir()
	manifestPath := filepath.Join(tempDir, "manifest.yaml")
	if err := os.WriteFile(manifestPath, []byte(`
apiVersion: flwd.addon/v1
kind: AddOn
metadata:
  name: Example Addon
  id: example.addon
  version: 1.0.0
requires: {}
jobs:
  - id: build
    name: Build Project
    summary: Compile the project
    argspec:
      args: []
`), 0o600); err != nil {
		t.Fatalf("write manifest: %v", err)
	}

	store.Upsert(sourcestore.Source{
		Name:      "addon",
		Type:      "oci",
		LocalPath: tempDir,
		Metadata: map[string]any{
			"manifest_path": manifestPath,
		},
	})

	handler := NewJobsHandler(JobsConfig{
		Root:    filepath.Join(t.TempDir(), "scripts"),
		Sources: store,
		Discover: func(string) (indexer.Result, error) {
			return indexer.Result{}, nil
		},
	})

	req := httptest.NewRequest(http.MethodGet, "/jobs", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	var jobs []jobView
	if err := json.NewDecoder(rec.Body).Decode(&jobs); err != nil {
		t.Fatalf("decode jobs: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("expected one job, got %d", len(jobs))
	}
	job := jobs[0]
	if job.ID != "addon/build" {
		t.Fatalf("expected job ID addon/build, got %s", job.ID)
	}
	if job.Source == nil || job.Source.Name != "addon" || job.Source.Type != "oci" {
		t.Fatalf("expected source metadata, got %+v", job.Source)
	}
	if rec.Header().Get(headers.DiscoveryErrors) != "0" {
		t.Fatalf("expected discovery errors header 0, got %s", rec.Header().Get(headers.DiscoveryErrors))
	}
}

func TestJobsHandlerOCIManifestErrorCounts(t *testing.T) {
	store := sourcestore.New()
	missingDir := t.TempDir()
	manifestPath := filepath.Join(missingDir, "missing.yaml")
	store.Upsert(sourcestore.Source{
		Name:      "broken",
		Type:      "oci",
		LocalPath: missingDir,
		Metadata: map[string]any{
			"manifest_path": manifestPath,
		},
	})

	handler := NewJobsHandler(JobsConfig{
		Sources: store,
		Discover: func(string) (indexer.Result, error) {
			return indexer.Result{}, nil
		},
	})

	req := httptest.NewRequest(http.MethodGet, "/jobs", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if rec.Header().Get(headers.DiscoveryErrors) != "1" {
		t.Fatalf("expected discovery errors header set to 1, got %s", rec.Header().Get(headers.DiscoveryErrors))
	}
}

func TestJobsHandlerAliasVisibilityPolicy(t *testing.T) {
	root := t.TempDir()
	aliasConfig := []byte(`aliases:
- from: demo
  to: hello-demo
  description: Friendly demo alias
`)
	if err := os.WriteFile(filepath.Join(root, "flwd.yaml"), aliasConfig, 0o600); err != nil {
		t.Fatalf("write flwd.yaml: %v", err)
	}

	discover := func(string) (indexer.Result, error) {
		return indexer.Result{
			Jobs: []indexer.JobInfo{{ID: "demo", Name: "Demo"}},
		}, nil
	}

	hidden := NewJobsHandler(JobsConfig{
		Root:          root,
		Discover:      discover,
		ExposeAliases: func(*http.Request) bool { return false },
	})
	req := httptest.NewRequest(http.MethodGet, "/jobs", nil)
	rec := httptest.NewRecorder()
	hidden.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 when aliases hidden, got %d: %s", rec.Code, rec.Body.String())
	}
	var jobs []jobView
	if err := json.NewDecoder(rec.Body).Decode(&jobs); err != nil {
		t.Fatalf("decode hidden jobs: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("expected only canonical job when aliases hidden, got %d", len(jobs))
	}
	if jobs[0].ID != "demo" {
		t.Fatalf("expected job id demo, got %s", jobs[0].ID)
	}

	visible := NewJobsHandler(JobsConfig{
		Root:          root,
		Discover:      discover,
		ExposeAliases: func(*http.Request) bool { return true },
	})
	req = httptest.NewRequest(http.MethodGet, "/jobs", nil)
	rec = httptest.NewRecorder()
	visible.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 when aliases visible, got %d: %s", rec.Code, rec.Body.String())
	}
	jobs = nil
	if err := json.NewDecoder(rec.Body).Decode(&jobs); err != nil {
		t.Fatalf("decode visible jobs: %v", err)
	}
	if len(jobs) != 2 {
		t.Fatalf("expected canonical job plus alias, got %d", len(jobs))
	}
	foundAlias := false
	for _, job := range jobs {
		if job.ID == "hello-demo" {
			foundAlias = true
			if job.AliasOf != "demo" {
				t.Fatalf("expected alias_of demo, got %s", job.AliasOf)
			}
		}
	}
	if !foundAlias {
		t.Fatalf("expected alias entry hello-demo in job list: %#v", jobs)
	}
}
