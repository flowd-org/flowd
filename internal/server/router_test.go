package server

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/flowd-org/flowd/internal/artifacts"
	"github.com/flowd-org/flowd/internal/coredb"
	"github.com/flowd-org/flowd/internal/paths"
	"github.com/flowd-org/flowd/internal/policy"
	"github.com/flowd-org/flowd/internal/policy/verify"
	"github.com/flowd-org/flowd/internal/server/metrics"
	"github.com/flowd-org/flowd/internal/server/sourcestore"
	"github.com/flowd-org/flowd/internal/types"
)

type bundleVerifierStub struct {
	called bool
	path   string
	err    error
}

func (s *bundleVerifierStub) Verify(ctx context.Context, ref string) error {
	s.called = true
	s.path = ref
	return s.err
}

func writePolicyFile(t *testing.T, contents string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "flwd.policy.yaml")
	if err := os.WriteFile(path, []byte(strings.TrimSpace(contents)+"\n"), 0o644); err != nil {
		t.Fatalf("write policy: %v", err)
	}
	return path
}

func TestLoadPolicyContextVerifiesBundleWhenSecure(t *testing.T) {
	policyPath := writePolicyFile(t, `allowed_registries: ["registry.corp.example"]`)
	t.Setenv("FLWD_POLICY_FILE", policyPath)

	stub := &bundleVerifierStub{}
	ctx := context.Background()
	policyCtx, err := loadPolicyContext(ctx, "secure", stub)
	if err != nil {
		t.Fatalf("loadPolicyContext: %v", err)
	}
	if policyCtx == nil {
		t.Fatal("expected non-nil policy context")
	}
	if !stub.called {
		t.Fatal("expected bundle verifier to be invoked")
	}
	if stub.path != policyPath {
		t.Fatalf("expected verifier path %q, got %q", policyPath, stub.path)
	}
}

func TestLoadPolicyContextSkipsVerificationWhenProfileNotSecure(t *testing.T) {
	policyPath := writePolicyFile(t, `allowed_registries: ["registry.corp.example"]`)
	t.Setenv("FLWD_POLICY_FILE", policyPath)

	stub := &bundleVerifierStub{err: errors.New("should not be called")}
	ctx := context.Background()
	policyCtx, err := loadPolicyContext(ctx, "permissive", stub)
	if err != nil {
		t.Fatalf("loadPolicyContext: %v", err)
	}
	if policyCtx == nil {
		t.Fatal("expected policy context even when verification skipped")
	}
	if stub.called {
		t.Fatal("expected bundle verifier not to be called for permissive profile")
	}
}

func TestLoadPolicyContextReturnsErrorWhenVerificationFails(t *testing.T) {
	policyPath := writePolicyFile(t, `allowed_registries: ["registry.corp.example"]`)
	t.Setenv("FLWD_POLICY_FILE", policyPath)

	stub := &bundleVerifierStub{err: errors.New("signature missing")}
	_, err := loadPolicyContext(context.Background(), "secure", stub)
	if err == nil {
		t.Fatal("expected error when verification fails")
	}
	if !strings.Contains(err.Error(), "signature missing") {
		t.Fatalf("expected underlying verifier error, got %v", err)
	}
}

// Ensure bundleVerifierStub satisfies the BundleVerifier interface used in production.
var _ verify.BundleVerifier = (*bundleVerifierStub)(nil)

func TestMetricsEndpointExposesSeries(t *testing.T) {
	metrics.Default = metrics.NewRegistry()
	cfg := Config{Bind: "127.0.0.1:0", Profile: "secure", MetricsEnabled: true}
	cfg = cfg.normalize()
	policyCtx, err := policy.NewContext(nil)
	if err != nil {
		t.Fatalf("policy context: %v", err)
	}
	handler := buildHandler(cfg, policyCtx, nil)
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, "flwd_build_info") {
		t.Fatalf("expected flwd_build_info metric, got body: %s", body)
	}
	if !strings.Contains(body, `spec_version="1.0.1"`) {
		t.Fatalf("expected flwd_build_info spec_version=1.0.1, got body: %s", body)
	}
	if !strings.Contains(body, "http_requests_total") {
		t.Fatalf("expected http_requests_total metric, got body: %s", body)
	}
}

func TestRuleYKVHandlerIntegration(t *testing.T) {
	tempDir := t.TempDir()
	cfg := Config{
		Bind:    "127.0.0.1:0",
		Profile: "secure",
		DataDir: tempDir,
		RuleY: types.RuleYConfig{
			Allowlist: map[string]types.RuleYNamespaceConfig{
				"core_triggers":         {MaxBytes: 9},
				"core_invocation_state": {MaxBytes: defaultRuleYMaxBytes},
			},
		},
	}
	cfg = cfg.normalize()
	db, err := coredb.Open(context.Background(), cfg.CoreDBOptions)
	if err != nil {
		t.Fatalf("open core db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	cfg.CoreDB = db

	policyCtx, err := policy.NewContext(nil)
	if err != nil {
		t.Fatalf("policy context: %v", err)
	}
	handler := buildHandler(cfg, policyCtx, nil)

	putBody := func(val string) *bytes.Reader {
		payload := map[string]string{"value": base64.StdEncoding.EncodeToString([]byte(val))}
		data, _ := json.Marshal(payload)
		return bytes.NewReader(data)
	}

	makeReq := func(method, path string, body *bytes.Reader, token string) *http.Request {
		var req *http.Request
		if body != nil {
			req = httptest.NewRequest(method, path, body)
		} else {
			req = httptest.NewRequest(method, path, nil)
		}
		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		req.Header.Set("Authorization", "Bearer "+token)
		return req
	}

	// Allowed namespace write
	resp := httptest.NewRecorder()
	handler.ServeHTTP(resp, makeReq(http.MethodPut, "/kv/core_triggers/a", putBody("abcd"), "ruley:write"))
	if resp.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", resp.Code)
	}

	// Disallowed namespace
	disallowed := httptest.NewRecorder()
	handler.ServeHTTP(disallowed, makeReq(http.MethodPut, "/kv/forbidden/a", putBody("abcd"), "ruley:write"))
	if disallowed.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for forbidden namespace, got %d", disallowed.Code)
	}
	var problem map[string]any
	if err := json.Unmarshal(disallowed.Body.Bytes(), &problem); err != nil {
		t.Fatalf("decode forbidden problem: %v", err)
	}
	if problem["type"] != "https://flowd.org/problems/namespace-forbidden" {
		t.Fatalf("expected namespace-forbidden problem, got %v", problem["type"])
	}

	// Quota exceeded on second write
	quota := httptest.NewRecorder()
	handler.ServeHTTP(quota, makeReq(http.MethodPut, "/kv/core_triggers/b", putBody("abcd"), "ruley:write"))
	if quota.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429 when quota exceeded, got %d", quota.Code)
	}

	// Scan limit is capped at 1000 even when a higher limit is requested.
	for i := 0; i < 1005; i++ {
		key := fmt.Sprintf("/kv/core_invocation_state/app:%04d", i)
		write := httptest.NewRecorder()
		handler.ServeHTTP(write, makeReq(http.MethodPut, key, putBody("x"), "ruley:write"))
		if write.Code != http.StatusNoContent {
			t.Fatalf("expected 204 while seeding scan rows, got %d for %s", write.Code, key)
		}
	}

	scan := httptest.NewRecorder()
	handler.ServeHTTP(scan, makeReq(http.MethodGet, "/kv/core_invocation_state?prefix=app:&limit=5000", nil, "ruley:read"))
	if scan.Code != http.StatusOK {
		t.Fatalf("expected 200 scan response, got %d: %s", scan.Code, scan.Body.String())
	}
	var scanBody struct {
		Items      []map[string]any `json:"items"`
		NextCursor string           `json:"nextCursor"`
	}
	if err := json.Unmarshal(scan.Body.Bytes(), &scanBody); err != nil {
		t.Fatalf("decode scan body: %v", err)
	}
	if got := len(scanBody.Items); got != 1000 {
		t.Fatalf("expected capped scan page size 1000, got %d", got)
	}
	if scanBody.NextCursor == "" {
		t.Fatal("expected nextCursor when scan page is capped")
	}
}

func TestCapabilitiesEndpointDoesNotAdvertiseKV(t *testing.T) {
	cfg := Config{Bind: "127.0.0.1:0", Profile: "secure", Dev: true}
	cfg = cfg.normalize()
	policyCtx, err := policy.NewContext(nil)
	if err != nil {
		t.Fatalf("policy context: %v", err)
	}
	handler := buildHandler(cfg, policyCtx, nil)

	token := unsignedJWT(`{"sub":"tester","scope":"jobs:read"}`)
	req := httptest.NewRequest(http.MethodGet, "/capabilities", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp := httptest.NewRecorder()

	handler.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.Code, resp.Body.String())
	}
	var raw map[string]any
	if err := json.Unmarshal(resp.Body.Bytes(), &raw); err != nil {
		t.Fatalf("decode capabilities body: %v", err)
	}
	if _, ok := raw["kv"]; ok {
		t.Fatal("capabilities must not advertise /kv")
	}
}

func TestArtifactsDownloadEndpointAuthzAndTenantIsolation(t *testing.T) {
	const artifactID = "018f22b0-1234-7abc-8def-0123456789ab"

	dataDir := t.TempDir()
	prevDataDir := paths.DataDir()
	paths.SetDataDirOverride(dataDir)
	t.Cleanup(func() { paths.SetDataDirOverride(prevDataDir) })

	cfg := Config{
		Bind:    "127.0.0.1:0",
		Profile: "secure",
		Dev:     true,
		DataDir: dataDir,
	}
	cfg = cfg.normalize()
	db, err := coredb.Open(context.Background(), cfg.CoreDBOptions)
	if err != nil {
		t.Fatalf("open core db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	cfg.CoreDB = db

	metaStore := coredb.NewArtifactStore(db)
	byteStore := artifacts.NewStore(artifacts.Options{})
	const artifactBody = "hello artifact bytes"
	size, err := byteStore.Write(context.Background(), artifactID, strings.NewReader(artifactBody))
	if err != nil {
		t.Fatalf("write artifact bytes: %v", err)
	}
	if err := metaStore.Create(context.Background(), coredb.ArtifactRecord{
		ArtifactID:  artifactID,
		Tenant:      "acme",
		JobID:       "demo",
		RunID:       "run-1",
		Name:        "stdout",
		ContentType: "text/plain; charset=utf-8",
		SizeBytes:   size,
	}); err != nil {
		t.Fatalf("create artifact metadata: %v", err)
	}

	policyCtx, err := policy.NewContext(nil)
	if err != nil {
		t.Fatalf("policy context: %v", err)
	}
	handler := buildHandler(cfg, policyCtx, nil)

	t.Run("allows artifacts read with matching tenant", func(t *testing.T) {
		token := unsignedJWT(`{"sub":"tester","tenant":"acme","scope":"artifacts:read"}`)
		req := httptest.NewRequest(http.MethodGet, "/artifacts/"+artifactID, nil)
		req.Header.Set("Authorization", "Bearer "+token)
		resp := httptest.NewRecorder()

		handler.ServeHTTP(resp, req)

		if resp.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d: %s", resp.Code, resp.Body.String())
		}
		if got := resp.Header().Get("Content-Type"); got != "text/plain; charset=utf-8" {
			t.Fatalf("expected content-type from metadata, got %q", got)
		}
		payload, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatalf("read response body: %v", err)
		}
		if string(payload) != artifactBody {
			t.Fatalf("unexpected body %q", string(payload))
		}
	})

	t.Run("forbids reads without artifacts scope", func(t *testing.T) {
		token := unsignedJWT(`{"sub":"tester","tenant":"acme","scope":"runs:read"}`)
		req := httptest.NewRequest(http.MethodGet, "/artifacts/"+artifactID, nil)
		req.Header.Set("Authorization", "Bearer "+token)
		resp := httptest.NewRecorder()

		handler.ServeHTTP(resp, req)

		if resp.Code != http.StatusForbidden {
			t.Fatalf("expected 403, got %d", resp.Code)
		}
	})

	t.Run("forbids reads when tenant differs only by case", func(t *testing.T) {
		token := unsignedJWT(`{"sub":"tester","tenant":"ACME","scope":"artifacts:read"}`)
		req := httptest.NewRequest(http.MethodGet, "/artifacts/"+artifactID, nil)
		req.Header.Set("Authorization", "Bearer "+token)
		resp := httptest.NewRecorder()

		handler.ServeHTTP(resp, req)

		if resp.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d", resp.Code)
		}
	})

	t.Run("forbids cross-tenant reads", func(t *testing.T) {
		token := unsignedJWT(`{"sub":"tester","tenant":"other","scope":"artifacts:read"}`)
		req := httptest.NewRequest(http.MethodGet, "/artifacts/"+artifactID, nil)
		req.Header.Set("Authorization", "Bearer "+token)
		resp := httptest.NewRecorder()

		handler.ServeHTTP(resp, req)

		if resp.Code != http.StatusNotFound {
			t.Fatalf("expected 404, got %d", resp.Code)
		}

		var problem map[string]any
		if err := json.Unmarshal(resp.Body.Bytes(), &problem); err != nil {
			t.Fatalf("decode problem response: %v", err)
		}

		if _, ok := problem["artifact_tenant"]; ok {
			t.Fatal("response must not include artifact tenant identifier")
		}
		if _, ok := problem["principal_tenant"]; ok {
			t.Fatal("response must not include principal tenant identifier")
		}

		body := resp.Body.String()
		if strings.Contains(body, "\"acme\"") || strings.Contains(body, "\"other\"") {
			t.Fatalf("response must not leak tenant identifiers, got %s", body)
		}
	})
}

// TestRun_StopsOnJanitorFailure verifies that the server stops when the janitor fails
// during steady state (not during shutdown).
func TestRun_StopsOnJanitorFailure(t *testing.T) {
	const sentinelErr = "janitor failed"

	// Override the janitor start function to immediately return an error.
	originalStartRuleYJanitor := startRuleYJanitor
	defer func() { startRuleYJanitor = originalStartRuleYJanitor }()

	startRuleYJanitor = func(ctx context.Context, j *coredb.RuleYJanitor) error {
		return errors.New(sentinelErr)
	}

	// Use a unique temp directory to avoid SQLite locking issues with concurrent tests.
	tempDir := t.TempDir()
	cfg := Config{
		Bind:    "127.0.0.1:0",
		Profile: "permissive",
		DataDir: tempDir,
	}
	cfg = cfg.normalize()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := Run(ctx, cfg)

	if err == nil {
		t.Fatal("expected Run to return an error")
	}
	if !strings.Contains(err.Error(), sentinelErr) {
		t.Fatalf("expected error to contain %q, got %v", sentinelErr, err)
	}
}

// TestSourcetoProvenance tests the sourcetoProvenance helper function.
func TestSourcetoProvenance(t *testing.T) {
	tests := []struct {
		name string
		src  sourcestore.Source
		want map[string]any
	}{
		{
			name: "minimal source",
			src:  sourcestore.Source{Name: "test", Type: "git"},
			want: map[string]any{"name": "test", "type": "git"},
		},
		{
			name: "source with ref and url",
			src:  sourcestore.Source{Name: "test", Type: "git", Ref: "main", URL: "https://example.com/repo.git"},
			want: map[string]any{"name": "test", "type": "git", "ref": "main", "url": "https://example.com/repo.git"},
		},
		{
			name: "source with resolved_ref from ResolvedRef",
			src:  sourcestore.Source{Name: "test", Type: "git", ResolvedRef: "abc123"},
			want: map[string]any{"name": "test", "type": "git", "resolved_ref": "abc123"},
		},
		{
			name: "source with resolved_ref from Digest when ResolvedRef empty",
			src:  sourcestore.Source{Name: "test", Type: "git", Digest: "sha256:def456"},
			want: map[string]any{"name": "test", "type": "git", "digest": "sha256:def456", "resolved_ref": "sha256:def456"},
		},
		{
			name: "source with Provenance merge",
			src: sourcestore.Source{
				Name:        "test",
				Type:        "git",
				ResolvedRef: "abc123",
				Provenance:  map[string]any{"key1": "val1", "key2": "val2"},
			},
			want: map[string]any{"name": "test", "type": "git", "resolved_ref": "abc123", "key1": "val1", "key2": "val2"},
		},
		{
			name: "source with Provenance does not override existing keys",
			src: sourcestore.Source{
				Name:        "test",
				Type:        "git",
				ResolvedRef: "abc123",
				Provenance:  map[string]any{"name": "override", "key1": "val1"},
			},
			want: map[string]any{"name": "test", "type": "git", "resolved_ref": "abc123", "key1": "val1"},
		},
		{
			name: "source with aliases",
			src: sourcestore.Source{
				Name:    "test",
				Type:    "git",
				Aliases: []types.CommandAlias{{From: "v1", To: "main", Description: "alias v1 to main"}},
			},
			want: map[string]any{"name": "test", "type": "git", "aliases": []map[string]string{{"from": "v1", "to": "main", "description": "alias v1 to main"}}},
		},
		{
			name: "source with trust",
			src: sourcestore.Source{
				Name:  "test",
				Type:  "git",
				Trust: map[string]any{"key": "value"},
			},
			want: map[string]any{"name": "test", "type": "git", "trust": map[string]any{"key": "value"}},
		},
		{
			name: "source with metadata",
			src: sourcestore.Source{
				Name:     "test",
				Type:     "git",
				Metadata: map[string]any{"version": "1.0"},
			},
			want: map[string]any{"name": "test", "type": "git", "metadata": map[string]any{"version": "1.0"}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sourcetoProvenance(tt.src)
			if !deepEqualMap(got, tt.want) {
				t.Errorf("sourcetoProvenance() = %v, want %v", got, tt.want)
			}
		})
	}
}

// deepEqualMap compares two maps for equality.
func deepEqualMap(a, b map[string]any) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if !reflectDeepEqual(v, b[k]) {
			return false
		}
	}
	return true
}

// reflectDeepEqual performs deep comparison for any type.
func reflectDeepEqual(x, y any) bool {
	if x == nil || y == nil {
		return x == y
	}
	switch xv := x.(type) {
	case map[string]any:
		yv, ok := y.(map[string]any)
		if !ok {
			return false
		}
		return deepEqualMap(xv, yv)
	case []any:
		yv, ok := y.([]any)
		if !ok {
			return false
		}
		if len(xv) != len(yv) {
			return false
		}
		for i := range xv {
			if !reflectDeepEqual(xv[i], yv[i]) {
				return false
			}
		}
		return true
	case []map[string]string:
		yv, ok := y.([]map[string]string)
		if !ok {
			return false
		}
		if len(xv) != len(yv) {
			return false
		}
		for i := range xv {
			if !deepEqualStringMap(xv[i], yv[i]) {
				return false
			}
		}
		return true
	default:
		return x == y
	}
}

// deepEqualStringMap compares two string maps for equality.
func deepEqualStringMap(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if v != b[k] {
			return false
		}
	}
	return true
}

// TestLoadPolicyContextWithInvalidFile tests loadPolicyContext behavior with invalid/unreadable policy files.
func TestLoadPolicyContextWithInvalidFile(t *testing.T) {
	// Use a non-existent file path.
	t.Setenv("FLWD_POLICY_FILE", "/nonexistent/policy/file.yaml")

	ctx := context.Background()
	policyCtx, err := loadPolicyContext(ctx, "secure", nil)

	if err == nil {
		t.Fatal("expected error when policy file does not exist")
	}
	if !strings.Contains(err.Error(), "load policy bundle") {
		t.Fatalf("expected 'load policy bundle' error, got %v", err)
	}
	if policyCtx != nil {
		t.Fatal("expected nil policy context on error")
	}
}

// TestLoadPolicyContextWithInvalidContent tests loadPolicyContext behavior with invalid YAML content.
func TestLoadPolicyContextWithInvalidContent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "invalid.yaml")
	if err := os.WriteFile(path, []byte("{{invalid yaml {{"), 0o600); err != nil {
		t.Fatalf("write policy: %v", err)
	}
	t.Setenv("FLWD_POLICY_FILE", path)

	ctx := context.Background()
	policyCtx, err := loadPolicyContext(ctx, "secure", &bundleVerifierStub{})

	if err == nil {
		t.Fatal("expected error when policy file contains invalid YAML")
	}
	if !strings.Contains(err.Error(), "load policy bundle") {
		t.Fatalf("expected 'load policy bundle' error, got %v", err)
	}
	if policyCtx != nil {
		t.Fatal("expected nil policy context on error")
	}
}

// TestLoadPolicyContextNonSecureNoVerifier tests that non-secure profile does not call verifier.
func TestLoadPolicyContextNonSecureNoVerifier(t *testing.T) {
	policyPath := writePolicyFile(t, `allowed_registries: ["registry.corp.example"]`)
	t.Setenv("FLWD_POLICY_FILE", policyPath)

	stub := &bundleVerifierStub{err: errors.New("should not be called")}
	ctx := context.Background()
	policyCtx, err := loadPolicyContext(ctx, "permissive", stub)

	if err != nil {
		t.Fatalf("loadPolicyContext: %v", err)
	}
	if policyCtx == nil {
		t.Fatal("expected non-nil policy context for permissive profile")
	}
	if stub.called {
		t.Fatal("expected bundle verifier not to be called for permissive profile")
	}
}

// TestLoadPolicyContextSecureWithInvalidBundle tests that secure profile returns error when bundle verification fails.
func TestLoadPolicyContextSecureWithInvalidBundle(t *testing.T) {
	policyPath := writePolicyFile(t, `allowed_registries: ["registry.corp.example"]`)
	t.Setenv("FLWD_POLICY_FILE", policyPath)

	stub := &bundleVerifierStub{err: errors.New("invalid signature")}
	ctx := context.Background()
	_, err := loadPolicyContext(ctx, "secure", stub)

	if err == nil {
		t.Fatal("expected error when bundle verification fails")
	}
	if !strings.Contains(err.Error(), "invalid signature") {
		t.Fatalf("expected underlying verifier error, got %v", err)
	}
}

// TestLoadPolicyContextReturnsNonNilWhenNoPolicyFile tests that loadPolicyContext returns non-nil context when no policy file is set.
func TestLoadPolicyContextReturnsNonNilWhenNoPolicyFile(t *testing.T) {
	// Ensure FLWD_POLICY_FILE is not set.
	t.Setenv("FLWD_POLICY_FILE", "")

	ctx := context.Background()
	policyCtx, err := loadPolicyContext(ctx, "secure", nil)

	if err != nil {
		t.Fatalf("loadPolicyContext: %v", err)
	}
	if policyCtx == nil {
		t.Fatal("expected non-nil policy context when no policy file is set")
	}
}

// TestLoadPolicyContextWithStubs verifies that the bundle verifier stub works correctly.
func TestLoadPolicyContextWithStubs(t *testing.T) {
	policyPath := writePolicyFile(t, `allowed_registries: ["registry.corp.example"]`)
	t.Setenv("FLWD_POLICY_FILE", policyPath)

	stub := &bundleVerifierStub{}
	ctx := context.Background()
	policyCtx, err := loadPolicyContext(ctx, "secure", stub)
	if err != nil {
		t.Fatalf("loadPolicyContext: %v", err)
	}
	if policyCtx == nil {
		t.Fatal("expected non-nil policy context")
	}
	if !stub.called {
		t.Fatal("expected bundle verifier to be invoked")
	}
	if stub.path != policyPath {
		t.Fatalf("expected verifier path %q, got %q", policyPath, stub.path)
	}

	// Verify stub satisfies BundleVerifier interface.
	var _ verify.BundleVerifier = stub
}

func TestStartRuleYJanitorReal_ReturnsRuleYUnavailable(t *testing.T) {
	t.Parallel()

	err := startRuleYJanitorReal(context.Background(), &coredb.RuleYJanitor{})
	if !errors.Is(err, coredb.ErrRuleYUnavailable) {
		t.Fatalf("expected ErrRuleYUnavailable, got %v", err)
	}
}

func TestSourcetoProvenance_MapsFields(t *testing.T) {
	t.Parallel()

	src := sourcestore.Source{
		Name:        "test",
		Type:        "git",
		Ref:         "main",
		URL:         "https://example.com/repo",
		ResolvedRef: "",
		Digest:      "sha256:abc123",
	}
	result := sourcetoProvenance(src)

	if result["name"] != "test" {
		t.Errorf("expected name 'test', got %v", result["name"])
	}
	if result["type"] != "git" {
		t.Errorf("expected type 'git', got %v", result["type"])
	}
	if result["ref"] != "main" {
		t.Errorf("expected ref 'main', got %v", result["ref"])
	}
	if result["url"] != "https://example.com/repo" {
		t.Errorf("expected url 'https://example.com/repo', got %v", result["url"])
	}
	if result["digest"] != "sha256:abc123" {
		t.Errorf("expected digest 'sha256:abc123', got %v", result["digest"])
	}
	// Digest implies resolved_ref when empty
	if result["resolved_ref"] != "sha256:abc123" {
		t.Errorf("expected resolved_ref from digest, got %v", result["resolved_ref"])
	}
}
