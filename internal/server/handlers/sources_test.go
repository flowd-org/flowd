// SC-REF: SC704 (Phase 7 — Alias visibility policy gating)
// Non-functional traceability tag for reviewer mapping.
package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/flowd-org/flowd/internal/executor/container"
	"github.com/flowd-org/flowd/internal/policy"
	policyverify "github.com/flowd-org/flowd/internal/policy/verify"
	"github.com/flowd-org/flowd/internal/server/metrics"
	"github.com/flowd-org/flowd/internal/server/sourcestore"
	"github.com/flowd-org/flowd/internal/types"
)

func TestSourcesHandlerLocalSuccess(t *testing.T) {
	root := t.TempDir()
	store := sourcestore.New()
	h := NewSourcesHandler(SourcesConfig{
		Store:           store,
		AllowLocalRoots: []string{root},
	})

	req := httptest.NewRequest(http.MethodPost, "/sources", strings.NewReader(`{"type":"local","ref":"demo"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201 Created, got %d: %s", rec.Code, rec.Body.String())
	}

	var payload map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload["name"] != "demo" {
		t.Fatalf("expected derived name demo, got %v", payload["name"])
	}
	if payload["type"] != "local" {
		t.Fatalf("expected type local, got %v", payload["type"])
	}
	if payload["resolved_ref"].(string) == "" {
		t.Fatalf("expected resolved_ref to be populated")
	}
	if payload["expose"] != "read" {
		t.Fatalf("expected expose read, got %v", payload["expose"])
	}
	if prov, ok := payload["provenance"].(map[string]any); !ok || prov["resolved_path"] == "" {
		t.Fatalf("expected provenance with resolved_path, got %+v", payload["provenance"])
	}

	listReq := httptest.NewRequest(http.MethodGet, "/sources", nil)
	listRec := httptest.NewRecorder()
	h.ServeHTTP(listRec, listReq)
	if listRec.Code != http.StatusOK {
		t.Fatalf("expected 200 for list, got %d", listRec.Code)
	}
	var list []map[string]any
	if err := json.NewDecoder(listRec.Body).Decode(&list); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected one source in list, got %d", len(list))
	}
	if prov, ok := list[0]["provenance"].(map[string]any); !ok || prov["resolved_path"] == "" {
		t.Fatalf("expected provenance in list response, got %+v", list[0]["provenance"])
	}
}

func TestSourcesHandlerGitHostBlocked(t *testing.T) {
	store := sourcestore.New()
	h := NewSourcesHandler(SourcesConfig{
		Store:           store,
		AllowLocalRoots: []string{t.TempDir()},
		AllowGitHosts:   []string{"github.com"},
	})

	req := httptest.NewRequest(http.MethodPost, "/sources", strings.NewReader(`{"type":"git","ref":"https://gitlab.com/example/repo.git"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for blocked host, got %d", rec.Code)
	}

	var problem map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&problem); err != nil {
		t.Fatalf("decode problem: %v", err)
	}
	if problem["code"] != "source.not.allowed" {
		t.Fatalf("expected problem code source.not.allowed, got %v", problem["code"])
	}
}

func TestSourceGetHandlerNotFound(t *testing.T) {
	store := sourcestore.New()
	handler := NewSourceGetHandler(SourcesConfig{Store: store})
	req := httptest.NewRequest(http.MethodGet, "/sources/unknown", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
}

func TestSourcesHandlerGitSuccess(t *testing.T) {
	repo, commit := createGitJobRepo(t, "remote", "")
	repoURL := url.URL{Scheme: "file", Path: filepath.ToSlash(repo)}
	store := sourcestore.New()
	checkoutDir := filepath.Join(t.TempDir(), "checkouts")
	h := NewSourcesHandler(SourcesConfig{
		Store:           store,
		AllowLocalRoots: []string{repo},
		AllowGitHosts:   []string{"example.com"},
		CheckoutDir:     checkoutDir,
	})

	payload := fmt.Sprintf(`{"type":"git","name":"remote","url":%q,"ref":"main"}`, repoURL.String())
	req := httptest.NewRequest(http.MethodPost, "/sources", strings.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201 Created, got %d: %s", rec.Code, rec.Body.String())
	}

	src, ok := store.Get("remote")
	if !ok {
		t.Fatalf("expected git source to be stored")
	}
	if src.Type != "git" {
		t.Fatalf("expected type git, got %s", src.Type)
	}
	if src.ResolvedRef == "" || src.ResolvedRef != commit {
		t.Fatalf("expected resolved ref %s, got %s", commit, src.ResolvedRef)
	}
	if src.ResolvedCommit != commit {
		t.Fatalf("expected resolved commit %s, got %s", commit, src.ResolvedCommit)
	}
	if src.LocalPath == "" {
		t.Fatalf("expected local checkout path")
	}
	if _, err := os.Stat(src.LocalPath); err != nil {
		t.Fatalf("expected checkout directory to exist: %v", err)
	}
	if meta, ok := src.Metadata["checkout_path"].(string); !ok || meta != src.LocalPath {
		t.Fatalf("expected metadata checkout_path %s, got %v", src.LocalPath, src.Metadata["checkout_path"])
	}
	if src.Expose != "read" {
		t.Fatalf("expected expose read, got %s", src.Expose)
	}
	if prov := src.Provenance; prov == nil || prov["resolved_commit"] != commit {
		t.Fatalf("expected provenance resolved_commit %s, got %+v", commit, prov)
	}
}

func TestSourcesHandlerGitFileURLOutsideAllowList(t *testing.T) {
	allowedRoot := t.TempDir()
	outsideRoot := t.TempDir()
	store := sourcestore.New()
	h := NewSourcesHandler(SourcesConfig{
		Store:           store,
		AllowLocalRoots: []string{allowedRoot},
		AllowGitHosts:   []string{"example.com"},
	})

	url := fmt.Sprintf(`"file://%s"`, filepath.ToSlash(outsideRoot))
	req := httptest.NewRequest(http.MethodPost, "/sources", strings.NewReader(fmt.Sprintf(`{"type":"git","url":%s,"ref":"main"}`, url)))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for disallowed file URL, got %d: %s", rec.Code, rec.Body.String())
	}

	var problem map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&problem); err != nil {
		t.Fatalf("decode problem: %v", err)
	}
	if problem["code"] != "source.not.allowed" {
		t.Fatalf("expected source.not.allowed, got %+v", problem)
	}
}

func TestSourcesHandlerAliasVisibilityToggle(t *testing.T) {
	store := sourcestore.New()
	store.Upsert(sourcestore.Source{
		Name:    "local",
		Type:    "local",
		Aliases: []types.CommandAlias{{From: "demo/build", To: "build-demo", Description: "friendly"}},
		Expose:  "readwrite",
	})

	hidden := NewSourcesHandler(SourcesConfig{
		Store:         store,
		ExposeAliases: func(*http.Request) bool { return false },
	})
	req := httptest.NewRequest(http.MethodGet, "/sources", nil)
	rec := httptest.NewRecorder()
	hidden.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for hidden aliases, got %d: %s", rec.Code, rec.Body.String())
	}
	var payload []map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatalf("decode hidden response: %v", err)
	}
	if len(payload) != 1 {
		t.Fatalf("expected single source entry, got %d", len(payload))
	}
	if _, ok := payload[0]["aliases"]; ok {
		t.Fatalf("expected aliases field to be omitted when exposure disabled: %+v", payload[0])
	}

	visible := NewSourcesHandler(SourcesConfig{
		Store:         store,
		ExposeAliases: func(*http.Request) bool { return true },
	})
	req = httptest.NewRequest(http.MethodGet, "/sources", nil)
	rec = httptest.NewRecorder()
	visible.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 for visible aliases, got %d: %s", rec.Code, rec.Body.String())
	}
	payload = nil
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatalf("decode visible response: %v", err)
	}
	aliases, ok := payload[0]["aliases"].([]any)
	if !ok || len(aliases) != 1 {
		t.Fatalf("expected aliases array with one entry, got %#v", payload[0]["aliases"])
	}
}

func TestSourcesHandlerExposeNoneHidesAliases(t *testing.T) {
	store := sourcestore.New()
	store.Upsert(sourcestore.Source{
		Name:    "git",
		Type:    "git",
		Aliases: []types.CommandAlias{{From: "demo", To: "demo", Description: "alias"}},
		Expose:  "none",
	})
	h := NewSourcesHandler(SourcesConfig{
		Store:         store,
		ExposeAliases: func(*http.Request) bool { return true },
	})
	req := httptest.NewRequest(http.MethodGet, "/sources", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var payload []map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	if len(payload) != 1 {
		t.Fatalf("expected one source, got %d", len(payload))
	}
	if aliases, ok := payload[0]["aliases"]; ok && aliases != nil {
		t.Fatalf("expected aliases hidden for expose=none, got %+v", aliases)
	}
}

func TestSourcesHandlerOCIRequiresTrust(t *testing.T) {
	t.Setenv("FLWD_PROFILE", "")
	store := sourcestore.New()
	policyCtx, err := policy.NewContext(nil)
	if err != nil {
		t.Fatalf("policy context: %v", err)
	}
	h := NewSourcesHandler(SourcesConfig{
		Store:    store,
		Profile:  "secure",
		Policy:   policyCtx,
		Verifier: &stubImageVerifier{result: policyverify.Result{Verified: true}},
	})

	req := httptest.NewRequest(http.MethodPost, "/sources", strings.NewReader(`{"type":"oci","ref":"ghcr.io/example/addon:1.0.0"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
	var problem map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&problem); err != nil {
		t.Fatalf("decode problem: %v", err)
	}
	if problem["code"] != "source.trust.required" {
		t.Fatalf("expected source.trust.required, got %+v", problem)
	}
}

func TestSourcesHandlerOCIRegistryDenied(t *testing.T) {
	t.Setenv("FLWD_PROFILE", "")
	store := sourcestore.New()
	bundle := &policy.Bundle{
		AllowedRegistries: []string{"allowed.registry"},
	}
	policyCtx, err := policy.NewContext(bundle)
	if err != nil {
		t.Fatalf("policy context: %v", err)
	}
	h := NewSourcesHandler(SourcesConfig{
		Store:    store,
		Profile:  "secure",
		Policy:   policyCtx,
		Verifier: &stubImageVerifier{result: policyverify.Result{Verified: true}},
	})

	reqBody := `{"type":"oci","ref":"ghcr.io/example/addon:1.0","trusted":true}`
	req := httptest.NewRequest(http.MethodPost, "/sources", strings.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422, got %d: %s", rec.Code, rec.Body.String())
	}
	var problem map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&problem); err != nil {
		t.Fatalf("decode problem: %v", err)
	}
	if problem["code"] != "image.registry.not.allowed" {
		t.Fatalf("expected image.registry.not.allowed, got %+v", problem)
	}
}

func TestSourcesHandlerOCISignatureFailureRequired(t *testing.T) {
	t.Setenv("FLWD_PROFILE", "")
	store := sourcestore.New()
	policyCtx, err := policy.NewContext(nil)
	if err != nil {
		t.Fatalf("policy context: %v", err)
	}
	h := NewSourcesHandler(SourcesConfig{
		Store:   store,
		Profile: "secure",
		Policy:  policyCtx,
		Verifier: &stubImageVerifier{
			result: policyverify.Result{Verified: false, Reason: "missing signature"},
		},
	})

	reqBody := `{"type":"oci","ref":"ghcr.io/example/addon:1.0","trusted":true}`
	req := httptest.NewRequest(http.MethodPost, "/sources", strings.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422, got %d: %s", rec.Code, rec.Body.String())
	}
	var problem map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&problem); err != nil {
		t.Fatalf("decode problem: %v", err)
	}
	if problem["code"] != "image.signature.required" {
		t.Fatalf("expected image.signature.required, got %+v", problem)
	}
}

func TestSourcesHandlerOCIPermissiveSignatureWarning(t *testing.T) {
	t.Setenv("FLWD_PROFILE", "")
	store := sourcestore.New()
	policyCtx, err := policy.NewContext(nil)
	if err != nil {
		t.Fatalf("policy context: %v", err)
	}
	cacheRoot := filepath.Join(t.TempDir(), "sources")
	manifest := `
apiVersion: flwd.addon/v1
kind: AddOn
metadata:
  name: Example
  id: example.addon
  version: 0.1.0
requires: {}
jobs:
  - id: example.job
    name: Example Job
    summary: Demo
    argspec:
      args: []
`

	withOCIRuntimeStub(t, func(ctx context.Context, runtime container.Runtime, args ...string) ([]byte, error) {
		switch {
		case len(args) >= 1 && args[0] == "pull":
			return []byte("pulled"), nil
		case len(args) >= 1 && args[0] == "run":
			return []byte(manifest), nil
		case len(args) >= 2 && args[0] == "image" && args[1] == "inspect":
			return ociInspectPayloadWithDigest("sha256:def456"), nil
		default:
			t.Fatalf("unexpected runtime args: %v", args)
		}
		return nil, nil
	})

	reqBody := `{"type":"oci","ref":"ghcr.io/example/addon:1.0","trusted":true}`
	req := httptest.NewRequest(http.MethodPost, "/sources", strings.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h := NewSourcesHandler(SourcesConfig{
		Store:       store,
		Profile:     "permissive",
		Policy:      policyCtx,
		Verifier:    &stubImageVerifier{result: policyverify.Result{Verified: false, Reason: "no signature"}},
		Runtime:     container.Runtime("podman"),
		CheckoutDir: cacheRoot,
	})
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201 Created, got %d: %s", rec.Code, rec.Body.String())
	}
	src, ok := store.Get("addon")
	if !ok {
		t.Fatalf("expected source to be stored")
	}
	if src.Type != "oci" {
		t.Fatalf("expected type oci, got %s", src.Type)
	}
	if src.PullPolicy != "always" {
		t.Fatalf("expected default pull policy always, got %s", src.PullPolicy)
	}
	if src.Metadata == nil {
		t.Fatalf("expected metadata to be populated")
	}
	if pp, ok := src.Metadata["pull_policy"].(string); !ok || pp != "always" {
		t.Fatalf("expected metadata pull_policy always, got %+v", src.Metadata["pull_policy"])
	}
	trustMeta, ok := src.Metadata["image_trust"].(map[string]any)
	if !ok {
		t.Fatalf("expected image_trust metadata, got %+v", src.Metadata)
	}
	if verified, ok := trustMeta["signature_verified"].(bool); !ok || verified {
		t.Fatalf("expected signature_verified=false, got %+v", trustMeta["signature_verified"])
	}
	if mode, ok := trustMeta["verify_mode"].(string); !ok || mode != string(policy.VerifyModePermissive) {
		t.Fatalf("expected verify_mode permissive, got %+v", trustMeta["verify_mode"])
	}
	if created, ok := src.Metadata["created"].(string); !ok || created == "" {
		t.Fatalf("expected created timestamp in metadata, got %+v", src.Metadata["created"])
	}
	if imageID, ok := src.Metadata["image_id"].(string); !ok || imageID == "" {
		t.Fatalf("expected image_id in metadata, got %+v", src.Metadata["image_id"])
	}
	if size, ok := src.Metadata["size_bytes"].(int64); !ok || size == 0 {
		t.Fatalf("expected size_bytes in metadata, got %+v", src.Metadata["size_bytes"])
	}
	labels, ok := src.Metadata["labels"].(map[string]string)
	if !ok {
		t.Fatalf("expected labels map in metadata, got %+v", src.Metadata["labels"])
	}
	if len(labels) == 0 {
		t.Fatalf("expected at least one label, got %+v", labels)
	}
	if src.Provenance == nil || src.Provenance["digest"] != src.Digest {
		t.Fatalf("expected provenance digest, got %+v", src.Provenance)
	}
}

func TestSourcesHandlerOCIPullPolicyOnRun(t *testing.T) {
	t.Setenv("FLWD_PROFILE", "")
	store := sourcestore.New()
	policyCtx, err := policy.NewContext(nil)
	if err != nil {
		t.Fatalf("policy context: %v", err)
	}
	cacheRoot := filepath.Join(t.TempDir(), "sources")
	manifest := `
apiVersion: flwd.addon/v1
kind: AddOn
metadata:
  name: Example
  id: example.addon
  version: 0.1.0
requires: {}
jobs:
  - id: example.job
    name: Example Job
    summary: Demo
    argspec:
      args: []
`
	withOCIRuntimeStub(t, func(ctx context.Context, runtime container.Runtime, args ...string) ([]byte, error) {
		if len(args) >= 1 && args[0] == "pull" {
			t.Fatalf("pull should be skipped for on-run policy")
		}
		switch {
		case len(args) >= 1 && args[0] == "run":
			return []byte(manifest), nil
		case len(args) >= 2 && args[0] == "image" && args[1] == "inspect":
			return ociInspectPayloadWithDigest(""), nil
		default:
			t.Fatalf("unexpected runtime args: %v", args)
		}
		return nil, nil
	})

	reqBody := `{"type":"oci","ref":"ghcr.io/example/addon:1.0","trusted":true,"pull_policy":"on-run"}`
	req := httptest.NewRequest(http.MethodPost, "/sources", strings.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h := NewSourcesHandler(SourcesConfig{
		Store:       store,
		Profile:     "permissive",
		Policy:      policyCtx,
		Verifier:    &stubImageVerifier{result: policyverify.Result{Verified: true}},
		Runtime:     container.Runtime("podman"),
		CheckoutDir: cacheRoot,
	})
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201 Created, got %d: %s", rec.Code, rec.Body.String())
	}
	src, ok := store.Get("addon")
	if !ok {
		t.Fatalf("expected source to be stored")
	}
	if src.PullPolicy != "ifNotPresent" {
		t.Fatalf("expected pull_policy ifNotPresent, got %s", src.PullPolicy)
	}
	if src.Metadata["pull_policy"] != "ifNotPresent" {
		t.Fatalf("expected metadata pull_policy ifNotPresent, got %v", src.Metadata["pull_policy"])
	}
	if src.Provenance == nil || src.Provenance["pull_policy"] != "ifNotPresent" {
		t.Fatalf("expected provenance pull_policy ifNotPresent, got %+v", src.Provenance)
	}
}

func TestSourcesHandlerOCIVerifySignaturesFailure(t *testing.T) {
	t.Setenv("FLWD_PROFILE", "")
	store := sourcestore.New()
	cacheRoot := filepath.Join(t.TempDir(), "sources")
	policyCtx, err := policy.NewContext(nil)
	if err != nil {
		t.Fatalf("policy context: %v", err)
	}
	manifest := `
apiVersion: flwd.addon/v1
kind: AddOn
metadata:
  name: Example Addon
  id: example.addon
  version: 1.0.0
requires: {}
jobs: []
`
	withOCIRuntimeStub(t, func(ctx context.Context, runtime container.Runtime, args ...string) ([]byte, error) {
		switch {
		case len(args) >= 1 && args[0] == "pull":
			return []byte("pulled"), nil
		case len(args) >= 1 && args[0] == "run":
			return []byte(manifest), nil
		case len(args) >= 2 && args[0] == "image" && args[1] == "inspect":
			return ociInspectPayloadWithDigest("sha256:deadbeef"), nil
		default:
			t.Fatalf("unexpected runtime args: %v", args)
		}
		return nil, nil
	})

	req := httptest.NewRequest(http.MethodPost, "/sources", strings.NewReader(`{"type":"oci","ref":"ghcr.io/example/addon:1.0","trusted":true,"verify_signatures":true}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h := NewSourcesHandler(SourcesConfig{
		Store:       store,
		Profile:     "secure",
		Policy:      policyCtx,
		Verifier:    &stubImageVerifier{result: policyverify.Result{Verified: false, Reason: "unsigned"}},
		Runtime:     container.Runtime("podman"),
		CheckoutDir: cacheRoot,
	})
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422 when signatures missing, got %d: %s", rec.Code, rec.Body.String())
	}
	var problem map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&problem); err != nil {
		t.Fatalf("decode problem: %v", err)
	}
	if problem["code"] != "source-signature-invalid" {
		t.Fatalf("expected code source-signature-invalid, got %+v", problem)
	}
	if _, ok := store.Get("addon"); ok {
		t.Fatalf("expected source not to be stored on signature failure")
	}
}

func TestSourcesHandlerOCIVerifySignaturesSuccess(t *testing.T) {
	t.Setenv("FLWD_PROFILE", "")
	store := sourcestore.New()
	cacheRoot := filepath.Join(t.TempDir(), "sources")
	policyCtx, err := policy.NewContext(nil)
	if err != nil {
		t.Fatalf("policy context: %v", err)
	}
	manifest := `
apiVersion: flwd.addon/v1
kind: AddOn
metadata:
  name: Signed Addon
  id: signed.addon
  version: 2.0.0
requires: {}
jobs:
  - id: signed.job
    name: Signed Job
    summary: Demo
    argspec:
      args:
        - name: input
          type: string
`
	withOCIRuntimeStub(t, func(ctx context.Context, runtime container.Runtime, args ...string) ([]byte, error) {
		switch {
		case len(args) >= 1 && args[0] == "pull":
			return []byte("pulled"), nil
		case len(args) >= 1 && args[0] == "run":
			return []byte(manifest), nil
		case len(args) >= 2 && args[0] == "image" && args[1] == "inspect":
			return ociInspectPayloadWithDigest("sha256:signed"), nil
		default:
			t.Fatalf("unexpected runtime args: %v", args)
		}
		return nil, nil
	})

	req := httptest.NewRequest(http.MethodPost, "/sources", strings.NewReader(`{"type":"oci","ref":"ghcr.io/example/addon:2.0","trusted":true,"verify_signatures":true}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h := NewSourcesHandler(SourcesConfig{
		Store:       store,
		Profile:     "secure",
		Policy:      policyCtx,
		Verifier:    &stubImageVerifier{result: policyverify.Result{Verified: true}},
		Runtime:     container.Runtime("podman"),
		CheckoutDir: cacheRoot,
	})
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201 Created, got %d: %s", rec.Code, rec.Body.String())
	}
	src, ok := store.Get("addon")
	if !ok {
		t.Fatalf("expected source stored")
	}
	if !src.VerifySignatures {
		t.Fatalf("expected source VerifySignatures true")
	}
	if src.Provenance == nil || src.Provenance["verify_signatures"] != true {
		t.Fatalf("expected provenance verify_signatures true, got %+v", src.Provenance)
	}
}

func TestSourcesHandlerOCIAddSuccess(t *testing.T) {
	t.Setenv("FLWD_PROFILE", "")
	store := sourcestore.New()
	cacheRoot := filepath.Join(t.TempDir(), "sources")
	policyCtx, err := policy.NewContext(nil)
	if err != nil {
		t.Fatalf("policy context: %v", err)
	}
	manifest := `
apiVersion: flwd.addon/v1
kind: AddOn
metadata:
  name: Example Addon
  id: example.addon
  version: 1.2.3
requires: {}
jobs:
  - id: example.job
    name: Example Job
    summary: Demo
    argspec:
      args:
        - name: input
          type: string
`

	withOCIRuntimeStub(t, func(ctx context.Context, runtime container.Runtime, args ...string) ([]byte, error) {
		switch {
		case len(args) >= 1 && args[0] == "pull":
			return []byte("pulled"), nil
		case len(args) >= 1 && args[0] == "run":
			return []byte(manifest), nil
		case len(args) >= 2 && args[0] == "image" && args[1] == "inspect":
			return ociInspectPayloadWithDigest("sha256:abc123"), nil
		default:
			t.Fatalf("unexpected runtime args: %v", args)
		}
		return nil, nil
	})

	h := NewSourcesHandler(SourcesConfig{
		Store:       store,
		Profile:     "secure",
		Policy:      policyCtx,
		Verifier:    &stubImageVerifier{result: policyverify.Result{Verified: true}},
		Runtime:     container.Runtime("podman"),
		CheckoutDir: cacheRoot,
	})

	reqBody := `{"type":"oci","ref":"ghcr.io/example/addon:1.2.3","trusted":true}`
	req := httptest.NewRequest(http.MethodPost, "/sources", strings.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201 Created, got %d: %s", rec.Code, rec.Body.String())
	}

	src, ok := store.Get("addon")
	if !ok {
		t.Fatalf("expected source stored")
	}
	if src.Digest != "sha256:abc123" {
		t.Fatalf("expected digest sha256:abc123, got %s", src.Digest)
	}
	if src.ResolvedRef != "sha256:abc123" {
		t.Fatalf("expected resolved ref sha256:abc123, got %s", src.ResolvedRef)
	}
	if src.LocalPath == "" {
		t.Fatalf("expected local path recorded")
	}
	manifestPath := filepath.Join(src.LocalPath, addonManifestFileName)
	if _, err := os.Stat(manifestPath); err != nil {
		t.Fatalf("expected manifest cached: %v", err)
	}
	if meta, ok := src.Metadata["manifest"].(map[string]any); !ok || meta["id"] != "example.addon" {
		t.Fatalf("expected manifest metadata stored, got %+v", src.Metadata["manifest"])
	}
	if created, ok := src.Metadata["created"].(string); !ok || created == "" {
		t.Fatalf("expected created metadata, got %+v", src.Metadata["created"])
	}
	if labels, ok := src.Metadata["labels"].(map[string]string); !ok {
		t.Fatalf("expected labels metadata, got %+v", src.Metadata["labels"])
	} else if len(labels) == 0 {
		t.Fatalf("expected labels metadata to be non-empty")
	}
	if size, ok := src.Metadata["size_bytes"].(int64); !ok || size == 0 {
		t.Fatalf("expected size_bytes metadata, got %+v", src.Metadata["size_bytes"])
	}
}

func TestSourceHandlerDeleteSuccess(t *testing.T) {
	store := sourcestore.New()
	store.Upsert(sourcestore.Source{Name: "addon", Type: "oci"})
	handler := NewSourceGetHandler(SourcesConfig{Store: store})
	req := httptest.NewRequest(http.MethodDelete, "/sources/addon", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204 No Content, got %d", rec.Code)
	}
	if _, ok := store.Get("addon"); ok {
		t.Fatalf("expected source to be removed")
	}
}

func TestSourceHandlerDeleteNotFound(t *testing.T) {
	store := sourcestore.New()
	handler := NewSourceGetHandler(SourcesConfig{Store: store})
	req := httptest.NewRequest(http.MethodDelete, "/sources/missing", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for missing source, got %d", rec.Code)
	}
}

func TestSourcesHandlerOCIManifestExtraField(t *testing.T) {
	t.Setenv("FLWD_PROFILE", "")
	store := sourcestore.New()
	cacheRoot := filepath.Join(t.TempDir(), "sources")
	policyCtx, err := policy.NewContext(nil)
	if err != nil {
		t.Fatalf("policy context: %v", err)
	}

	manifest := `
apiVersion: flwd.addon/v1
kind: AddOn
metadata:
  name: Example Addon
  id: example.addon
  version: 1.2.3
requires: {}
jobs:
  - id: job.one
    name: Example Job
    summary: Demo job
    argspec:
      args: []
unexpected: true
`

	withOCIRuntimeStub(t, func(ctx context.Context, runtime container.Runtime, args ...string) ([]byte, error) {
		switch {
		case len(args) >= 1 && args[0] == "pull":
			return []byte("pulled"), nil
		case len(args) >= 1 && args[0] == "run":
			return []byte(manifest), nil
		case len(args) >= 2 && args[0] == "image" && args[1] == "inspect":
			return ociInspectPayloadWithDigest("sha256:abc123"), nil
		default:
			t.Fatalf("unexpected runtime args: %v", args)
		}
		return nil, nil
	})

	h := NewSourcesHandler(SourcesConfig{
		Store:       store,
		Profile:     "secure",
		Policy:      policyCtx,
		Verifier:    &stubImageVerifier{result: policyverify.Result{Verified: true}},
		Runtime:     container.Runtime("podman"),
		CheckoutDir: cacheRoot,
	})

	req := httptest.NewRequest(http.MethodPost, "/sources", strings.NewReader(`{"type":"oci","ref":"ghcr.io/example/addon:1.2.3","trusted":true}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for extra manifest field, got %d: %s", rec.Code, rec.Body.String())
	}
	var problem map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&problem); err != nil {
		t.Fatalf("decode problem: %v", err)
	}
	detail, _ := problem["detail"].(string)
	if !strings.Contains(detail, "manifest.unexpected is not allowed") {
		t.Fatalf("expected detail mentioning disallowed field, got %q", detail)
	}
}

func TestSourcesHandlerOCIMetricsCounters(t *testing.T) {
	t.Setenv("FLWD_PROFILE", "")
	metrics.Default = metrics.NewRegistry()
	store := sourcestore.New()
	cacheRoot := filepath.Join(t.TempDir(), "sources")
	policyCtx, err := policy.NewContext(nil)
	if err != nil {
		t.Fatalf("policy context: %v", err)
	}
	manifest := `
apiVersion: flwd.addon/v1
kind: AddOn
metadata:
  name: Example
  id: example.addon
  version: 0.1.0
requires: {}
jobs:
  - id: job
    name: Job
    summary: Demo
    argspec:
      args: []
`

	withOCIRuntimeStub(t, func(ctx context.Context, runtime container.Runtime, args ...string) ([]byte, error) {
		switch {
		case len(args) >= 1 && args[0] == "pull":
			return []byte("pulled"), nil
		case len(args) >= 1 && args[0] == "run":
			return []byte(manifest), nil
		case len(args) >= 2 && args[0] == "image" && args[1] == "inspect":
			return ociInspectPayloadWithDigest("sha256:abc123"), nil
		default:
			t.Fatalf("unexpected args: %v", args)
		}
		return nil, nil
	})

	h := NewSourcesHandler(SourcesConfig{
		Store:       store,
		Profile:     "secure",
		Policy:      policyCtx,
		Verifier:    &stubImageVerifier{result: policyverify.Result{Verified: true}},
		Runtime:     container.Runtime("podman"),
		CheckoutDir: cacheRoot,
	})

	req := httptest.NewRequest(http.MethodPost, "/sources", strings.NewReader(`{"type":"oci","ref":"ghcr.io/example/addon:1.0","trusted":true}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", rec.Code, rec.Body.String())
	}
	if metrics.Default.ContainerPullsTotal() != 1 {
		t.Fatalf("expected container pull recorded")
	}
	added := metrics.Default.SourceAddedTotals()
	if added["oci"] != 1 {
		t.Fatalf("expected oci sources added counter to be 1, got %+v", added)
	}
	if metrics.Default.AddonManifestInvalidTotal() != 0 {
		t.Fatalf("expected no invalid manifest counts")
	}
}

func TestSourcesHandlerOCIManifestInvalidMetric(t *testing.T) {
	t.Setenv("FLWD_PROFILE", "")
	metrics.Default = metrics.NewRegistry()
	store := sourcestore.New()
	cacheRoot := filepath.Join(t.TempDir(), "sources")
	policyCtx, err := policy.NewContext(nil)
	if err != nil {
		t.Fatalf("policy context: %v", err)
	}

	withOCIRuntimeStub(t, func(ctx context.Context, runtime container.Runtime, args ...string) ([]byte, error) {
		switch {
		case len(args) >= 1 && args[0] == "pull":
			return []byte("pulled"), nil
		case len(args) >= 1 && args[0] == "run":
			return []byte("not yaml"), nil
		default:
			return []byte{}, fmt.Errorf("unexpected args: %v", args)
		}
	})

	h := NewSourcesHandler(SourcesConfig{
		Store:       store,
		Profile:     "secure",
		Policy:      policyCtx,
		Verifier:    &stubImageVerifier{result: policyverify.Result{Verified: true}},
		Runtime:     container.Runtime("podman"),
		CheckoutDir: cacheRoot,
	})

	req := httptest.NewRequest(http.MethodPost, "/sources", strings.NewReader(`{"type":"oci","ref":"ghcr.io/example/addon:1.0","trusted":true}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
	if metrics.Default.AddonManifestInvalidTotal() != 1 {
		t.Fatalf("expected invalid manifest counter updated, got %d", metrics.Default.AddonManifestInvalidTotal())
	}
}

func TestSourcesHandlerOCIManifestInvalid(t *testing.T) {
	t.Setenv("FLWD_PROFILE", "")
	store := sourcestore.New()
	cacheRoot := filepath.Join(t.TempDir(), "sources")
	policyCtx, err := policy.NewContext(nil)
	if err != nil {
		t.Fatalf("policy context: %v", err)
	}

	withOCIRuntimeStub(t, func(ctx context.Context, runtime container.Runtime, args ...string) ([]byte, error) {
		switch {
		case len(args) >= 1 && args[0] == "pull":
			return []byte("pulled"), nil
		case len(args) >= 1 && args[0] == "run":
			return []byte("not yaml"), nil
		default:
			return []byte{}, fmt.Errorf("unexpected args: %v", args)
		}
	})

	h := NewSourcesHandler(SourcesConfig{
		Store:       store,
		Profile:     "secure",
		Policy:      policyCtx,
		Verifier:    &stubImageVerifier{result: policyverify.Result{Verified: true}},
		Runtime:     container.Runtime("podman"),
		CheckoutDir: cacheRoot,
	})

	reqBody := `{"type":"oci","ref":"ghcr.io/example/addon:1.2.3","trusted":true}`
	req := httptest.NewRequest(http.MethodPost, "/sources", strings.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
	var problem map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&problem); err != nil {
		t.Fatalf("decode problem: %v", err)
	}
	if problem["code"] != "E_ADDON_MANIFEST" {
		t.Fatalf("expected E_ADDON_MANIFEST, got %+v", problem["code"])
	}
}

func withOCIRuntimeStub(t *testing.T, fn func(context.Context, container.Runtime, ...string) ([]byte, error)) {
	t.Helper()
	prev := ociRuntimeCommand
	ociRuntimeCommand = fn
	t.Cleanup(func() {
		ociRuntimeCommand = prev
	})
}

type stubImageVerifier struct {
	result policyverify.Result
	err    error
	calls  int
	image  string
}

func ociInspectPayloadWithDigest(digest string) []byte {
	repoDigest := ""
	if digest != "" {
		repoDigest = fmt.Sprintf("\"ghcr.io/example/addon@%s\"", digest)
	}
	if repoDigest == "" {
		return []byte(`[{"Digest":"","RepoDigests":[],"Created":"2025-01-01T00:00:00Z","Id":"sha256:imageid","Size":4096,"Config":{"Labels":{"com.example":"demo"}}}]`)
	}
	return []byte(fmt.Sprintf(`[{"Digest":%q,"RepoDigests":[%s],"Created":"2025-01-01T00:00:00Z","Id":"sha256:imageid","Size":4096,"Config":{"Labels":{"com.example":"demo"}}}]`, digest, repoDigest))
}

func (s *stubImageVerifier) Verify(ctx context.Context, image string) (policyverify.Result, error) {
	s.calls++
	s.image = image
	if s.err != nil {
		return policyverify.Result{}, s.err
	}
	return s.result, nil
}
