package requestctx

import (
	"bytes"
	"context"
	"log/slog"
	"testing"
)

func TestTenantAbsentWithoutContext(t *testing.T) {
	if _, ok := Tenant(context.TODO()); ok {
		t.Fatalf("expected tenant to be absent when context is empty")
	}
	if _, ok := Tenant(context.Background()); ok {
		t.Fatalf("expected tenant to be absent when context has no tenant")
	}
}

func TestTenantAbsentWithPrincipalOnly(t *testing.T) {
	ctx := WithPrincipal(context.Background(), "tester")
	if _, ok := Tenant(ctx); ok {
		t.Fatalf("expected tenant to be absent when only principal is present")
	}
}

func TestTenantPresent(t *testing.T) {
	ctx := WithTenant(context.Background(), "acme")
	got, ok := Tenant(ctx)
	if !ok {
		t.Fatalf("expected tenant to be present")
	}
	if got != "acme" {
		t.Fatalf("expected tenant %q, got %q", "acme", got)
	}
}

func TestEffectiveProfileEmpty(t *testing.T) {
	ctx := context.Background()
	got, ok := EffectiveProfile(ctx)
	if ok || got != "" {
		t.Fatalf("expected empty profile to be no-op")
	}
}

func TestEffectiveProfileNilContext(t *testing.T) {
	ctx := context.TODO()
	got, ok := EffectiveProfile(ctx)
	if ok || got != "" {
		t.Fatalf("expected nil context to return (\"\")/false")
	}
}

func TestEffectiveProfileWithProfile(t *testing.T) {
	ctx := WithEffectiveProfile(context.Background(), "strict")
	got, ok := EffectiveProfile(ctx)
	if !ok {
		t.Fatalf("expected profile to be present")
	}
	if got != "strict" {
		t.Fatalf("expected profile %q, got %q", "strict", got)
	}
}

func TestWithMetadataNil(t *testing.T) {
	ctx := context.Background()
	result := WithMetadata(ctx, nil)
	if result != ctx {
		t.Fatalf("expected nil metadata to return unchanged context")
	}
}

func TestMetadataFromContextNil(t *testing.T) {
	got := MetadataFromContext(context.TODO())
	if got != nil {
		t.Fatalf("expected nil context to return nil metadata")
	}
}

func TestMetadataFromContextEmpty(t *testing.T) {
	ctx := context.Background()
	got := MetadataFromContext(ctx)
	if got != nil {
		t.Fatalf("expected empty context to return nil metadata")
	}
}

func TestWithRuntime(t *testing.T) {
	ctx := WithRuntime(context.Background(), "k8s")
	meta := MetadataFromContext(ctx)
	if meta == nil {
		t.Fatalf("expected metadata to be created")
	}
	if meta.Runtime != "k8s" {
		t.Fatalf("expected runtime %q, got %q", "k8s", meta.Runtime)
	}
}

func TestWithRuntimeEmpty(t *testing.T) {
	ctx := context.Background()
	result := WithRuntime(ctx, "")
	if result != ctx {
		t.Fatalf("expected empty runtime to return unchanged context")
	}
}

func TestRuntime(t *testing.T) {
	ctx := WithRuntime(context.Background(), "k8s")
	got, ok := Runtime(ctx)
	if !ok {
		t.Fatalf("expected runtime to be present")
	}
	if got != "k8s" {
		t.Fatalf("expected runtime %q, got %q", "k8s", got)
	}
}

func TestRuntimeAbsent(t *testing.T) {
	ctx := context.Background()
	got, ok := Runtime(ctx)
	if ok || got != "" {
		t.Fatalf("expected absent runtime to return (\"\")/false")
	}
}

func TestWithRequestID(t *testing.T) {
	ctx := WithRequestID(context.Background(), "req-123")
	meta := MetadataFromContext(ctx)
	if meta == nil {
		t.Fatalf("expected metadata to be created")
	}
	if meta.RequestID != "req-123" {
		t.Fatalf("expected requestID %q, got %q", "req-123", meta.RequestID)
	}
}

func TestWithRequestIDEmpty(t *testing.T) {
	ctx := context.Background()
	result := WithRequestID(ctx, "")
	if result != ctx {
		t.Fatalf("expected empty requestID to return unchanged context")
	}
}

func TestRequestID(t *testing.T) {
	ctx := WithRequestID(context.Background(), "req-123")
	got, ok := RequestID(ctx)
	if !ok {
		t.Fatalf("expected requestID to be present")
	}
	if got != "req-123" {
		t.Fatalf("expected requestID %q, got %q", "req-123", got)
	}
}

func TestRequestIDAbsent(t *testing.T) {
	ctx := context.Background()
	got, ok := RequestID(ctx)
	if ok || got != "" {
		t.Fatalf("expected absent requestID to return (\"\")/false")
	}
}

func TestWithRoute(t *testing.T) {
	ctx := WithRoute(context.Background(), "/api/v1/{id}")
	meta := MetadataFromContext(ctx)
	if meta == nil {
		t.Fatalf("expected metadata to be created")
	}
	if meta.Route != "/api/v1/{id}" {
		t.Fatalf("expected route %q, got %q", "/api/v1/{id}", meta.Route)
	}
}

func TestWithRouteEmpty(t *testing.T) {
	ctx := context.Background()
	result := WithRoute(ctx, "")
	if result != ctx {
		t.Fatalf("expected empty route to return unchanged context")
	}
}

func TestRoute(t *testing.T) {
	ctx := WithRoute(context.Background(), "/api/v1/{id}")
	got, ok := Route(ctx)
	if !ok {
		t.Fatalf("expected route to be present")
	}
	if got != "/api/v1/{id}" {
		t.Fatalf("expected route %q, got %q", "/api/v1/{id}", got)
	}
}

func TestRouteAbsent(t *testing.T) {
	ctx := context.Background()
	got, ok := Route(ctx)
	if ok || got != "" {
		t.Fatalf("expected absent route to return (\"\")/false")
	}
}

func TestPrincipal(t *testing.T) {
	ctx := WithPrincipal(context.Background(), "user-123")
	got, ok := Principal(ctx)
	if !ok {
		t.Fatalf("expected principal to be present")
	}
	if got != "user-123" {
		t.Fatalf("expected principal %q, got %q", "user-123", got)
	}
}

func TestPrincipalEmpty(t *testing.T) {
	ctx := context.Background()
	result := WithPrincipal(ctx, "")
	if result != ctx {
		t.Fatalf("expected empty principal to return unchanged context")
	}
}

func TestPrincipalNilContext(t *testing.T) {
	got, ok := Principal(context.TODO())
	if ok || got != "" {
		t.Fatalf("expected empty context to return (\"\")/false")
	}
}

func TestWithScrubbedLoggerNilScrub(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))
	ctx := WithLogger(context.Background(), logger)
	result := WithScrubbedLogger(ctx, nil)
	if result != ctx {
		t.Fatalf("expected nil scrubber to return unchanged context")
	}
}

func TestWithScrubbedLoggerNilLogger(t *testing.T) {
	ctx := context.Background()
	result := WithScrubbedLogger(ctx, func(s string) string { return s })
	if result != ctx {
		t.Fatalf("expected nil logger to return unchanged context")
	}
}

func TestLogPolicyDecisionNilLogger(t *testing.T) {
	ctx := context.Background()
	// Should not panic
	LogPolicyDecision(ctx, "subject", "allowed", "", "reason")
}

func TestLogPolicyDecisionDenied(t *testing.T) {
	var buf bytes.Buffer
	handler := slog.NewTextHandler(&buf, nil)
	logger := slog.New(handler)

	ctx := WithLogger(context.Background(), logger)
	LogPolicyDecision(ctx, "subject", "denied", "ACCESS_DENIED", "insufficient permissions")

	if !bytes.Contains(buf.Bytes(), []byte("policy_decision")) {
		t.Fatalf("expected policy_decision log for denied")
	}
}

func TestLogPolicyDecisionWarn(t *testing.T) {
	var buf bytes.Buffer
	handler := slog.NewTextHandler(&buf, nil)
	logger := slog.New(handler)

	ctx := WithLogger(context.Background(), logger)
	LogPolicyDecision(ctx, "subject", "warn", "", "")

	if !bytes.Contains(buf.Bytes(), []byte("policy_decision")) {
		t.Fatalf("expected policy_decision log for warn")
	}
}

func TestLogPolicyDecisionWarning(t *testing.T) {
	var buf bytes.Buffer
	handler := slog.NewTextHandler(&buf, nil)
	logger := slog.New(handler)

	ctx := WithLogger(context.Background(), logger)
	LogPolicyDecision(ctx, "subject", "warning", "", "")

	if !bytes.Contains(buf.Bytes(), []byte("policy_decision")) {
		t.Fatalf("expected policy_decision log for warning")
	}
}

func TestLogPolicyDecisionAllowed(t *testing.T) {
	var buf bytes.Buffer
	handler := slog.NewTextHandler(&buf, nil)
	logger := slog.New(handler)

	ctx := WithLogger(context.Background(), logger)
	LogPolicyDecision(ctx, "subject", "allowed", "", "")

	if !bytes.Contains(buf.Bytes(), []byte("policy_decision")) {
		t.Fatalf("expected policy_decision log for allowed")
	}
}

func TestLogPolicyDecisionWithProfile(t *testing.T) {
	var buf bytes.Buffer
	handler := slog.NewTextHandler(&buf, nil)
	logger := slog.New(handler)

	ctx := WithEffectiveProfile(WithLogger(context.Background(), logger), "strict")
	LogPolicyDecision(ctx, "subject", "allowed", "", "")

	if !bytes.Contains(buf.Bytes(), []byte("profile.effective")) {
		t.Fatalf("expected profile.effective in log output")
	}
}

func TestMetadataHelpersCreateOnWrite(t *testing.T) {
	ctx := context.Background()
	ctx = WithRuntime(ctx, "k8s")
	ctx = WithRequestID(ctx, "req-123")
	ctx = WithRoute(ctx, "/api/v1/{id}")

	meta := MetadataFromContext(ctx)
	if meta == nil {
		t.Fatalf("expected metadata to be created")
	}
	if meta.Runtime != "k8s" || meta.RequestID != "req-123" || meta.Route != "/api/v1/{id}" {
		t.Fatalf("expected all fields set, got %+v", meta)
	}
}

func TestMetadataNilContext(t *testing.T) {
	got := MetadataFromContext(nil)
	if got != nil {
		t.Fatalf("expected nil context to return nil metadata")
	}
}
