package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/flowd-org/flowd/internal/configloader"
	"github.com/flowd-org/flowd/internal/server/requestctx"
	"github.com/flowd-org/flowd/internal/server/response"
)

func TestTenantMismatchProblemProperties(t *testing.T) {
	ctx := requestctx.WithTenant(requestctx.WithPrincipal(context.Background(), "user"), "acme")
	_, prob := resolveTenant(ctx, "other")
	if prob == nil {
		t.Fatalf("expected tenant mismatch problem")
	}
	if prob.Status != http.StatusForbidden {
		t.Fatalf("expected status %d, got %d", http.StatusForbidden, prob.Status)
	}
	if prob.Type != response.ProblemTypeTenantMismatch {
		t.Fatalf("expected type %q, got %q", response.ProblemTypeTenantMismatch, prob.Type)
	}
	if prob.Ext["code"] != response.ProblemCodeTenantMismatch {
		t.Fatalf("expected code %q, got %v", response.ProblemCodeTenantMismatch, prob.Ext["code"])
	}
	if prob.Ext["request_tenant"] != "other" {
		t.Fatalf("expected request_tenant other, got %v", prob.Ext["request_tenant"])
	}
	if prob.Ext["principal_tenant"] != "acme" {
		t.Fatalf("expected principal_tenant acme, got %v", prob.Ext["principal_tenant"])
	}
	rec := httptest.NewRecorder()
	response.Write(rec, *prob)
	if rec.Header().Get("Content-Type") != "application/problem+json" {
		t.Fatalf("expected application/problem+json, got %q", rec.Header().Get("Content-Type"))
	}
}

func TestResolveTenant_CaseSensitiveAfterTrimming(t *testing.T) {
	ctx := requestctx.WithTenant(requestctx.WithPrincipal(context.Background(), "user"), "acme")

	resolved, prob := resolveTenant(ctx, " acme ")
	if prob != nil {
		t.Fatalf("expected trimmed exact tenant to resolve, got problem %v", prob)
	}
	if resolved != "acme" {
		t.Fatalf("expected resolved tenant acme, got %q", resolved)
	}

	_, prob = resolveTenant(ctx, " ACME ")
	if prob == nil {
		t.Fatalf("expected tenant mismatch for case-only difference")
	}
	if prob.Ext["request_tenant"] != "ACME" {
		t.Fatalf("expected trimmed request_tenant ACME, got %v", prob.Ext["request_tenant"])
	}
}

func TestDiscoveryProblemDualConfigProperties(t *testing.T) {
	err := &configloader.DualConfigError{
		ScriptDir:   "/tmp/scripts/demo",
		PrimaryPath: "/tmp/scripts/demo/config.yaml",
		LegacyPath:  "/tmp/scripts/demo/config.d/config.yaml",
	}
	prob, ok := discoveryProblem(err)
	if !ok || prob == nil {
		t.Fatalf("expected discovery problem for dual config error")
	}
	if prob.Status != http.StatusConflict {
		t.Fatalf("expected status %d, got %d", http.StatusConflict, prob.Status)
	}
	if prob.Type != response.ProblemTypeJobConfigDualSentinel {
		t.Fatalf("expected type %q, got %q", response.ProblemTypeJobConfigDualSentinel, prob.Type)
	}
	if prob.Ext["code"] != configloader.DualConfigCode {
		t.Fatalf("expected code %q, got %v", configloader.DualConfigCode, prob.Ext["code"])
	}
	if prob.Ext["primary_path"] != err.PrimaryPath {
		t.Fatalf("expected primary_path %q, got %v", err.PrimaryPath, prob.Ext["primary_path"])
	}
	if prob.Ext["legacy_path"] != err.LegacyPath {
		t.Fatalf("expected legacy_path %q, got %v", err.LegacyPath, prob.Ext["legacy_path"])
	}
	rec := httptest.NewRecorder()
	response.Write(rec, *prob)
	if rec.Header().Get("Content-Type") != "application/problem+json" {
		t.Fatalf("expected application/problem+json, got %q", rec.Header().Get("Content-Type"))
	}
}

func TestJobIDCollisionProblemProperties(t *testing.T) {
	contenders := []jobCollisionContender{
		{
			CanonicalJobID: "demo",
			Origin: jobCollisionOrigin{
				SourceKind: "fs",
				SourceName: "local",
			},
			MountPath: ".",
			JobDir:    ".",
		},
		{
			CanonicalJobID: "demo",
			Origin: jobCollisionOrigin{
				SourceKind: "git",
				SourceName: "repo",
			},
			MountPath: "scripts",
			JobDir:    "demo",
		},
	}
	prob := jobIDCollisionProblem("demo", contenders)
	if prob.Status != http.StatusConflict {
		t.Fatalf("expected status %d, got %d", http.StatusConflict, prob.Status)
	}
	if prob.Type != response.ProblemTypeJobIDCollision {
		t.Fatalf("expected type %q, got %q", response.ProblemTypeJobIDCollision, prob.Type)
	}
	if prob.Ext["code"] != response.ProblemCodeJobIDCollision {
		t.Fatalf("expected code %q, got %v", response.ProblemCodeJobIDCollision, prob.Ext["code"])
	}
	if prob.Ext["canonical_job_id"] != "demo" {
		t.Fatalf("expected canonical_job_id demo, got %v", prob.Ext["canonical_job_id"])
	}
	list, ok := prob.Ext["contenders"].([]jobCollisionContender)
	if !ok {
		t.Fatalf("expected contenders slice, got %T", prob.Ext["contenders"])
	}
	if len(list) != 2 {
		t.Fatalf("expected 2 contenders, got %d", len(list))
	}
	rec := httptest.NewRecorder()
	response.Write(rec, prob)
	if rec.Header().Get("Content-Type") != "application/problem+json" {
		t.Fatalf("expected application/problem+json, got %q", rec.Header().Get("Content-Type"))
	}
}
