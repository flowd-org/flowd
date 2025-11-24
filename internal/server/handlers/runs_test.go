package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/flowd-org/flowd/internal/coredb"
	"github.com/flowd-org/flowd/internal/engine"
	"github.com/flowd-org/flowd/internal/executor/container"
	"github.com/flowd-org/flowd/internal/indexer"
	"github.com/flowd-org/flowd/internal/paths"
	"github.com/flowd-org/flowd/internal/policy"
	"github.com/flowd-org/flowd/internal/policy/verify"
	"github.com/flowd-org/flowd/internal/server/requestctx"
	"github.com/flowd-org/flowd/internal/server/runstore"
	"github.com/flowd-org/flowd/internal/server/sourcestore"
	"github.com/flowd-org/flowd/internal/server/sse"
)

var idempotencySeq uint64

func newIdempotencyKey() string {
	seq := atomic.AddUint64(&idempotencySeq, 1)
	return fmt.Sprintf("idem-key-%012d", seq)
}

func addIdempotencyHeader(req *http.Request) string {
	key := newIdempotencyKey()
	req.Header.Set("Idempotency-Key", key)
	return key
}

func setSpecificIdempotencyKey(req *http.Request, key string) {
	req.Header.Set("Idempotency-Key", key)
}

func TestMain(m *testing.M) {
	tmp, err := os.MkdirTemp("", "flowd-runs-test-")
	if err != nil {
		panic(err)
	}
	paths.SetDataDirOverride(tmp)
	code := m.Run()
	paths.SetDataDirOverride("")
	_ = os.RemoveAll(tmp)
	os.Exit(code)
}

func TestRunsHandlerSuccess(t *testing.T) {
	root := t.TempDir()
	writeJobConfig(t, root, "demo", `
version: v1
job:
  id: demo
  name: Demo Job
argspec:
  args:
    - name: name
      type: string
      required: true
`)

	store := runstore.New()
	fixed := time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC)
	h := NewRunsHandler(RunsConfig{Root: root, Now: func() time.Time { return fixed }, Store: store})

	req := httptest.NewRequest(http.MethodPost, "/runs", strings.NewReader(`{"job_id":"demo","args":{"name":"Alice"}}`))
	req.Header.Set("Content-Type", "application/json")
	addIdempotencyHeader(req)
	resp := httptest.NewRecorder()
	h.ServeHTTP(resp, req)

	if resp.Code != http.StatusCreated {
		t.Fatalf("expected 201 Created, got %d: %s", resp.Code, resp.Body.String())
	}

	var payload map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload["job_id"] != "demo" {
		t.Fatalf("expected job_id demo, got %v", payload["job_id"])
	}
	if payload["status"] != "queued" {
		t.Fatalf("expected status queued, got %v", payload["status"])
	}
	if started, ok := payload["started_at"].(string); !ok || started == "" {
		t.Fatalf("expected started_at timestamp, got %v", payload["started_at"])
	}
	result, ok := payload["result"].(map[string]any)
	if !ok {
		t.Fatalf("expected result payload, got %T", payload["result"])
	}
	resolved, ok := result["resolved_args"].(map[string]any)
	if !ok {
		t.Fatalf("expected resolved_args, got %T", result["resolved_args"])
	}
	if resolved["name"] != "Alice" {
		t.Fatalf("expected resolved name Alice, got %v", resolved["name"])
	}
	if payload["executor"] != "shell" {
		t.Fatalf("expected executor shell, got %v", payload["executor"])
	}
	prov, ok := payload["provenance"].(map[string]any)
	if !ok {
		t.Fatalf("expected provenance map, got %T", payload["provenance"])
	}
	source, ok := prov["source"].(map[string]any)
	if !ok {
		t.Fatalf("expected provenance.source map, got %T", prov["source"])
	}
	if source["type"] != "local" {
		t.Fatalf("expected provenance source type local, got %v", source["type"])
	}
	if source["resolved_ref"] == "" {
		t.Fatalf("expected resolved_ref in provenance")
	}
	getHandler := NewRunGetHandler(store)
	getReq := httptest.NewRequest(http.MethodGet, "/runs/"+payload["id"].(string), nil)
	getResp := httptest.NewRecorder()
	getHandler.ServeHTTP(getResp, getReq)
	if getResp.Code != http.StatusOK {
		t.Fatalf("expected GET /runs/{id} 200, got %d", getResp.Code)
	}
}

func TestRunsHandlerEmitsRunStartEvent(t *testing.T) {
	root := t.TempDir()
	writeJobConfig(t, root, "demo", `
version: v1
job:
  id: demo
  name: Demo Job
argspec:
  args:
    - name: name
      type: string
      required: true
`)

	store := runstore.New()
	sink := &recordingSink{}
	h := NewRunsHandler(RunsConfig{Root: root, Store: store, Events: sink})
	t.Logf("DataDir before run: %s", paths.DataDir())

	req := httptest.NewRequest(http.MethodPost, "/runs", strings.NewReader(`{"job_id":"demo","args":{"name":"Alice"}}`))
	req.Header.Set("Content-Type", "application/json")
	addIdempotencyHeader(req)
	resp := httptest.NewRecorder()
	h.ServeHTTP(resp, req)

	if resp.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", resp.Code)
	}
	waitFor(func() bool { return sink.count() >= 1 }, 200*time.Millisecond, t)
	e := sink.snapshot()[0]
	if e.runID == "" || e.event.Event != "run.start" {
		t.Fatalf("unexpected event payload: %+v", e)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(e.event.Data), &payload); err != nil {
		t.Fatalf("decode event: %v", err)
	}
	if payload["status"] != "running" {
		t.Fatalf("expected status running, got %v", payload["status"])
	}
	if payload["run_id"] == "" {
		t.Fatalf("expected run_id in event payload")
	}
	if payload["executor"] != "shell" {
		t.Fatalf("expected executor shell, got %v", payload["executor"])
	}
	if _, ok := payload["provenance"].(map[string]any); !ok {
		t.Fatalf("expected provenance in event payload, got %T", payload["provenance"])
	}
	waitFor(func() bool { return sink.countBy("run.finish") >= 1 }, 500*time.Millisecond, t)
}

func TestRunsHandlerProvenanceFromResolver(t *testing.T) {
	root := t.TempDir()
	writeJobConfig(t, root, "demo", `
version: v1
job:
  id: demo
  name: Demo Job
argspec:
  args:
    - name: name
      type: string
      required: true
`)

	store := runstore.New()
	resolver := func(jobID string, ref *RunSourceRef) (map[string]any, bool) {
		if ref == nil || ref.Name != "main-git" {
			return nil, false
		}
		return map[string]any{
			"type":         "git",
			"name":         "main-git",
			"ref":          "https://git.example/project.git",
			"resolved_ref": "sha256:abcd",
		}, true
	}

	h := NewRunsHandler(RunsConfig{Root: root, Store: store, ResolveSource: resolver})
	req := httptest.NewRequest(http.MethodPost, "/runs", strings.NewReader(`{"job_id":"demo","args":{"name":"Alice"},"source":{"name":"main-git"}}`))
	req.Header.Set("Content-Type", "application/json")
	addIdempotencyHeader(req)
	resp := httptest.NewRecorder()
	h.ServeHTTP(resp, req)

	if resp.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", resp.Code, resp.Body.String())
	}

	var payload map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	prov, ok := payload["provenance"].(map[string]any)
	if !ok {
		t.Fatalf("expected provenance map, got %T", payload["provenance"])
	}
	source, ok := prov["source"].(map[string]any)
	if !ok {
		t.Fatalf("expected provenance.source, got %T", prov["source"])
	}
	if source["type"] != "git" {
		t.Fatalf("expected git provenance, got %v", source["type"])
	}
	if source["resolved_ref"] != "sha256:abcd" {
		t.Fatalf("expected resolved_ref sha256:abcd, got %v", source["resolved_ref"])
	}
}

func TestRunsHandlerGitSource(t *testing.T) {
	repo, _ := createGitJobRepo(t, "gitjob", "")
	repoURL := url.URL{Scheme: "file", Path: filepath.ToSlash(repo)}
	sourceStore := sourcestore.New()
	checkoutDir := filepath.Join(t.TempDir(), "checkouts")
	sourcesHandler := NewSourcesHandler(SourcesConfig{
		Store:           sourceStore,
		AllowLocalRoots: []string{repo},
		AllowGitHosts:   []string{"example.com"},
		CheckoutDir:     checkoutDir,
	})

	registerReq := httptest.NewRequest(http.MethodPost, "/sources", strings.NewReader("{\"type\":\"git\",\"name\":\"git-remote\",\"url\":\""+repoURL.String()+"\",\"ref\":\"main\"}"))
	registerReq.Header.Set("Content-Type", "application/json")
	registerRec := httptest.NewRecorder()
	sourcesHandler.ServeHTTP(registerRec, registerReq)
	if registerRec.Code != http.StatusCreated {
		t.Fatalf("expected git source 201, got %d: %s", registerRec.Code, registerRec.Body.String())
	}

	runStore := runstore.New()
	sink := &recordingSink{}
	h := NewRunsHandler(RunsConfig{
		Root:    t.TempDir(),
		Store:   runStore,
		Events:  sink,
		Sources: sourceStore,
	})

	req := httptest.NewRequest(http.MethodPost, "/runs", strings.NewReader(`{"job_id":"gitjob","args":{"name":"Dana"},"source":{"name":"git-remote"}}`))
	req.Header.Set("Content-Type", "application/json")
	addIdempotencyHeader(req)
	resp := httptest.NewRecorder()
	h.ServeHTTP(resp, req)

	if resp.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", resp.Code, resp.Body.String())
	}

	var payload map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	prov, ok := payload["provenance"].(map[string]any)
	if !ok {
		t.Fatalf("expected provenance map, got %T", payload["provenance"])
	}
	source, ok := prov["source"].(map[string]any)
	if !ok {
		t.Fatalf("expected provenance.source, got %T", prov["source"])
	}
	if source["name"] != "git-remote" {
		t.Fatalf("expected provenance source git-remote, got %v", source["name"])
	}
	if source["resolved_ref"] == "" {
		t.Fatalf("expected resolved_ref to be populated")
	}

	waitFor(func() bool { return sink.countBy("run.finish") >= 1 }, 2*time.Second, t)
}

func TestRunsHandlerUsesLocalSource(t *testing.T) {
	defaultRoot := t.TempDir()
	sourceRoot := t.TempDir()

	writeJobConfig(t, defaultRoot, "local", `
version: v1
job:
  id: local
  name: Local Job
`)
	writeJobConfig(t, sourceRoot, "remote", `
version: v1
job:
  id: remote
  name: Remote Job
argspec:
  args:
    - name: name
      type: string
      required: true
`)

	store := runstore.New()
	ss := sourcestore.New()
	ss.Upsert(sourcestore.Source{
		Name:        "external",
		Type:        "local",
		ResolvedRef: sourceRoot,
		LocalPath:   sourceRoot,
	})

	h := NewRunsHandler(RunsConfig{Root: defaultRoot, Store: store, Sources: ss})
	req := httptest.NewRequest(http.MethodPost, "/runs", strings.NewReader(`{"job_id":"remote","args":{"name":"Bob"},"source":{"name":"external"}}`))
	req.Header.Set("Content-Type", "application/json")
	addIdempotencyHeader(req)
	resp := httptest.NewRecorder()
	h.ServeHTTP(resp, req)

	if resp.Code != http.StatusCreated {
		t.Fatalf("expected 201 Created, got %d: %s", resp.Code, resp.Body.String())
	}

	var payload map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	prov, ok := payload["provenance"].(map[string]any)
	if !ok {
		t.Fatalf("expected provenance map, got %T", payload["provenance"])
	}
	source, ok := prov["source"].(map[string]any)
	if !ok {
		t.Fatalf("expected provenance.source, got %T", prov["source"])
	}
	if source["name"] != "external" {
		t.Fatalf("expected provenance source name external, got %v", source["name"])
	}
	if source["type"] != "local" {
		t.Fatalf("expected provenance source type local, got %v", source["type"])
	}
	if ref := source["resolved_ref"]; ref == nil || ref == "" {
		t.Fatalf("expected resolved_ref in provenance source, got %v", ref)
	}

	runID, _ := payload["id"].(string)
	saved, ok := store.Get(runID)
	if !ok {
		t.Fatalf("expected stored run for %s", runID)
	}
	if saved.Provenance == nil {
		t.Fatalf("expected stored run to contain provenance")
	}
	savedSource, ok := saved.Provenance["source"].(map[string]any)
	if !ok || savedSource["name"] != "external" {
		t.Fatalf("expected stored provenance source external, got %+v", saved.Provenance)
	}
}

func TestRunsHandlerValidationError(t *testing.T) {
	root := t.TempDir()
	writeJobConfig(t, root, "demo", `
version: v1
job:
  id: demo
  name: Demo Job
argspec:
  args:
    - name: name
      type: string
      required: true
`)

	h := NewRunsHandler(RunsConfig{Root: root, Store: runstore.New()})
	req := httptest.NewRequest(http.MethodPost, "/runs", strings.NewReader(`{"job_id":"demo","args":{}}`))
	req.Header.Set("Content-Type", "application/json")
	addIdempotencyHeader(req)
	resp := httptest.NewRecorder()
	h.ServeHTTP(resp, req)

	if resp.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422, got %d: %s", resp.Code, resp.Body.String())
	}
	if resp.Header().Get("Content-Type") != "application/problem+json" {
		t.Fatalf("expected problem response header, got %q", resp.Header().Get("Content-Type"))
	}
	var problem map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&problem); err != nil {
		t.Fatalf("decode problem: %v", err)
	}
	if _, ok := problem["errors"].([]any); !ok {
		t.Fatalf("expected errors field, got %v", problem["errors"])
	}
}

func TestRunsHandlerUnknownJob(t *testing.T) {
	root := t.TempDir()
	store := runstore.New()
	h := NewRunsHandler(RunsConfig{Root: root, Store: store})
	req := httptest.NewRequest(http.MethodPost, "/runs", strings.NewReader(`{"job_id":"missing"}`))
	req.Header.Set("Content-Type", "application/json")
	addIdempotencyHeader(req)
	resp := httptest.NewRecorder()
	h.ServeHTTP(resp, req)

	if resp.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.Code)
	}

	getHandler := NewRunGetHandler(store)
	getReq := httptest.NewRequest(http.MethodGet, "/runs/does-not-exist", nil)
	getResp := httptest.NewRecorder()
	getHandler.ServeHTTP(getResp, getReq)
	if getResp.Code != http.StatusNotFound {
		t.Fatalf("expected GET missing run 404, got %d", getResp.Code)
	}
}

func TestRunsHandlerIdempotency(t *testing.T) {
	root := t.TempDir()
	writeJobConfig(t, root, "demo", `
version: v1
job:
  id: demo
  name: Demo Job
argspec:
  args:
    - name: name
      type: string
      required: true
`)

	store := runstore.New()
	sink := &recordingSink{}
	h := NewRunsHandler(RunsConfig{Root: root, Now: func() time.Time { return time.Unix(0, 0).UTC() }, Store: store, Events: sink})
	payload := `{"job_id":"demo","args":{"name":"Alice"}}`

	first := httptest.NewRecorder()
	req1 := httptest.NewRequest(http.MethodPost, "/runs", strings.NewReader(payload))
	req1.Header.Set("Content-Type", "application/json")
	req1.Header.Set("Idempotency-Key", "aaaaaaaaaaaaaaaaaaaa")
	h.ServeHTTP(first, req1)
	if first.Code != http.StatusCreated {
		t.Fatalf("expected first request 201, got %d", first.Code)
	}
	var firstBody map[string]any
	if err := json.NewDecoder(first.Body).Decode(&firstBody); err != nil {
		t.Fatalf("decode first body: %v", err)
	}
	firstID, _ := firstBody["id"].(string)

	second := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodPost, "/runs", strings.NewReader(payload))
	req2.Header.Set("Content-Type", "application/json")
	req2.Header.Set("Idempotency-Key", "aaaaaaaaaaaaaaaaaaaa")
	h.ServeHTTP(second, req2)
	if second.Code != http.StatusCreated {
		t.Fatalf("expected second request 201, got %d", second.Code)
	}
	if replay := second.Header().Get("Idempotent-Replay"); replay != "true" {
		t.Fatalf("expected Idempotent-Replay header true, got %q", replay)
	}
	var secondBody map[string]any
	if err := json.NewDecoder(second.Body).Decode(&secondBody); err != nil {
		t.Fatalf("decode second body: %v", err)
	}
	if secondBody["id"] != firstID {
		t.Fatalf("expected idempotent response id %s, got %v", firstID, secondBody["id"])
	}
	waitFor(func() bool { return sink.countBy("run.finish") >= 1 }, 500*time.Millisecond, t)
	t.Logf("events: %+v", sink.snapshot())
	if sink.countBy("run.start") != 1 {
		t.Fatalf("expected single run.start emission under idempotency, got %d", sink.countBy("run.start"))
	}
}

type quotaFailingIdempotencyStore struct{}

func (quotaFailingIdempotencyStore) Lookup(context.Context, string, string, time.Time) (RunPayload, int, string, bool, error) {
	return RunPayload{}, 0, "", false, nil
}

func (quotaFailingIdempotencyStore) Store(context.Context, string, string, string, RunPayload, int, time.Time) error {
	return coredb.ErrJournalQuotaExceeded
}

func TestRunsHandlerStorageQuotaExceeded(t *testing.T) {
	root := t.TempDir()
	writeJobConfig(t, root, "demo", `
version: v1
job:
  id: demo
  name: Demo Job
argspec:
  args:
    - name: name
      type: string
      required: true
`)

	store := runstore.New()
	h := NewRunsHandler(RunsConfig{Root: root, Store: store})
	h.idempotency = quotaFailingIdempotencyStore{}

	req := httptest.NewRequest(http.MethodPost, "/runs", strings.NewReader(`{"job_id":"demo","args":{"name":"Casey"}}`))
	req.Header.Set("Content-Type", "application/json")
	addIdempotencyHeader(req)
	resp := httptest.NewRecorder()

	h.ServeHTTP(resp, req)

	if resp.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429 storage quota response, got %d (%s)", resp.Code, resp.Body.String())
	}
	var prob map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&prob); err != nil {
		t.Fatalf("decode problem: %v", err)
	}
	if prob["type"] != storageQuotaProblemType {
		t.Fatalf("expected problem type %s, got %v", storageQuotaProblemType, prob["type"])
	}
	if prob["title"] != "storage quota exceeded" {
		t.Fatalf("unexpected title: %v", prob["title"])
	}
	if runs := store.List(); len(runs) != 0 {
		t.Fatalf("expected no runs persisted on quota failure, found %d", len(runs))
	}
}

func TestRunsHandlerMissingIdempotencyKey(t *testing.T) {
	root := t.TempDir()
	writeJobConfig(t, root, "demo", `
version: v1
job:
  id: demo
  name: Demo Job
argspec:
  args:
    - name: name
      type: string
      required: true
`)

	h := NewRunsHandler(RunsConfig{Root: root})
	req := httptest.NewRequest(http.MethodPost, "/runs", strings.NewReader(`{"job_id":"demo","args":{"name":"Alice"}}`))
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()

	h.ServeHTTP(resp, req)

	if resp.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 when Idempotency-Key missing, got %d", resp.Code)
	}
}

func TestRunsHandlerIdempotencyHashMismatch(t *testing.T) {
	root := t.TempDir()
	writeJobConfig(t, root, "demo", `
version: v1
job:
  id: demo
  name: Demo Job
argspec:
  args:
    - name: name
      type: string
      required: true
`)

	h := NewRunsHandler(RunsConfig{Root: root})
	payload := `{"job_id":"demo","args":{"name":"Alice"}}`
	req := httptest.NewRequest(http.MethodPost, "/runs", strings.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	setSpecificIdempotencyKey(req, "bbbbbbbbbbbbbbbbbbbb")
	req.Header.Set("Idempotency-SHA256", strings.Repeat("0", 64))
	resp := httptest.NewRecorder()

	h.ServeHTTP(resp, req)

	if resp.Code != http.StatusConflict {
		t.Fatalf("expected 409 for hash mismatch, got %d", resp.Code)
	}
}

func TestRunsHandlerIdempotencyScopedByPrincipal(t *testing.T) {
	root := t.TempDir()
	writeJobConfig(t, root, "demo", `
version: v1
job:
  id: demo
  name: Demo Job
argspec:
  args:
    - name: name
      type: string
      required: true
`)

	store := runstore.New()
	h := NewRunsHandler(RunsConfig{Root: root, Store: store})
	key := "cccccccccccccccccccc"

	req1 := httptest.NewRequest(http.MethodPost, "/runs", strings.NewReader(`{"job_id":"demo","args":{"name":"Alice"}}`))
	req1.Header.Set("Content-Type", "application/json")
	req1 = req1.WithContext(requestctx.WithPrincipal(req1.Context(), "tenant-A"))
	setSpecificIdempotencyKey(req1, key)
	resp1 := httptest.NewRecorder()
	h.ServeHTTP(resp1, req1)
	if resp1.Code != http.StatusCreated {
		t.Fatalf("expected 201 for first principal, got %d", resp1.Code)
	}
	if resp1.Header().Get("Idempotent-Replay") != "" {
		t.Fatalf("did not expect replay header on first request")
	}

	req2 := httptest.NewRequest(http.MethodPost, "/runs", strings.NewReader(`{"job_id":"demo","args":{"name":"Alice"}}`))
	req2.Header.Set("Content-Type", "application/json")
	req2 = req2.WithContext(requestctx.WithPrincipal(req2.Context(), "tenant-B"))
	setSpecificIdempotencyKey(req2, key)
	resp2 := httptest.NewRecorder()
	h.ServeHTTP(resp2, req2)
	if resp2.Code != http.StatusCreated {
		t.Fatalf("expected 201 for different principal, got %d", resp2.Code)
	}
	if resp2.Header().Get("Idempotent-Replay") == "true" {
		t.Fatalf("did not expect replay for different principal")
	}
}

func TestRunsHandlerContainerRuntimeMissing(t *testing.T) {
	root := t.TempDir()
	writeJobConfig(t, root, "container", `
version: v1
job:
  id: container
  name: Container Demo
interpreter: "container:alpine:3.20"
executor: container
argspec:
  args:
    - name: name
      type: string
      required: true
`)

	oldDetect := detectContainerRuntime
	detectContainerRuntime = func(func(string) (string, error)) (container.Runtime, error) {
		return "", errors.New("no runtime")
	}
	defer func() { detectContainerRuntime = oldDetect }()

	h := NewRunsHandler(RunsConfig{Root: root})
	req := httptest.NewRequest(http.MethodPost, "/runs", strings.NewReader(`{"job_id":"container","args":{"name":"Alice"}}`))
	req.Header.Set("Content-Type", "application/json")
	addIdempotencyHeader(req)
	resp := httptest.NewRecorder()

	h.ServeHTTP(resp, req)

	if resp.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422 when runtime missing, got %d", resp.Code)
	}
	if ct := resp.Header().Get("Content-Type"); ct != "application/problem+json" {
		t.Fatalf("expected application/problem+json, got %q", ct)
	}
	var problem map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&problem); err != nil {
		t.Fatalf("decode problem: %v", err)
	}
	if problem["code"] != "container.runtime.unavailable" {
		t.Fatalf("expected code container.runtime.unavailable, got %+v", problem)
	}
}

func TestRunsHandlerCancel(t *testing.T) {
	root := t.TempDir()
	writeJobConfig(t, root, "sleepy", `
version: v1
job:
  id: sleepy
  name: Sleepy Job
interpreter: "/bin/bash"
`)
	scriptPath := filepath.Join(root, "sleepy", "100_main.sh")
	script := "#!/usr/bin/env bash\nsleep 2\n"
	if err := os.WriteFile(scriptPath, []byte(script), 0o755); err != nil {
		t.Fatalf("write script: %v", err)
	}

	runStore := runstore.New()
	sink := &recordingSink{}
	h := NewRunsHandler(RunsConfig{Root: root, Store: runStore, Events: sink})

	createReq := httptest.NewRequest(http.MethodPost, "/runs", strings.NewReader(`{"job_id":"sleepy"}`))
	createReq.Header.Set("Content-Type", "application/json")
	addIdempotencyHeader(createReq)
	createResp := httptest.NewRecorder()
	h.ServeHTTP(createResp, createReq)
	if createResp.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", createResp.Code, createResp.Body.String())
	}
	var payload map[string]any
	if err := json.NewDecoder(createResp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode run payload: %v", err)
	}
	runID, _ := payload["id"].(string)
	if runID == "" {
		t.Fatal("expected run id")
	}

	cancelReq := httptest.NewRequest(http.MethodPost, "/runs/"+runID+":cancel", nil)
	cancelResp := httptest.NewRecorder()
	h.HandleCancel(cancelResp, cancelReq, runID)
	if cancelResp.Code != http.StatusAccepted {
		t.Fatalf("expected 202 Accepted, got %d: %s", cancelResp.Code, cancelResp.Body.String())
	}

	waitFor(func() bool {
		run, ok := runStore.Get(runID)
		return ok && run.Status == "canceled"
	}, 3*time.Second, t)

	if sink.countBy("run.canceled") == 0 {
		t.Fatal("expected run.canceled event")
	}
}

func TestRunsHandlerContainerNameConflict(t *testing.T) {
	root := t.TempDir()
	writeJobConfig(t, root, "container", `
version: v1
job:
  id: container
  name: Container Demo
interpreter: "container:alpine:3.20"
executor: container
argspec:
  args:
    - name: name
      type: string
      required: false
`)

	scriptPath := filepath.Join(root, "container", "100_main.sh")
	script := "#!/usr/bin/env bash\nexit 0\n"
	if err := os.WriteFile(scriptPath, []byte(script), 0o755); err != nil {
		t.Fatalf("write script: %v", err)
	}

	stubDir := t.TempDir()
	runtimeName := "stubruntime"
	runtimePath := filepath.Join(stubDir, runtimeName)
	stubScript := "#!/usr/bin/env bash\nif [ \"$1\" = \"rm\" ]; then\n  echo 'cannot remove' >&2\n  exit 1\nfi\nexit 0\n"
	if err := os.WriteFile(runtimePath, []byte(stubScript), 0o755); err != nil {
		t.Fatalf("write stub runtime: %v", err)
	}
	oldPath := os.Getenv("PATH")
	if err := os.Setenv("PATH", stubDir+":"+oldPath); err != nil {
		t.Fatalf("set PATH: %v", err)
	}
	defer os.Setenv("PATH", oldPath)

	oldDetect := detectContainerRuntime
	detectContainerRuntime = func(func(string) (string, error)) (container.Runtime, error) {
		return container.Runtime(runtimeName), nil
	}
	defer func() { detectContainerRuntime = oldDetect }()

	h := NewRunsHandler(RunsConfig{Root: root})
	req := httptest.NewRequest(http.MethodPost, "/runs", strings.NewReader(`{"job_id":"container","args":{"name":"Alice"}}`))
	req.Header.Set("Content-Type", "application/json")
	addIdempotencyHeader(req)
	resp := httptest.NewRecorder()

	h.ServeHTTP(resp, req)

	if resp.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422 on name conflict, got %d: %s", resp.Code, resp.Body.String())
	}
	var problem map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&problem); err != nil {
		t.Fatalf("decode problem: %v", err)
	}
	if problem["code"] != "container.name.conflict" {
		t.Fatalf("expected container.name.conflict, got %+v", problem)
	}
}

func TestRunsHandlerContainerSuccess(t *testing.T) {
	root := t.TempDir()
	writeJobConfig(t, root, "container", `
version: v1
job:
  id: container
  name: Container Demo
interpreter: "container:alpine:3.20"
executor: container
argspec:
  args:
    - name: name
      type: string
      required: true
`)
	scriptPath := filepath.Join(root, "container", "100_main.sh")
	script := "#!/usr/bin/env bash\nset -euo pipefail\nexit 0\n"
	if err := os.WriteFile(scriptPath, []byte(script), 0o755); err != nil {
		t.Fatalf("write container script: %v", err)
	}

	stubDir := t.TempDir()
	runtimeName := "testruntime"
	runtimePath := filepath.Join(stubDir, runtimeName)
	if err := os.WriteFile(runtimePath, []byte("#!/usr/bin/env bash\nexit 0\n"), 0o755); err != nil {
		t.Fatalf("write stub runtime: %v", err)
	}

	oldPath := os.Getenv("PATH")
	if err := os.Setenv("PATH", stubDir+":"+oldPath); err != nil {
		t.Fatalf("set PATH: %v", err)
	}
	defer os.Setenv("PATH", oldPath)

	oldDetect := detectContainerRuntime
	detectContainerRuntime = func(func(string) (string, error)) (container.Runtime, error) {
		return container.Runtime(runtimeName), nil
	}
	defer func() { detectContainerRuntime = oldDetect }()

	runStore := runstore.New()
	sink := &recordingSink{}
	h := NewRunsHandler(RunsConfig{Root: root, Store: runStore, Events: sink})

	req := httptest.NewRequest(http.MethodPost, "/runs", strings.NewReader(`{"job_id":"container","args":{"name":"Alice"}}`))
	req.Header.Set("Content-Type", "application/json")
	addIdempotencyHeader(req)
	resp := httptest.NewRecorder()
	h.ServeHTTP(resp, req)

	if resp.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", resp.Code, resp.Body.String())
	}

	var payload map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload["executor"] != "container" {
		t.Fatalf("expected executor container, got %v", payload["executor"])
	}
	if payload["runtime"] != runtimeName {
		t.Fatalf("expected runtime %s, got %v", runtimeName, payload["runtime"])
	}
	runID, _ := payload["id"].(string)
	if runID == "" {
		t.Fatalf("expected run id in response")
	}

	waitFor(func() bool { return sink.countBy("run.finish") >= 1 }, 2*time.Second, t)

	saved, ok := runStore.Get(runID)
	if !ok {
		t.Fatalf("expected run to be stored")
	}
	if saved.Status != "completed" {
		t.Fatalf("expected stored status completed, got %s", saved.Status)
	}
	if saved.Executor != "container" {
		t.Fatalf("expected stored executor container, got %s", saved.Executor)
	}
	if saved.Runtime != runtimeName {
		t.Fatalf("expected stored runtime %s, got %s", runtimeName, saved.Runtime)
	}
}

func TestRunsHandlerListPagination(t *testing.T) {
	root := t.TempDir()
	writeJobConfig(t, root, "demo", `
version: v1
job:
  id: demo
  name: Demo Job
argspec:
  args:
    - name: name
      type: string
      required: true
`)

	store := runstore.New()
	times := []time.Time{
		time.Unix(0, 0).UTC(),
		time.Unix(100, 0).UTC(),
		time.Unix(200, 0).UTC(),
	}
	var idx int
	h := NewRunsHandler(RunsConfig{
		Root:  root,
		Store: store,
		Now: func() time.Time {
			if idx >= len(times) {
				return time.Now().UTC()
			}
			t := times[idx]
			idx++
			return t
		},
	})

	for _, name := range []string{"Alice", "Bob", "Carol"} {
		req := httptest.NewRequest(http.MethodPost, "/runs", strings.NewReader(`{"job_id":"demo","args":{"name":"`+name+`"}}`))
		req.Header.Set("Content-Type", "application/json")
		addIdempotencyHeader(req)
		resp := httptest.NewRecorder()
		h.ServeHTTP(resp, req)
		if resp.Code != http.StatusCreated {
			t.Fatalf("expected 201, got %d", resp.Code)
		}
	}

	listReq := httptest.NewRequest(http.MethodGet, "/runs?per_page=2", nil)
	listResp := httptest.NewRecorder()
	h.ServeHTTP(listResp, listReq)
	if listResp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", listResp.Code)
	}
	var pageOne []map[string]any
	if err := json.NewDecoder(listResp.Body).Decode(&pageOne); err != nil {
		t.Fatalf("decode page one: %v", err)
	}
	if len(pageOne) != 2 {
		t.Fatalf("expected 2 runs on first page, got %d", len(pageOne))
	}
	if pageOne[0]["job_id"] != "demo" {
		t.Fatalf("expected job_id demo first entry")
	}
	if pageOne[0]["id"] == pageOne[1]["id"] {
		t.Fatalf("expected distinct run ids")
	}

	listReq2 := httptest.NewRequest(http.MethodGet, "/runs?page=2&per_page=2", nil)
	listResp2 := httptest.NewRecorder()
	h.ServeHTTP(listResp2, listReq2)
	if listResp2.Code != http.StatusOK {
		t.Fatalf("expected page 2 200, got %d", listResp2.Code)
	}
	var pageTwo []map[string]any
	if err := json.NewDecoder(listResp2.Body).Decode(&pageTwo); err != nil {
		t.Fatalf("decode page two: %v", err)
	}
	if len(pageTwo) != 1 {
		t.Fatalf("expected 1 run on second page, got %d", len(pageTwo))
	}
}

func TestRunsHandlerRejectsInvalidRequestedProfile(t *testing.T) {
	root := t.TempDir()
	writeJobConfig(t, root, "demo", `
version: v1
job:
  id: demo
  name: Demo Job
`)

	store := runstore.New()
	h := NewRunsHandler(RunsConfig{Root: root, Profile: "secure", Store: store})

	req := httptest.NewRequest(http.MethodPost, "/runs", strings.NewReader(`{"job_id":"demo","requested_security_profile":"invalid"}`))
	req.Header.Set("Content-Type", "application/json")
	addIdempotencyHeader(req)
	resp := httptest.NewRecorder()

	h.ServeHTTP(resp, req)

	if resp.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422, got %d: %s", resp.Code, resp.Body.String())
	}
	var problem map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&problem); err != nil {
		t.Fatalf("decode problem: %v", err)
	}
	if problem["code"] != "E_POLICY" {
		t.Fatalf("expected E_POLICY, got %+v", problem)
	}
	if len(store.List()) != 0 {
		t.Fatalf("run should not be persisted on invalid profile")
	}
}

func TestRunsHandlerBlocksDisallowedRegistry(t *testing.T) {
	root := t.TempDir()
	writeJobConfig(t, root, "registry", `
version: v1
job:
  id: registry
  name: Registry Job
executor: container
interpreter: "container:ghcr.io/example/app:1"
container:
  image: ghcr.io/example/app:1
`)

	policyCtx, err := policy.NewContext(&policy.Bundle{
		AllowedRegistries: []string{"registry.corp.example"},
	})
	if err != nil {
		t.Fatalf("policy context: %v", err)
	}

	oldDetect := detectContainerRuntime
	detectContainerRuntime = func(func(string) (string, error)) (container.Runtime, error) {
		return container.RuntimeDocker, nil
	}
	defer func() { detectContainerRuntime = oldDetect }()

	store := runstore.New()
	h := NewRunsHandler(RunsConfig{
		Root:     root,
		Profile:  "secure",
		Policy:   policyCtx,
		Store:    store,
		Verifier: stubVerifier{result: verify.Result{Verified: true}},
	})

	req := httptest.NewRequest(http.MethodPost, "/runs", strings.NewReader(`{"job_id":"registry"}`))
	req.Header.Set("Content-Type", "application/json")
	addIdempotencyHeader(req)
	resp := httptest.NewRecorder()

	h.ServeHTTP(resp, req)

	if resp.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422, got %d: %s", resp.Code, resp.Body.String())
	}
	var problem map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&problem); err != nil {
		t.Fatalf("decode problem: %v", err)
	}
	if problem["code"] != "image.registry.not.allowed" {
		t.Fatalf("expected image.registry.not.allowed, got %+v", problem)
	}
	if len(store.List()) != 0 {
		t.Fatalf("expected no run persisted on registry denial")
	}
}

func TestRunsHandlerOCIRunUnsupported(t *testing.T) {
	t.Setenv("FLWD_PROFILE", "")
	sources := sourcestore.New()
	manifestPath := writeOCIRunManifest(t, `
apiVersion: flwd.addon/v1
kind: AddOn
metadata:
  name: OCI Addon
  id: oci.addon
  version: 1.0.0
requires: {}
jobs:
  - id: build
    name: Build
    summary: Demo job
    argspec:
      args: []
`)
	sources.Upsert(sourcestore.Source{
		Name:        "addon",
		Type:        "oci",
		LocalPath:   filepath.Dir(manifestPath),
		Ref:         "ghcr.io/example/addon:1.0.0",
		Digest:      "sha256:deadbeef",
		ResolvedRef: "sha256:deadbeef",
		Metadata: map[string]any{
			"manifest_path": manifestPath,
		},
	})

	h := NewRunsHandler(RunsConfig{
		Root:    filepath.Join(t.TempDir(), "scripts"),
		Store:   runstore.New(),
		Sources: sources,
		Profile: "secure",
		Discover: func(string) (indexer.Result, error) {
			return indexer.Result{}, nil
		},
	})

	req := httptest.NewRequest(http.MethodPost, "/runs", strings.NewReader(`{"job_id":"addon/build"}`))
	req.Header.Set("Content-Type", "application/json")
	addIdempotencyHeader(req)
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("expected 501, got %d: %s", rec.Code, rec.Body.String())
	}
	var problem map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&problem); err != nil {
		t.Fatalf("decode problem: %v", err)
	}
	if problem["code"] != "E_OCI_RUN_UNSUPPORTED" {
		t.Fatalf("expected E_OCI_RUN_UNSUPPORTED, got %+v", problem["code"])
	}
	if sourcePayload, ok := problem["source"].(map[string]any); !ok || sourcePayload["type"] != "oci" {
		t.Fatalf("expected provenance source, got %+v", problem["source"])
	}
}

func TestRunsHandlerSignatureRequiredFailure(t *testing.T) {
	root := t.TempDir()
	writeJobConfig(t, root, "signed", `
version: v1
job:
  id: signed
  name: Signed Job
executor: container
interpreter: "container:registry.corp.example/app:1"
container:
  image: registry.corp.example/app:1
`)

	mode := "required"
	policyCtx, err := policy.NewContext(&policy.Bundle{
		VerifySignatures:  &mode,
		AllowedRegistries: []string{"registry.corp.example"},
	})
	if err != nil {
		t.Fatalf("policy context: %v", err)
	}

	oldDetect := detectContainerRuntime
	detectContainerRuntime = func(func(string) (string, error)) (container.Runtime, error) {
		return container.RuntimeDocker, nil
	}
	defer func() { detectContainerRuntime = oldDetect }()

	store := runstore.New()
	h := NewRunsHandler(RunsConfig{
		Root:     root,
		Profile:  "secure",
		Policy:   policyCtx,
		Store:    store,
		Verifier: stubVerifier{result: verify.Result{Verified: false, Reason: "unsigned"}},
	})

	req := httptest.NewRequest(http.MethodPost, "/runs", strings.NewReader(`{"job_id":"signed"}`))
	req.Header.Set("Content-Type", "application/json")
	addIdempotencyHeader(req)
	resp := httptest.NewRecorder()

	h.ServeHTTP(resp, req)

	if resp.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422, got %d: %s", resp.Code, resp.Body.String())
	}
	var problem map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&problem); err != nil {
		t.Fatalf("decode problem: %v", err)
	}
	if problem["code"] != "image.signature.required" {
		t.Fatalf("expected image.signature.required, got %+v", problem)
	}
	if len(store.List()) != 0 {
		t.Fatalf("expected no run stored on signature failure")
	}
}

func TestRunsHandlerResourceCeilingExceeded(t *testing.T) {
	root := t.TempDir()
	writeJobConfig(t, root, "ceiling", `
version: v1
job:
  id: ceiling
  name: Ceiling Job
executor: container
interpreter: "container:registry.corp.example/app:1"
container:
  image: registry.corp.example/app:1
  resources:
    cpu: "750m"
    memory: "512Mi"
`)

	policyCtx, err := policy.NewContext(&policy.Bundle{
		AllowedRegistries: []string{"registry.corp.example"},
		Ceilings: &policy.Ceilings{
			CPU:    "500m",
			Memory: "1Gi",
		},
	})
	if err != nil {
		t.Fatalf("policy context: %v", err)
	}

	oldDetect := detectContainerRuntime
	detectContainerRuntime = func(func(string) (string, error)) (container.Runtime, error) {
		return container.RuntimeDocker, nil
	}
	defer func() { detectContainerRuntime = oldDetect }()

	store := runstore.New()
	h := NewRunsHandler(RunsConfig{
		Root:     root,
		Profile:  "secure",
		Policy:   policyCtx,
		Store:    store,
		Verifier: stubVerifier{result: verify.Result{Verified: true}},
	})

	req := httptest.NewRequest(http.MethodPost, "/runs", strings.NewReader(`{"job_id":"ceiling"}`))
	req.Header.Set("Content-Type", "application/json")
	addIdempotencyHeader(req)
	resp := httptest.NewRecorder()

	h.ServeHTTP(resp, req)

	if resp.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422, got %d: %s", resp.Code, resp.Body.String())
	}
	var problem map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&problem); err != nil {
		t.Fatalf("decode problem: %v", err)
	}
	if problem["code"] != "E_IMAGE_POLICY" {
		t.Fatalf("expected E_IMAGE_POLICY, got %+v", problem)
	}
	if len(store.List()) != 0 {
		t.Fatalf("expected run not persisted on ceiling violation")
	}
}

func TestRunsHandlerOverrideDeniedSecure(t *testing.T) {
	root := t.TempDir()
	writeJobConfig(t, root, "override", `
version: v1
job:
  id: override
  name: Override Job
executor: container
interpreter: "container:alpine:3.20"
container:
  network: bridge
`)

	sink := &recordingSink{}
	h := NewRunsHandler(RunsConfig{
		Root:    root,
		Profile: "secure",
		Store:   runstore.New(),
		Events:  sink,
	})
	req := httptest.NewRequest(http.MethodPost, "/runs", strings.NewReader(`{"job_id":"override"}`))
	req.Header.Set("Content-Type", "application/json")
	addIdempotencyHeader(req)
	resp := httptest.NewRecorder()

	h.ServeHTTP(resp, req)

	if resp.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422, got %d: %s", resp.Code, resp.Body.String())
	}
	var problem map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&problem); err != nil {
		t.Fatalf("decode problem: %v", err)
	}
	if problem["code"] != "policy.denied" {
		t.Fatalf("expected policy.denied, got %+v", problem)
	}
	waitFor(func() bool { return sink.countBy("policy.decision") >= 1 }, 500*time.Millisecond, t)
	if sink.countBy("policy.decision") == 0 {
		t.Fatal("expected policy decision event on denial")
	}
	for _, evt := range sink.snapshot() {
		if evt.event.Event != "policy.decision" {
			continue
		}
		var payload map[string]any
		if err := json.Unmarshal([]byte(evt.event.Data), &payload); err != nil {
			t.Fatalf("decode policy decision event: %v", err)
		}
		if payload["code"] != "policy.denied" {
			t.Fatalf("expected event code policy.denied, got %+v", payload)
		}
		if payload["subject"] != "container.network" {
			t.Fatalf("expected subject container.network, got %+v", payload)
		}
		break
	}
}

func TestRunsHandlerOverrideAllowedPermissive(t *testing.T) {
	root := t.TempDir()
	writeJobConfig(t, root, "override", `
version: v1
job:
  id: override
  name: Override Job
executor: container
interpreter: "container:alpine:3.20"
container:
  network: bridge
`)

	bundle := &policy.Bundle{Overrides: &policy.Overrides{Network: []string{"bridge"}}}
	policyCtx, err := policy.NewContext(bundle)
	if err != nil {
		t.Fatalf("policy context: %v", err)
	}
	stubDir := t.TempDir()
	runtimeName := "testruntime"
	runtimePath := filepath.Join(stubDir, runtimeName)
	if err := os.WriteFile(runtimePath, []byte("#!/usr/bin/env bash\nexit 0\n"), 0o755); err != nil {
		t.Fatalf("write stub runtime: %v", err)
	}
	oldPath := os.Getenv("PATH")
	if err := os.Setenv("PATH", stubDir+":"+oldPath); err != nil {
		t.Fatalf("set PATH: %v", err)
	}
	defer os.Setenv("PATH", oldPath)

	oldDetect := detectContainerRuntime
	detectContainerRuntime = func(func(string) (string, error)) (container.Runtime, error) {
		return container.Runtime(runtimeName), nil
	}
	defer func() { detectContainerRuntime = oldDetect }()
	sink := &recordingSink{}
	h := NewRunsHandler(RunsConfig{
		Root:     root,
		Profile:  "permissive",
		Policy:   policyCtx,
		Store:    runstore.New(),
		Events:   sink,
		Verifier: stubVerifier{result: verify.Result{Verified: true}},
	})
	req := httptest.NewRequest(http.MethodPost, "/runs", strings.NewReader(`{"job_id":"override"}`))
	req.Header.Set("Content-Type", "application/json")
	addIdempotencyHeader(req)
	resp := httptest.NewRecorder()

	h.ServeHTTP(resp, req)

	if resp.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", resp.Code, resp.Body.String())
	}
	waitFor(func() bool { return sink.countBy("policy.decision") >= 1 }, 500*time.Millisecond, t)
	if sink.countBy("policy.decision") == 0 {
		t.Fatal("expected policy decision event")
	}
	for _, evt := range sink.snapshot() {
		if evt.event.Event != "policy.decision" {
			continue
		}
		var payload map[string]any
		if err := json.Unmarshal([]byte(evt.event.Data), &payload); err != nil {
			t.Fatalf("decode policy decision event: %v", err)
		}
		if payload["code"] != "policy.override.allowed" {
			t.Fatalf("expected policy.override.allowed code, got %+v", payload)
		}
		if payload["subject"] != "container.network" {
			t.Fatalf("expected subject container.network, got %+v", payload)
		}
		break
	}
}

func TestRunsHandlerEnvInheritanceDeniedSecure(t *testing.T) {
	root := t.TempDir()
	writeJobConfig(t, root, "env", `
version: v1
job:
  id: env
  name: Env Job
executor: container
interpreter: "container:alpine:3.20"
env_inheritance: true
`)

	h := NewRunsHandler(RunsConfig{
		Root:    root,
		Profile: "secure",
		Store:   runstore.New(),
	})
	req := httptest.NewRequest(http.MethodPost, "/runs", strings.NewReader(`{"job_id":"env"}`))
	req.Header.Set("Content-Type", "application/json")
	addIdempotencyHeader(req)
	resp := httptest.NewRecorder()

	h.ServeHTTP(resp, req)

	if resp.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422, got %d: %s", resp.Code, resp.Body.String())
	}
	var problem map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&problem); err != nil {
		t.Fatalf("decode problem: %v", err)
	}
	if problem["code"] != "policy.denied" {
		t.Fatalf("expected policy.denied, got %+v", problem)
	}
}

func TestSourceToProvenanceIncludesDigest(t *testing.T) {
	src := sourcestore.Source{
		Name:       "addon",
		Type:       "oci",
		Ref:        "ghcr.io/example/addon:1.0.0",
		Digest:     "sha256:1234",
		PullPolicy: "on-run",
		Metadata: map[string]any{
			"manifest": map[string]any{"id": "example.addon"},
		},
	}
	prov := sourceToProvenance(src)
	if prov["digest"] != "sha256:1234" {
		t.Fatalf("expected digest propagated, got %+v", prov)
	}
	if prov["pull_policy"] != "on-run" {
		t.Fatalf("expected pull_policy propagated, got %+v", prov)
	}
	if prov["resolved_ref"] != "sha256:1234" {
		t.Fatalf("expected resolved_ref to default to digest, got %+v", prov["resolved_ref"])
	}
	metadata, ok := prov["metadata"].(map[string]any)
	if !ok {
		t.Fatalf("expected metadata map, got %+v", prov["metadata"])
	}
	manifest, ok := metadata["manifest"].(map[string]any)
	if !ok || manifest["id"] != "example.addon" {
		t.Fatalf("expected manifest metadata preserved, got %+v", metadata)
	}
}

func TestPrepareSecretsWritesFiles(t *testing.T) {
	runDir := t.TempDir()
	binding := &engine.Binding{
		Values:      map[string]interface{}{"api-key": "supersecret"},
		SecretNames: map[string]struct{}{"api-key": {}},
	}
	secretDir, err := prepareSecrets(runDir, binding)
	if err != nil {
		t.Fatalf("prepare secrets: %v", err)
	}
	if secretDir == "" {
		t.Fatal("expected secrets directory path")
	}
	content, err := os.ReadFile(filepath.Join(secretDir, "api-key"))
	if err != nil {
		t.Fatalf("read secret file: %v", err)
	}
	if string(content) != "supersecret" {
		t.Fatalf("expected secret content preserved, got %q", string(content))
	}
	info, err := os.Stat(filepath.Join(secretDir, "api-key"))
	if err != nil {
		t.Fatalf("stat secret file: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("expected secret file perms 0600, got %v", info.Mode().Perm())
	}
}

func writeJobConfig(t *testing.T, root, jobID, yaml string) {
	t.Helper()
	jobDir := filepath.Join(root, jobID)
	if err := os.MkdirAll(filepath.Join(jobDir, "config.d"), 0o755); err != nil {
		t.Fatalf("mkdir config.d: %v", err)
	}
	path := filepath.Join(jobDir, "config.d", "config.yaml")
	if err := os.WriteFile(path, []byte(strings.TrimSpace(yaml)+"\n"), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
}

func writeOCIRunManifest(t *testing.T, yaml string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "manifest.yaml")
	if err := os.WriteFile(path, []byte(strings.TrimSpace(yaml)+"\n"), 0o600); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	return path
}

type recordedEvent struct {
	runID string
	event sse.Event
}

type recordingSink struct {
	mu     sync.Mutex
	events []recordedEvent
}

func (r *recordingSink) Publish(runID string, ev sse.Event) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, recordedEvent{runID: runID, event: ev})
}

func (r *recordingSink) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.events)
}

func (r *recordingSink) snapshot() []recordedEvent {
	r.mu.Lock()
	defer r.mu.Unlock()
	snapshot := make([]recordedEvent, len(r.events))
	copy(snapshot, r.events)
	return snapshot
}

func (r *recordingSink) countBy(event string) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	n := 0
	for _, e := range r.events {
		if e.event.Event == event {
			n++
		}
	}
	return n
}

func waitFor(cond func() bool, timeout time.Duration, t *testing.T) {
	deadline := time.Now().Add(timeout)
	for {
		if cond() {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("condition not met within %s", timeout)
		}
		time.Sleep(10 * time.Millisecond)
	}
}
