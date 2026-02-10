package server

import (
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/flowd-org/flowd/internal/server/requestctx"
)

func TestAuthMiddlewareRequiresToken(t *testing.T) {
	t.Setenv("FLWD_JWT_SECRET", "")
	mw := authMiddleware(Config{})
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest(http.MethodGet, "/jobs", nil)
	resp := httptest.NewRecorder()
	handler.ServeHTTP(resp, req)
	if resp.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for missing token, got %d", resp.Code)
	}
	if challenge := resp.Header().Get("WWW-Authenticate"); challenge == "" {
		t.Fatalf("expected WWW-Authenticate header")
	}
}

func TestAuthMiddlewareAllowsDevModeWithoutToken(t *testing.T) {
	mw := authMiddleware(Config{Dev: true})
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest(http.MethodGet, "/jobs", nil)
	resp := httptest.NewRecorder()
	handler.ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200 in dev mode, got %d", resp.Code)
	}
}

func TestAuthMiddlewareScopes(t *testing.T) {
	mw := authMiddleware(Config{})
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest(http.MethodGet, "/jobs", nil)
	req.Header.Set("Authorization", "Bearer jobs:read runs:write")
	resp := httptest.NewRecorder()
	handler.ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200 with jobs:read scope, got %d", resp.Code)
	}
}

func TestAuthMiddlewareHealthStorageScope(t *testing.T) {
	mw := authMiddleware(Config{})
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest(http.MethodGet, "/health/storage", nil)
	req.Header.Set("Authorization", "Bearer jobs:read")
	resp := httptest.NewRecorder()
	handler.ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200 with jobs:read scope, got %d", resp.Code)
	}

	reqMissing := httptest.NewRequest(http.MethodGet, "/health/storage", nil)
	reqMissing.Header.Set("Authorization", "Bearer runs:read")
	respMissing := httptest.NewRecorder()
	handler.ServeHTTP(respMissing, reqMissing)
	if respMissing.Code != http.StatusForbidden {
		t.Fatalf("expected 403 when jobs:read scope missing, got %d", respMissing.Code)
	}
}

func TestAuthMiddlewareForbidden(t *testing.T) {
	mw := authMiddleware(Config{})
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest(http.MethodGet, "/jobs", nil)
	req.Header.Set("Authorization", "Bearer runs:write")
	resp := httptest.NewRecorder()
	handler.ServeHTTP(resp, req)
	if resp.Code != http.StatusForbidden {
		t.Fatalf("expected 403 when scope missing, got %d", resp.Code)
	}
	if challenge := resp.Header().Get("WWW-Authenticate"); challenge != "" {
		t.Fatalf("did not expect WWW-Authenticate on 403, got %q", challenge)
	}
}

func TestAuthMiddlewareTenantClaimPresent(t *testing.T) {
	mw := authMiddleware(Config{Dev: true})
	var gotTenant string
	var gotTenantOK bool
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotTenant, gotTenantOK = requestctx.Tenant(r.Context())
		w.WriteHeader(http.StatusOK)
	}))
	token := unsignedJWT(`{"sub":"tester","tenant":"acme","scope":"jobs:read"}`)
	req := httptest.NewRequest(http.MethodGet, "/jobs", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp := httptest.NewRecorder()
	handler.ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.Code)
	}
	if !gotTenantOK {
		t.Fatalf("expected tenant claim to be present")
	}
	if gotTenant != "acme" {
		t.Fatalf("expected tenant %q, got %q", "acme", gotTenant)
	}
}

func TestAuthMiddlewareTenantClaimAbsent(t *testing.T) {
	mw := authMiddleware(Config{Dev: true})
	var gotTenantOK bool
	var gotPrincipalOK bool
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, gotTenantOK = requestctx.Tenant(r.Context())
		_, gotPrincipalOK = requestctx.Principal(r.Context())
		w.WriteHeader(http.StatusOK)
	}))
	token := unsignedJWT(`{"sub":"tester","scope":"jobs:read"}`)
	req := httptest.NewRequest(http.MethodGet, "/jobs", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp := httptest.NewRecorder()
	handler.ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.Code)
	}
	if gotTenantOK {
		t.Fatalf("expected tenant claim to be absent")
	}
	if !gotPrincipalOK {
		t.Fatalf("expected principal to be present")
	}
}

type nopWriter struct{}

func (nopWriter) Write(p []byte) (int, error) { return len(p), nil }
func (nopWriter) Sync() error                 { return nil }

func unsignedJWT(payload string) string {
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none"}`))
	body := base64.RawURLEncoding.EncodeToString([]byte(payload))
	return header + "." + body + "."
}
