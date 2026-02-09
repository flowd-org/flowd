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
	"strings"
	"testing"

	"github.com/flowd-org/flowd/internal/configloader"
	"github.com/flowd-org/flowd/internal/indexer"
	"github.com/flowd-org/flowd/internal/server/headers"
	"github.com/flowd-org/flowd/internal/server/response"
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

func TestJobsHandlerDualConfigProblem(t *testing.T) {
	handler := NewJobsHandler(JobsConfig{
		Discover: func(string) (indexer.Result, error) {
			return indexer.Result{}, &configloader.DualConfigError{
				ScriptDir:   "/tmp/scripts/demo",
				PrimaryPath: "/tmp/scripts/demo/config.yaml",
				LegacyPath:  "/tmp/scripts/demo/config.d/config.yaml",
			}
		},
	})

	req := httptest.NewRequest(http.MethodGet, "/jobs", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d: %s", rec.Code, rec.Body.String())
	}
	contentType := rec.Header().Get("Content-Type")
	if !strings.HasPrefix(contentType, "application/problem+json") {
		t.Fatalf("expected problem+json content type, got %s", contentType)
	}

	var prob map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&prob); err != nil {
		t.Fatalf("decode problem: %v", err)
	}
	if prob["code"] != configloader.DualConfigCode {
		t.Fatalf("expected code %q, got %+v", configloader.DualConfigCode, prob["code"])
	}
	if prob["type"] != response.ProblemTypeJobConfigDualSentinel {
		t.Fatalf("expected type %q, got %+v", response.ProblemTypeJobConfigDualSentinel, prob["type"])
	}
}

func TestJobsHandlerInvalidJobIDProblem(t *testing.T) {
	root := t.TempDir()
	jobDir := filepath.Join(root, "!!!")
	if err := os.MkdirAll(jobDir, 0o755); err != nil {
		t.Fatalf("mkdir job dir: %v", err)
	}
	config := `version: v1
job:
  name: Bad Job
`
	if err := os.WriteFile(filepath.Join(jobDir, "config.yaml"), []byte(config), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	handler := NewJobsHandler(JobsConfig{Root: root})
	req := httptest.NewRequest(http.MethodGet, "/jobs", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d: %s", rec.Code, rec.Body.String())
	}
	contentType := rec.Header().Get("Content-Type")
	if !strings.HasPrefix(contentType, "application/problem+json") {
		t.Fatalf("expected problem+json content type, got %s", contentType)
	}
	var prob map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&prob); err != nil {
		t.Fatalf("decode problem: %v", err)
	}
	if prob["code"] != response.ProblemCodeJobIDInvalidSegment {
		t.Fatalf("expected code %q, got %+v", response.ProblemCodeJobIDInvalidSegment, prob["code"])
	}
	if prob["type"] != response.ProblemTypeJobIDInvalidSegment {
		t.Fatalf("expected type %q, got %+v", response.ProblemTypeJobIDInvalidSegment, prob["type"])
	}
	if seg, ok := prob["segment"].(string); !ok || seg == "" {
		t.Fatalf("expected segment string, got %+v", prob["segment"])
	}
	if path, ok := prob["path"].(string); !ok || path == "" {
		t.Fatalf("expected path string, got %+v", prob["path"])
	}
}

func TestJobsHandlerCollisionDeterministicOrder(t *testing.T) {
	root := t.TempDir()
	config := `version: v1
job:
  name: Demo
`

	remoteRoot := t.TempDir()
	remoteBase := filepath.Join(remoteRoot, "demo")
	if err := os.MkdirAll(remoteBase, 0o755); err != nil {
		t.Fatalf("mkdir remote base: %v", err)
	}
	if err := os.WriteFile(filepath.Join(remoteBase, "config.yaml"), []byte(config), 0o644); err != nil {
		t.Fatalf("write remote base config: %v", err)
	}

	remoteAlt := filepath.Join(remoteRoot, "demo!")
	if err := os.MkdirAll(remoteAlt, 0o755); err != nil {
		t.Fatalf("mkdir remote alt: %v", err)
	}
	if err := os.WriteFile(filepath.Join(remoteAlt, "config.yaml"), []byte(config), 0o644); err != nil {
		t.Fatalf("write remote alt config: %v", err)
	}

	store := sourcestore.New()
	store.Upsert(sourcestore.Source{
		Name:      "alpha",
		Type:      "git",
		LocalPath: remoteRoot,
	})

	handler := NewJobsHandler(JobsConfig{
		Root:    root,
		Sources: store,
	})

	req := httptest.NewRequest(http.MethodGet, "/jobs", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d: %s", rec.Code, rec.Body.String())
	}
	contentType := rec.Header().Get("Content-Type")
	if !strings.HasPrefix(contentType, "application/problem+json") {
		t.Fatalf("expected problem+json content type, got %s", contentType)
	}

	var prob map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&prob); err != nil {
		t.Fatalf("decode problem: %v", err)
	}
	if prob["code"] != response.ProblemCodeJobIDCollision {
		t.Fatalf("expected code %q, got %+v", response.ProblemCodeJobIDCollision, prob["code"])
	}
	expectedID, err := indexer.CanonicalJobID("alpha", "demo")
	if err != nil {
		t.Fatalf("canonical id: %v", err)
	}
	if prob["canonical_job_id"] != expectedID {
		t.Fatalf("expected canonical_job_id %s, got %+v", expectedID, prob["canonical_job_id"])
	}
	contendersRaw, ok := prob["contenders"].([]any)
	if !ok {
		t.Fatalf("expected contenders array, got %+v", prob["contenders"])
	}
	if len(contendersRaw) != 2 {
		t.Fatalf("expected 2 contenders, got %d", len(contendersRaw))
	}
	first, ok := contendersRaw[0].(map[string]any)
	if !ok {
		t.Fatalf("expected contender map, got %+v", contendersRaw[0])
	}
	second, ok := contendersRaw[1].(map[string]any)
	if !ok {
		t.Fatalf("expected contender map, got %+v", contendersRaw[1])
	}
	firstOrigin, ok := first["origin"].(map[string]any)
	if !ok {
		t.Fatalf("expected origin map, got %+v", first["origin"])
	}
	secondOrigin, ok := second["origin"].(map[string]any)
	if !ok {
		t.Fatalf("expected origin map, got %+v", second["origin"])
	}
	if firstOrigin["source_kind"] != "git" || firstOrigin["source_name"] != "alpha" {
		t.Fatalf("expected git contender first, got %+v", firstOrigin)
	}
	if secondOrigin["source_kind"] != "git" || secondOrigin["source_name"] != "alpha" {
		t.Fatalf("expected git contender second, got %+v", secondOrigin)
	}
	if first["mountPath"] != remoteRoot || second["mountPath"] != remoteRoot {
		t.Fatalf("expected contenders mountPath %s, got %+v and %+v", remoteRoot, first["mountPath"], second["mountPath"])
	}
	if first["job_dir"] != "demo" || second["job_dir"] != "demo!" {
		t.Fatalf("expected deterministic job_dir order demo then demo!, got %+v and %+v", first["job_dir"], second["job_dir"])
	}
}
