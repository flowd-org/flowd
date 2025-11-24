// SC-REF: SC702 (Phase 7 — Alias collision diagnostics with canonical RFC7807)
// Non-functional traceability tag for reviewer mapping.
package handlers

import (
	"bytes"
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
	"testing"

	"github.com/flowd-org/flowd/internal/executor/container"
	"github.com/flowd-org/flowd/internal/indexer"
	"github.com/flowd-org/flowd/internal/policy"
	"github.com/flowd-org/flowd/internal/policy/verify"
	"github.com/flowd-org/flowd/internal/server/sourcestore"
	"github.com/flowd-org/flowd/internal/types"
)

func TestPlansHandlerSuccess(t *testing.T) {
	root := t.TempDir()
	writePlanConfig(t, root, "demo", `
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

	h := NewPlansHandler(PlansConfig{Root: root, Runtime: container.Runtime("podman")})

	body := `{"job_id":"demo","args":{"name":"Alice"}}`
	req := httptest.NewRequest(http.MethodPost, "/plans", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d: %s", rr.Code, rr.Body.String())
	}
	if got := rr.Header().Get("Content-Type"); got != "application/json" {
		t.Fatalf("expected application/json, got %q", got)
	}

	var plan types.Plan
	if err := json.NewDecoder(rr.Body).Decode(&plan); err != nil {
		t.Fatalf("decode plan: %v", err)
	}
	if plan.JobID != "demo" {
		t.Fatalf("expected job_id demo, got %s", plan.JobID)
	}
	if len(plan.ResolvedArgs) == 0 || plan.ResolvedArgs["name"] != "Alice" {
		t.Fatalf("expected resolved name Alice, got %+v", plan.ResolvedArgs)
	}
}

func TestPlansHandlerContainerExecutor(t *testing.T) {
	root := t.TempDir()
	writePlanConfig(t, root, "container", `
version: v1
job:
  id: container
  name: Container Demo
executor: container
interpreter: "container:alpine:3.20"
`)

	h := NewPlansHandler(PlansConfig{Root: root, Runtime: container.Runtime("podman")})
	req := httptest.NewRequest(http.MethodPost, "/plans", strings.NewReader(`{"job_id":"container"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var plan types.Plan
	if err := json.NewDecoder(rec.Body).Decode(&plan); err != nil {
		t.Fatalf("decode plan: %v", err)
	}
	execPreview := plan.ExecutorPreview
	if execPreview["executor"] != "container" {
		t.Fatalf("expected executor preview container, got %+v", execPreview)
	}
	if execPreview["container_image"] != "alpine:3.20" {
		t.Fatalf("expected container image in preview, got %+v", execPreview)
	}
}

func TestPlansHandlerDAGPlanIncludesSteps(t *testing.T) {
	root := t.TempDir()
	writePlanConfig(t, root, "dag", `
version: v1
job:
  id: dag
  name: DAG Container Job
composition: steps
executor: container
container:
  image: alpine:3.18
steps:
  - id: prep
    script: scripts/prep.sh
  - id: run
    script: scripts/run.sh
    container:
      image: alpine:3.19
`)

	h := NewPlansHandler(PlansConfig{
		Root:     root,
		Runtime:  container.Runtime("podman"),
		Verifier: stubVerifier{result: verify.Result{Verified: true}},
	})
	req := httptest.NewRequest(http.MethodPost, "/plans", strings.NewReader(`{"job_id":"dag"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var plan types.Plan
	if err := json.NewDecoder(rec.Body).Decode(&plan); err != nil {
		t.Fatalf("decode plan: %v", err)
	}
	if plan.ExecutorPreview["composition"] != "steps" {
		t.Fatalf("expected composition steps in preview, got %+v", plan.ExecutorPreview)
	}
	if len(plan.Steps) != 2 {
		t.Fatalf("expected 2 steps in preview, got %+v", plan.Steps)
	}
	if plan.Steps[0].ContainerImage != "alpine:3.18" {
		t.Fatalf("expected default container image on first step, got %+v", plan.Steps[0])
	}
	if plan.Steps[1].ContainerImage != "alpine:3.19" {
		t.Fatalf("expected override container image on second step, got %+v", plan.Steps[1])
	}
	if plan.ImageTrust != nil {
		t.Fatalf("expected top-level image trust omitted for multi-step plan, got %+v", plan.ImageTrust)
	}
}

func TestPlansHandlerDAGValidationMixedExecutors(t *testing.T) {
	root := t.TempDir()
	writePlanConfig(t, root, "dag-invalid", `
version: v1
job:
  id: dag-invalid
  name: invalid
composition: steps
executor: container
steps:
  - id: a
    script: scripts/a.sh
    executor: proc
`)

	h := NewPlansHandler(PlansConfig{Root: root, Runtime: container.Runtime("podman")})
	req := httptest.NewRequest(http.MethodPost, "/plans", strings.NewReader(`{"job_id":"dag-invalid"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422 for mixed executors, got %d", rec.Code)
	}
	var problem map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&problem); err != nil {
		t.Fatalf("decode problem: %v", err)
	}
	if problem["code"] != "E_POLICY" {
		t.Fatalf("expected E_POLICY code, got %+v", problem)
	}
}

func TestPlansHandlerRuntimeMissing(t *testing.T) {
	root := t.TempDir()
	writePlanConfig(t, root, "container", `
version: v1
job:
  id: container
  name: Container Demo
executor: container
interpreter: "container:alpine:3.20"
`)

	oldDetect := detectContainerRuntime
	detectContainerRuntime = func(func(string) (string, error)) (container.Runtime, error) {
		return "", errors.New("no runtime")
	}
	defer func() { detectContainerRuntime = oldDetect }()

	h := NewPlansHandler(PlansConfig{Root: root})
	req := httptest.NewRequest(http.MethodPost, "/plans", strings.NewReader(`{"job_id":"container"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422 when runtime missing, got %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/problem+json" {
		t.Fatalf("expected application/problem+json, got %q", ct)
	}
	var problem map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&problem); err != nil {
		t.Fatalf("decode problem: %v", err)
	}
	if problem["code"] != "container.runtime.unavailable" {
		t.Fatalf("expected code container.runtime.unavailable, got %+v", problem)
	}
}

func TestPlansHandlerValidationError(t *testing.T) {
	root := t.TempDir()
	writePlanConfig(t, root, "demo", `
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

	h := NewPlansHandler(PlansConfig{Root: root})

	body := []byte(`{"job_id":"demo","args":{}}`)
	req := httptest.NewRequest(http.MethodPost, "/plans", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422, got %d: %s", rr.Code, rr.Body.String())
	}
	if got := rr.Header().Get("Content-Type"); got != "application/problem+json" {
		t.Fatalf("expected application/problem+json, got %q", got)
	}

	var problem map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&problem); err != nil {
		t.Fatalf("decode problem: %v", err)
	}
	errorsField, ok := problem["errors"].([]any)
	if !ok || len(errorsField) == 0 {
		t.Fatalf("expected errors field in problem: %+v", problem)
	}
}

func TestPlansHandlerUsesLocalSource(t *testing.T) {
	defaultRoot := t.TempDir()
	sourceRoot := t.TempDir()

	writePlanConfig(t, defaultRoot, "local", `
version: v1
job:
  id: local
  name: Local Job
`)
	writePlanConfig(t, sourceRoot, "remote", `
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

	store := sourcestore.New()
	store.Upsert(sourcestore.Source{
		Name:        "external",
		Type:        "local",
		ResolvedRef: sourceRoot,
		LocalPath:   sourceRoot,
	})

	h := NewPlansHandler(PlansConfig{
		Root:    defaultRoot,
		Sources: store,
	})

	body := `{"job_id":"remote","args":{"name":"Bob"},"source":{"name":"external"}}`
	req := httptest.NewRequest(http.MethodPost, "/plans", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d: %s", rr.Code, rr.Body.String())
	}

	var plan types.Plan
	if err := json.NewDecoder(rr.Body).Decode(&plan); err != nil {
		t.Fatalf("decode plan: %v", err)
	}
	if plan.JobID != "remote" {
		t.Fatalf("expected plan job_id remote, got %s", plan.JobID)
	}
	if plan.ResolvedArgs["name"] != "Bob" {
		t.Fatalf("expected resolved arg Bob, got %+v", plan.ResolvedArgs)
	}
}

func TestPlansHandlerUsesGitSource(t *testing.T) {
	repo, _ := createGitJobRepo(t, "gitjob", "")
	repoURL := url.URL{Scheme: "file", Path: filepath.ToSlash(repo)}
	store := sourcestore.New()
	checkoutDir := filepath.Join(t.TempDir(), "checkouts")
	sourcesHandler := NewSourcesHandler(SourcesConfig{
		Store:           store,
		AllowLocalRoots: []string{repo},
		AllowGitHosts:   []string{"example.com"},
		CheckoutDir:     checkoutDir,
	})

	registerPayload := "{" + `"type":"git","name":"git-remote","url":"` + repoURL.String() + `","ref":"main"` + "}"
	registerReq := httptest.NewRequest(http.MethodPost, "/sources", strings.NewReader(registerPayload))
	registerReq.Header.Set("Content-Type", "application/json")
	registerRec := httptest.NewRecorder()
	sourcesHandler.ServeHTTP(registerRec, registerReq)
	if registerRec.Code != http.StatusCreated {
		t.Fatalf("expected git source 201, got %d: %s", registerRec.Code, registerRec.Body.String())
	}

	h := NewPlansHandler(PlansConfig{Root: t.TempDir(), Sources: store})
	body := `{"job_id":"gitjob","args":{"name":"Charlie"},"source":{"name":"git-remote"}}`
	req := httptest.NewRequest(http.MethodPost, "/plans", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()
	h.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d: %s", resp.Code, resp.Body.String())
	}

	var plan types.Plan
	if err := json.NewDecoder(resp.Body).Decode(&plan); err != nil {
		t.Fatalf("decode plan: %v", err)
	}
	if plan.JobID != "gitjob" {
		t.Fatalf("expected job gitjob, got %s", plan.JobID)
	}
	if plan.ResolvedArgs["name"] != "Charlie" {
		t.Fatalf("expected resolved name Charlie, got %+v", plan.ResolvedArgs)
	}
}

func TestPlansHandlerRejectsInvalidRequestedProfile(t *testing.T) {
	root := t.TempDir()
	writePlanConfig(t, root, "demo", `
version: v1
job:
  id: demo
  name: Demo Job
`)

	h := NewPlansHandler(PlansConfig{Root: root, Profile: "secure"})
	body := `{"job_id":"demo","requested_security_profile":"bogus"}`
	req := httptest.NewRequest(http.MethodPost, "/plans", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422, got %d: %s", rr.Code, rr.Body.String())
	}

	var problem map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&problem); err != nil {
		t.Fatalf("decode problem: %v", err)
	}
	if problem["code"] != "E_POLICY" {
		t.Fatalf("expected code E_POLICY, got %+v", problem)
	}
}

func TestPlansHandlerBlocksDisallowedRegistry(t *testing.T) {
	root := t.TempDir()
	writePlanConfig(t, root, "registry", `
version: v1
job:
  id: registry
  name: Registry Job
executor: container
interpreter: "container:ghcr.io/example/app:1"
container:
  image: ghcr.io/example/app:1
`)

	bundle := &policy.Bundle{
		AllowedRegistries: []string{"registry.corp.example"},
	}
	policyCtx, err := policy.NewContext(bundle)
	if err != nil {
		t.Fatalf("new policy context: %v", err)
	}

	h := NewPlansHandler(PlansConfig{
		Root:     root,
		Profile:  "secure",
		Policy:   policyCtx,
		Verifier: stubVerifier{result: verify.Result{Verified: true}},
	})

	req := httptest.NewRequest(http.MethodPost, "/plans", strings.NewReader(`{"job_id":"registry"}`))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422, got %d: %s", rr.Code, rr.Body.String())
	}
	var problem map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&problem); err != nil {
		t.Fatalf("decode problem: %v", err)
	}
	if problem["code"] != "image.registry.not.allowed" {
		t.Fatalf("expected image.registry.not.allowed, got %+v", problem)
	}
}

func TestPlansHandlerSignatureRequiredFailure(t *testing.T) {
	root := t.TempDir()
	writePlanConfig(t, root, "signed", `
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
	bundle := &policy.Bundle{
		VerifySignatures:  &mode,
		AllowedRegistries: []string{"registry.corp.example"},
	}
	policyCtx, err := policy.NewContext(bundle)
	if err != nil {
		t.Fatalf("policy context: %v", err)
	}

	h := NewPlansHandler(PlansConfig{
		Root:     root,
		Profile:  "secure",
		Policy:   policyCtx,
		Verifier: stubVerifier{result: verify.Result{Verified: false, Reason: "no signature"}},
	})

	req := httptest.NewRequest(http.MethodPost, "/plans", strings.NewReader(`{"job_id":"signed"}`))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422, got %d: %s", rr.Code, rr.Body.String())
	}
	var problem map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&problem); err != nil {
		t.Fatalf("decode problem: %v", err)
	}
	if problem["code"] != "image.signature.required" {
		t.Fatalf("expected image.signature.required, got %+v", problem)
	}
	if detail, _ := problem["detail"].(string); !strings.Contains(detail, "no signature") {
		t.Fatalf("expected detail containing verifier reason, got %q", detail)
	}
}

func TestPlansHandlerPermissiveFinding(t *testing.T) {
	root := t.TempDir()
	writePlanConfig(t, root, "permissive", `
version: v1
job:
  id: permissive
  name: Permissive Job
executor: container
interpreter: "container:registry.corp.example/app:1"
container:
  image: registry.corp.example/app:1
`)

	policyCtx, err := policy.NewContext(&policy.Bundle{
		AllowedRegistries: []string{"registry.corp.example"},
	})
	if err != nil {
		t.Fatalf("policy context: %v", err)
	}

	h := NewPlansHandler(PlansConfig{
		Root:     root,
		Profile:  "secure",
		Policy:   policyCtx,
		Verifier: stubVerifier{result: verify.Result{Verified: false, Reason: "unsigned"}},
	})

	body := `{"job_id":"permissive","requested_security_profile":"permissive"}`
	req := httptest.NewRequest(http.MethodPost, "/plans", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	var plan types.Plan
	if err := json.NewDecoder(rr.Body).Decode(&plan); err != nil {
		t.Fatalf("decode plan: %v", err)
	}
	if plan.ImageTrust == nil || plan.ImageTrust.Mode != string(policy.VerifyModePermissive) || plan.ImageTrust.Verified {
		t.Fatalf("unexpected image trust preview: %+v", plan.ImageTrust)
	}
	if len(plan.PolicyFindings) != 1 || plan.PolicyFindings[0].Code != "image.signature.permissive" {
		t.Fatalf("expected permissive finding, got %+v", plan.PolicyFindings)
	}
}

func TestPlansHandlerResourceCeilingExceeded(t *testing.T) {
	root := t.TempDir()
	writePlanConfig(t, root, "ceiling", `
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

	bundle := &policy.Bundle{
		AllowedRegistries: []string{"registry.corp.example"},
		Ceilings: &policy.Ceilings{
			CPU:    "500m",
			Memory: "1Gi",
		},
	}
	policyCtx, err := policy.NewContext(bundle)
	if err != nil {
		t.Fatalf("policy context: %v", err)
	}

	h := NewPlansHandler(PlansConfig{
		Root:     root,
		Profile:  "secure",
		Policy:   policyCtx,
		Verifier: stubVerifier{result: verify.Result{Verified: true}},
	})

	req := httptest.NewRequest(http.MethodPost, "/plans", strings.NewReader(`{"job_id":"ceiling"}`))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422, got %d: %s", rr.Code, rr.Body.String())
	}
	var problem map[string]any
	if err := json.NewDecoder(rr.Body).Decode(&problem); err != nil {
		t.Fatalf("decode problem: %v", err)
	}
	if problem["code"] != "E_IMAGE_POLICY" {
		t.Fatalf("expected E_IMAGE_POLICY, got %+v", problem)
	}
}

func TestPlansHandlerOverrideDeniedSecure(t *testing.T) {
	root := t.TempDir()
	writePlanConfig(t, root, "override", `
version: v1
job:
  id: override
  name: Override Job
executor: container
interpreter: "container:alpine:3.20"
container:
  network: bridge
`)

	h := NewPlansHandler(PlansConfig{
		Root:     root,
		Profile:  "secure",
		Verifier: stubVerifier{result: verify.Result{Verified: true}},
	})
	req := httptest.NewRequest(http.MethodPost, "/plans", strings.NewReader(`{"job_id":"override"}`))
	req.Header.Set("Content-Type", "application/json")
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

func TestPlansHandlerOverrideAllowedPermissive(t *testing.T) {
	root := t.TempDir()
	writePlanConfig(t, root, "override", `
version: v1
job:
  id: override
  name: Override Job
executor: container
interpreter: "container:alpine:3.20"
container:
  network: bridge
`)

	bundle := &policy.Bundle{
		Overrides: &policy.Overrides{Network: []string{"bridge"}},
	}
	policyCtx, err := policy.NewContext(bundle)
	if err != nil {
		t.Fatalf("policy context: %v", err)
	}

	h := NewPlansHandler(PlansConfig{
		Root:     root,
		Profile:  "permissive",
		Policy:   policyCtx,
		Verifier: stubVerifier{result: verify.Result{Verified: true}},
	})
	req := httptest.NewRequest(http.MethodPost, "/plans", strings.NewReader(`{"job_id":"override"}`))
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()

	h.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.Code, resp.Body.String())
	}
	var plan types.Plan
	if err := json.NewDecoder(resp.Body).Decode(&plan); err != nil {
		t.Fatalf("decode plan: %v", err)
	}
	if len(plan.PolicyFindings) == 0 {
		t.Fatalf("expected policy findings for override, got none")
	}
	if plan.PolicyFindings[0].Code != "policy.override.allowed" {
		t.Fatalf("expected policy.override.allowed, got %+v", plan.PolicyFindings)
	}
}

func TestPlansHandlerEnvInheritanceDeniedSecure(t *testing.T) {
	root := t.TempDir()
	writePlanConfig(t, root, "env", `
version: v1
job:
  id: env
  name: Env Job
executor: container
interpreter: "container:alpine:3.20"
env_inheritance: true
`)

	h := NewPlansHandler(PlansConfig{
		Root:     root,
		Profile:  "secure",
		Verifier: stubVerifier{result: verify.Result{Verified: true}},
	})
	req := httptest.NewRequest(http.MethodPost, "/plans", strings.NewReader(`{"job_id":"env"}`))
	req.Header.Set("Content-Type", "application/json")
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

func TestPlansHandlerEnvInheritanceAllowedPermissive(t *testing.T) {
	root := t.TempDir()
	writePlanConfig(t, root, "env", `
version: v1
job:
  id: env
  name: Env Job
executor: container
interpreter: "container:alpine:3.20"
env_inheritance: true
`)
	bundle := &policy.Bundle{Overrides: &policy.Overrides{EnvInheritance: boolPtr(true)}}
	policyCtx, err := policy.NewContext(bundle)
	if err != nil {
		t.Fatalf("policy context: %v", err)
	}
	h := NewPlansHandler(PlansConfig{
		Root:     root,
		Profile:  "permissive",
		Policy:   policyCtx,
		Verifier: stubVerifier{result: verify.Result{Verified: true}},
	})
	req := httptest.NewRequest(http.MethodPost, "/plans", strings.NewReader(`{"job_id":"env"}`))
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()

	h.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.Code, resp.Body.String())
	}
	var plan types.Plan
	if err := json.NewDecoder(resp.Body).Decode(&plan); err != nil {
		t.Fatalf("decode plan: %v", err)
	}
	allowed := false
	for _, finding := range plan.PolicyFindings {
		if finding.Code == "policy.override.allowed" {
			allowed = true
			break
		}
	}
	if !allowed {
		t.Fatalf("expected policy override finding for env inheritance, got %+v", plan.PolicyFindings)
	}
}

func TestPlansHandlerOCIJobSuccess(t *testing.T) {
	t.Setenv("FLWD_PROFILE", "")
	store := sourcestore.New()
	tempDir := t.TempDir()
	manifestPath := filepath.Join(tempDir, "manifest.yaml")
	if err := os.WriteFile(manifestPath, []byte(`
apiVersion: flwd.addon/v1
kind: AddOn
metadata:
  name: OCI Addon
  id: oci.addon
  version: 1.0.0
requires:
  permissions: []
  containers:
    - image: ghcr.io/example/addon:1.0.0
      platform: linux/amd64
jobs:
  - id: build
    name: Build
    summary: Compile image
    argspec:
      args:
        - name: image-tag
          type: string
          required: true
    requirements:
      tools:
        - name: docker
          version: "24"
`), 0o600); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	store.Upsert(sourcestore.Source{
		Name:        "addon",
		Type:        "oci",
		LocalPath:   tempDir,
		Digest:      "sha256:deadbeef",
		ResolvedRef: "sha256:deadbeef",
		PullPolicy:  "on-add",
		Ref:         "ghcr.io/example/addon:1.0.0",
		Metadata: map[string]any{
			"manifest_path": manifestPath,
		},
	})

	bundle := &policy.Bundle{AllowedRegistries: []string{"ghcr.io"}}
	policyCtx, err := policy.NewContext(bundle)
	if err != nil {
		t.Fatalf("policy context: %v", err)
	}

	handler := NewPlansHandler(PlansConfig{
		Root:     filepath.Join(t.TempDir(), "scripts"),
		Sources:  store,
		Profile:  "secure",
		Policy:   policyCtx,
		Verifier: stubVerifier{result: verify.Result{Verified: true}},
		Discover: func(string) (indexer.Result, error) {
			return indexer.Result{}, nil
		},
		LoadConfig: func(string) (*types.Config, error) {
			return nil, errors.New("should not load config for oci")
		},
		Runtime: container.Runtime("podman"),
	})

	req := httptest.NewRequest(http.MethodPost, "/plans", strings.NewReader(`{"job_id":"addon/build","args":{"image-tag":"latest"}}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var plan types.Plan
	if err := json.NewDecoder(rec.Body).Decode(&plan); err != nil {
		t.Fatalf("decode plan: %v", err)
	}
	if plan.JobID != "addon/build" {
		t.Fatalf("unexpected job_id %s", plan.JobID)
	}
	if plan.SecurityProfile != "secure" {
		t.Fatalf("expected security profile secure, got %s", plan.SecurityProfile)
	}
	if val, ok := plan.ResolvedArgs["image-tag"].(string); !ok || val != "latest" {
		t.Fatalf("expected resolved arg image-tag=latest, got %+v", plan.ResolvedArgs)
	}
	if plan.ExecutorPreview["container_image"].(string) != "ghcr.io/example/addon:1.0.0" {
		t.Fatalf("unexpected container image %+v", plan.ExecutorPreview)
	}
	if plan.ExecutorPreview["resolved_digest"].(string) != "sha256:deadbeef" {
		t.Fatalf("expected resolved digest sha256:deadbeef, got %+v", plan.ExecutorPreview)
	}
	if plan.ImageTrust == nil || !plan.ImageTrust.Verified {
		t.Fatalf("expected image trust verified, got %+v", plan.ImageTrust)
	}
	if plan.Provenance == nil {
		t.Fatalf("expected provenance populated")
	} else if srcProv, ok := plan.Provenance["source"].(map[string]any); !ok || srcProv["name"] != "addon" {
		t.Fatalf("unexpected provenance %+v", plan.Provenance)
	}
	if plan.Requirements == nil || len(plan.Requirements.Tools) != 1 {
		t.Fatalf("expected tool requirements, got %+v", plan.Requirements)
	}
}

func TestPlansHandlerOCIPlanPermissiveSignatureWarning(t *testing.T) {
	store := sourcestore.New()
	manifestPath := writeOCIManifest(t, `
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
    summary: Demo
    argspec:
      args: []
`)
	store.Upsert(sourcestore.Source{
		Name:        "addon",
		Type:        "oci",
		LocalPath:   filepath.Dir(manifestPath),
		Ref:         "ghcr.io/example/addon:1.0.0",
		Digest:      "sha256:feedface",
		ResolvedRef: "sha256:feedface",
		PullPolicy:  "on-add",
		Metadata: map[string]any{
			"manifest_path": manifestPath,
		},
	})

	bundle := &policy.Bundle{AllowedRegistries: []string{"ghcr.io"}}
	policyCtx, err := policy.NewContext(bundle)
	if err != nil {
		t.Fatalf("policy context: %v", err)
	}

	handler := NewPlansHandler(PlansConfig{
		Sources:  store,
		Profile:  "permissive",
		Policy:   policyCtx,
		Verifier: stubVerifier{result: verify.Result{Verified: false, Reason: "no signature"}},
		Runtime:  container.Runtime("podman"),
		Discover: func(string) (indexer.Result, error) { return indexer.Result{}, nil },
		LoadConfig: func(string) (*types.Config, error) {
			return nil, errors.New("should not load config for oci")
		},
	})

	req := httptest.NewRequest(http.MethodPost, "/plans", strings.NewReader(`{"job_id":"addon/build"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var plan types.Plan
	if err := json.NewDecoder(rec.Body).Decode(&plan); err != nil {
		t.Fatalf("decode plan: %v", err)
	}
	if plan.ImageTrust == nil || plan.ImageTrust.Mode != string(policy.VerifyModePermissive) || plan.ImageTrust.Verified {
		t.Fatalf("expected permissive image trust warning, got %+v", plan.ImageTrust)
	}
	foundWarning := false
	for _, finding := range plan.PolicyFindings {
		if finding.Code == "image.signature.permissive" {
			foundWarning = true
			break
		}
	}
	if !foundWarning {
		t.Fatalf("expected signature warning finding, got %+v", plan.PolicyFindings)
	}
}

func writePlanJobConfig(t *testing.T, scriptsDir, relPath, jobID string) {
	t.Helper()
	configDir := filepath.Join(scriptsDir, relPath, "config.d")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", configDir, err)
	}
	content := fmt.Sprintf("version: v1\njob:\n  id: %s\n  name: %s\n", jobID, strings.ToUpper(jobID))
	if err := os.WriteFile(filepath.Join(configDir, "config.yaml"), []byte(content), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
}

func TestPlansHandlerAliasCollision(t *testing.T) {
	root := t.TempDir()
	scriptsDir := filepath.Join(root, "scripts")
	writePlanJobConfig(t, scriptsDir, "demo/build", "demo.build")
	writePlanJobConfig(t, scriptsDir, "demo/test", "demo.test")
	flwdYaml := `aliases:
  - from: "demo/build"
    to: "build-alias"
  - from: "demo/test"
    to: "build-alias"
`
	if err := os.WriteFile(filepath.Join(scriptsDir, "flwd.yaml"), []byte(flwdYaml), 0o644); err != nil {
		t.Fatalf("write flwd.yaml: %v", err)
	}

	handler := NewPlansHandler(PlansConfig{Root: scriptsDir, Profile: "secure"})
	req := httptest.NewRequest(http.MethodPost, "/plans", strings.NewReader(`{"job_id":"build-alias"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d: %s", rec.Code, rec.Body.String())
	}
	var prob map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&prob); err != nil {
		t.Fatalf("decode problem: %v", err)
	}
	if prob["code"] != "alias.collision" {
		t.Fatalf("expected alias.collision code, got %+v", prob)
	}
	contenders, ok := prob["contenders"].([]any)
	if !ok || len(contenders) != 2 {
		t.Fatalf("expected two contenders, got %+v", prob["contenders"])
	}
}

func TestPlansHandlerAliasReservedName(t *testing.T) {
	root := t.TempDir()
	scriptsDir := filepath.Join(root, "scripts")
	writePlanJobConfig(t, scriptsDir, "demo/build", "demo.build")
	flwdYaml := `aliases:
  - from: "demo/build"
    to: ":gen-completion"
`
	if err := os.WriteFile(filepath.Join(scriptsDir, "flwd.yaml"), []byte(flwdYaml), 0o644); err != nil {
		t.Fatalf("write flwd.yaml: %v", err)
	}

	handler := NewPlansHandler(PlansConfig{Root: scriptsDir, Profile: "secure"})
	req := httptest.NewRequest(http.MethodPost, "/plans", strings.NewReader(`{"job_id":":gen-completion"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
	var prob map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&prob); err != nil {
		t.Fatalf("decode problem: %v", err)
	}
	if prob["code"] != "alias.reserved" {
		t.Fatalf("expected alias.reserved code, got %+v", prob)
	}
}

func TestPlansHandlerAliasInvalidTarget(t *testing.T) {
	root := t.TempDir()
	scriptsDir := filepath.Join(root, "scripts")
	writePlanJobConfig(t, scriptsDir, "demo/build", "demo.build")
	flwdYaml := `aliases:
  - from: "demo/build"
    to: "build-alias"
  - from: "build-alias"
    to: "run-alias"
`
	if err := os.WriteFile(filepath.Join(scriptsDir, "flwd.yaml"), []byte(flwdYaml), 0o644); err != nil {
		t.Fatalf("write flwd.yaml: %v", err)
	}

	handler := NewPlansHandler(PlansConfig{Root: scriptsDir, Profile: "secure"})
	req := httptest.NewRequest(http.MethodPost, "/plans", strings.NewReader(`{"job_id":"run-alias"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
	var prob map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&prob); err != nil {
		t.Fatalf("decode problem: %v", err)
	}
	if prob["code"] != "alias.target.invalid" {
		t.Fatalf("expected alias.target.invalid code, got %+v", prob)
	}
}

func TestPlansHandlerOCIPlanOnRunNoDigest(t *testing.T) {
	store := sourcestore.New()
	manifestPath := writeOCIManifest(t, `
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
    summary: Demo
    argspec:
      args: []
`)
	store.Upsert(sourcestore.Source{
		Name:       "addon",
		Type:       "oci",
		LocalPath:  filepath.Dir(manifestPath),
		Ref:        "ghcr.io/example/addon:latest",
		PullPolicy: "on-run",
		Metadata: map[string]any{
			"manifest_path": manifestPath,
		},
	})

	bundle := &policy.Bundle{AllowedRegistries: []string{"ghcr.io"}}
	policyCtx, err := policy.NewContext(bundle)
	if err != nil {
		t.Fatalf("policy context: %v", err)
	}

	handler := NewPlansHandler(PlansConfig{
		Sources:  store,
		Profile:  "secure",
		Policy:   policyCtx,
		Verifier: stubVerifier{result: verify.Result{Verified: true}},
		Runtime:  container.Runtime("podman"),
		Discover: func(string) (indexer.Result, error) { return indexer.Result{}, nil },
		LoadConfig: func(string) (*types.Config, error) {
			return nil, errors.New("should not load config for oci")
		},
	})

	req := httptest.NewRequest(http.MethodPost, "/plans", strings.NewReader(`{"job_id":"addon/build"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var plan types.Plan
	if err := json.NewDecoder(rec.Body).Decode(&plan); err != nil {
		t.Fatalf("decode plan: %v", err)
	}
	if _, ok := plan.ExecutorPreview["resolved_digest"]; ok {
		t.Fatalf("did not expect resolved digest for on-run policy, got %+v", plan.ExecutorPreview)
	}
	found := false
	for _, finding := range plan.PolicyFindings {
		if finding.Code == "oci.digest.missing" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected digest missing finding, got %+v", plan.PolicyFindings)
	}
}

func TestPlansHandlerOCIPlanRegistryDenied(t *testing.T) {
	store := sourcestore.New()
	manifestPath := writeOCIManifest(t, `
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
    summary: Demo
    argspec:
      args: []
`)
	store.Upsert(sourcestore.Source{
		Name:        "addon",
		Type:        "oci",
		LocalPath:   filepath.Dir(manifestPath),
		Ref:         "ghcr.io/example/addon:1.0.0",
		Digest:      "sha256:1234",
		ResolvedRef: "sha256:1234",
		PullPolicy:  "on-add",
		Metadata: map[string]any{
			"manifest_path": manifestPath,
		},
	})

	bundle := &policy.Bundle{AllowedRegistries: []string{"registry.example.com"}}
	policyCtx, err := policy.NewContext(bundle)
	if err != nil {
		t.Fatalf("policy context: %v", err)
	}

	handler := NewPlansHandler(PlansConfig{
		Sources:  store,
		Profile:  "secure",
		Policy:   policyCtx,
		Verifier: stubVerifier{result: verify.Result{Verified: true}},
		Runtime:  container.Runtime("podman"),
		Discover: func(string) (indexer.Result, error) { return indexer.Result{}, nil },
		LoadConfig: func(string) (*types.Config, error) {
			return nil, errors.New("should not load config for oci")
		},
	})

	req := httptest.NewRequest(http.MethodPost, "/plans", strings.NewReader(`{"job_id":"addon/build"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422, got %d: %s", rec.Code, rec.Body.String())
	}
	var problem map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&problem); err != nil {
		t.Fatalf("decode problem: %v", err)
	}
	if problem["code"] != "image.registry.not.allowed" {
		t.Fatalf("expected problem code image.registry.not.allowed, got %+v", problem["code"])
	}
}

func writePlanConfig(t *testing.T, root, jobID, yaml string) {
	t.Helper()
	jobDir := filepath.Join(root, jobID)
	if err := os.MkdirAll(filepath.Join(jobDir, "config.d"), 0o755); err != nil {
		t.Fatalf("mkdir config.d: %v", err)
	}
	if err := os.WriteFile(filepath.Join(jobDir, "config.d", "config.yaml"), []byte(strings.TrimSpace(yaml)+"\n"), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
}

type stubVerifier struct {
	result verify.Result
	err    error
}

func (s stubVerifier) Verify(ctx context.Context, image string) (verify.Result, error) {
	if s.err != nil {
		return verify.Result{}, s.err
	}
	return s.result, nil
}

func writeOCIManifest(t *testing.T, contents string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "manifest.yaml")
	if err := os.WriteFile(path, []byte(strings.TrimSpace(contents)+"\n"), 0o600); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	return path
}

func boolPtr(v bool) *bool {
	b := v
	return &b
}
