package handlers

import (
	"context"
	"testing"

	"github.com/flowd-org/flowd/internal/server/requestctx"
	"github.com/flowd-org/flowd/internal/server/response"
)

func TestResolveTenant_NoPrincipalTenant_UsesRequestTenant(t *testing.T) {
	ctx := context.Background()
	resolved, prob := resolveTenant(ctx, "acme")

	if resolved != "acme" {
		t.Errorf("expected resolved='acme', got %q", resolved)
	}
	if prob != nil {
		t.Errorf("expected no problem, got %+v", prob)
	}
}

func TestResolveTenant_NoPrincipalTenant_DefaultsWhenEmptyRequest(t *testing.T) {
	ctx := context.Background()
	resolved, prob := resolveTenant(ctx, "")

	if resolved != defaultTenant {
		t.Errorf("expected resolved=%q (default), got %q", defaultTenant, resolved)
	}
	if prob != nil {
		t.Errorf("expected no problem, got %+v", prob)
	}
}

func TestResolveTenant_PrincipalTenant_EmptyRequestReturnsPrincipalTenant(t *testing.T) {
	ctx := requestctx.WithTenant(context.Background(), "acme")
	resolved, prob := resolveTenant(ctx, "")

	if resolved != "acme" {
		t.Errorf("expected resolved='acme', got %q", resolved)
	}
	if prob != nil {
		t.Errorf("expected no problem, got %+v", prob)
	}
}

func TestResolveTenant_PrincipalTenant_MatchingRequestReturnsPrincipal(t *testing.T) {
	ctx := requestctx.WithTenant(context.Background(), "acme")
	resolved, prob := resolveTenant(ctx, "acme")

	if resolved != "acme" {
		t.Errorf("expected resolved='acme', got %q", resolved)
	}
	if prob != nil {
		t.Errorf("expected no problem, got %+v", prob)
	}
}

func TestResolveTenant_PrincipalTenant_MismatchReturnsProblem(t *testing.T) {
	ctx := requestctx.WithTenant(context.Background(), "acme")
	resolved, prob := resolveTenant(ctx, "other")

	if resolved != "" {
		t.Errorf("expected empty resolved on error, got %q", resolved)
	}
	if prob == nil {
		t.Fatal("expected problem on tenant mismatch")
	}
	if prob.Type != response.ProblemTypeTenantMismatch {
		t.Errorf("expected Type=%q, got %q", response.ProblemTypeTenantMismatch, prob.Type)
	}
}

func TestResolveTenant_NoPrincipal_HasPrincipalDefaults(t *testing.T) {
	ctx := requestctx.WithPrincipal(context.Background(), "user123")
	resolved, prob := resolveTenant(ctx, "")

	if resolved != defaultTenant {
		t.Errorf("expected resolved=%q (default), got %q", defaultTenant, resolved)
	}
	if prob != nil {
		t.Errorf("expected no problem, got %+v", prob)
	}
}
