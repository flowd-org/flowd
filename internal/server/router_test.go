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
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/flowd-org/flowd/internal/artifacts"
	"github.com/flowd-org/flowd/internal/coredb"
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

// Test_sourcetoProvenance tests the sourcetoProvenance helper function.
func Test_sourcetoProvenance(t *testing.T) {
	tests := []struct {
		name   string
		source sourcestore.Source
		want   map[string]any
	}{
		{
			name: "minimal source",
			source: sourcestore.Source{
				Name: "test-source",
				Type: "git",
			},
			want: map[string]any{
				"name": "test-source",
				"type": "git",
			},
		},
		{
			name: "source with ref and url",
			source: sourcestore.Source{
				Name: "test-source",
				Type: "oci",
				Ref:  "v1.0.0",
				URL:  "https://example.com/repo",
			},
			want: map[string]any{
				"name": "test-source",
				"type": "oci",
				"ref":  "v1.0.0",
				"url":  "https://example.com/repo",
			},
		},
		{
			name: "source with resolved ref and commit",
			source: sourcestore.Source{
				Name:           "test-source",
				Type:           "git",
				ResolvedRef:    "refs/heads/main",
				ResolvedCommit: "abc123def456",
			},
			want: map[string]any{
				"name":            "test-source",
				"type":            "git",
				"resolved_ref":    "refs/heads/main",
				"resolved_commit": "abc123def456",
			},
		},
		{
			name: "source with digest",
			source: sourcestore.Source{
				Name:   "test-source",
				Type:   "oci",
				Digest: "sha256:abcdef123456",
			},
			want: map[string]any{
				"name":   "test-source",
				"type":   "oci",
				"digest": "sha256:abcdef123456",
			},
		},
		{
			name: "source with resolved_ref set to digest when empty",
			source: sourcestore.Source{
				Name:        "test-source",
				Type:        "oci",
				Digest:      "sha256:abcdef123456",
				ResolvedRef: "",
			},
			want: map[string]any{
				"name":   "test-source",
				"type":   "oci",
				"digest": "sha256:abcdef123456",
			},
		},
		{
			name: "source with pull policy and verify signatures",
			source: sourcestore.Source{
				Name:             "test-source",
				Type:             "oci",
				PullPolicy:       "always",
				VerifySignatures: true,
			},
			want: map[string]any{
				"name":              "test-source",
				"type":              "oci",
				"pull_policy":       "always",
				"verify_signatures": true,
			},
		},
		{
			name: "source with provenance",
			source: sourcestore.Source{
				Name: "test-source",
				Type: "git",
				Provenance: map[string]any{
					"build_id":   "12345",
					"builder":    "github-actions",
					"source_ref": "main",
				},
			},
			want: map[string]any{
				"name":       "test-source",
				"type":       "git",
				"build_id":   "12345",
				"builder":    "github-actions",
				"source_ref": "main",
			},
		},
		{
			name: "source with provenance merging (no conflict)",
			source: sourcestore.Source{
				Name: "test-source",
				Type: "git",
				Provenance: map[string]any{
					"custom_key": "custom_value",
				},
			},
			want: map[string]any{
				"name":       "test-source",
				"type":       "git",
				"custom_key": "custom_value",
			},
		},
		{
			name: "source with provenance merging (conflict - source wins)",
			source: sourcestore.Source{
				Name: "test-source",
				Type: "oci",
				Provenance: map[string]any{
					"name":  "should_not_override",
					"extra": "additional",
				},
			},
			want: map[string]any{
				"name":  "test-source", // source field wins
				"type":  "oci",
				"extra": "additional", // provenance field added
			},
		},
		{
			name: "source with aliases",
			source: sourcestore.Source{
				Name: "test-source",
				Type: "git",
				Aliases: []types.CommandAlias{
					{From: "build", To: "make build", Description: "Build the project"},
					{From: "test", To: "make test", Description: "Run tests"},
				},
			},
			want: map[string]any{
				"name": "test-source",
				"type": "git",
				"aliases": []map[string]string{
					{"from": "build", "to": "make build", "description": "Build the project"},
					{"from": "test", "to": "make test", "description": "Run tests"},
				},
			},
		},
		{
			name: "source with trust",
			source: sourcestore.Source{
				Name: "test-source",
				Type: "oci",
				Trust: map[string]any{
					"oidc_issuer": "https://accounts.google.com",
					"repository":  "my-repo",
				},
			},
			want: map[string]any{
				"name": "test-source",
				"type": "oci",
				"trust": map[string]any{
					"oidc_issuer": "https://accounts.google.com",
					"repository":  "my-repo",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sourcetoProvenance(tt.source)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("sourcetoProvenance() = %v, want %v", got, tt.want)
			}
		})
	}
}

// Test_loadPolicyContext tests the loadPolicyContext helper function.
func Test_loadPolicyContext(t *testing.T) {
	tests := []struct {
		name           string
		policyFile     string
		profile        string
		bundleVerifier policyverify.BundleVerifier
		expectError    bool
	}{
		{
			name:           "no policy file - returns non-nil context",
			policyFile:     "",
			profile:        "permissive",
			bundleVerifier: nil,
			expectError:    false,
		},
		{
			name:           "invalid policy file path - returns error",
			policyFile:     "/nonexistent/policy.yaml",
			profile:        "permissive",
			bundleVerifier: nil,
			expectError:    true,
		},
		{
			name:           "secure profile with bundle verifier stub",
			policyFile:     "",
			profile:        "secure",
			bundleVerifier: &bundleVerifierStub{},
			expectError:    false,
		},
		{
			name:           "non-secure profile - does not call verifier",
			policyFile:     "",
			profile:        "permissive",
			bundleVerifier: &bundleVerifierStub{},
			expectError:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()

			if tt.policyFile != "" {
				t.Setenv("FLWD_POLICY_FILE", tt.policyFile)
			} else {
				os.Unsetenv("FLWD_POLICY_FILE")
			}

			policyCtx, err := loadPolicyContext(ctx, tt.profile, tt.bundleVerifier)

			if tt.expectError {
				if err == nil {
					t.Fatal("expected error but got none")
				}
				if !strings.Contains(err.Error(), "load policy bundle") &&
					!strings.Contains(err.Error(), "verify policy bundle") {
					t.Fatalf("expected load/verify error, got: %v", err)
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if policyCtx == nil {
					t.Fatal("expected non-nil policy context")
				}
			}
		})
	}
}
