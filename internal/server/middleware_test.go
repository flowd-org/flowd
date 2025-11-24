package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
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

type nopWriter struct{}

func (nopWriter) Write(p []byte) (int, error) { return len(p), nil }
func (nopWriter) Sync() error                 { return nil }
