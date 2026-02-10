package requestctx

import (
	"context"
	"testing"
)

func TestTenantAbsentWithoutContext(t *testing.T) {
	if _, ok := Tenant(nil); ok {
		t.Fatalf("expected tenant to be absent when context is nil")
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
