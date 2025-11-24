package server

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/flowd-org/flowd/internal/coredb"
	"github.com/flowd-org/flowd/internal/policy"
	"github.com/flowd-org/flowd/internal/policy/verify"
	"github.com/flowd-org/flowd/internal/server/metrics"
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
				"core_triggers":         {LimitBytes: 9},
				"core_invocation_state": {LimitBytes: defaultRuleYLimitBytes},
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
	if problem["type"] != "https://flowd.dev/problems/namespace-forbidden" {
		t.Fatalf("expected namespace-forbidden problem, got %v", problem["type"])
	}

	// Quota exceeded on second write
	quota := httptest.NewRecorder()
	handler.ServeHTTP(quota, makeReq(http.MethodPut, "/kv/core_triggers/b", putBody("abcd"), "ruley:write"))
	if quota.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429 when quota exceeded, got %d", quota.Code)
	}
}
