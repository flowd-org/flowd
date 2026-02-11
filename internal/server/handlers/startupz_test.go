package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestStartupzHandlerMethodNotAllowed(t *testing.T) {
	handler := NewStartupzHandlerWithState(nil)
	req := httptest.NewRequest(http.MethodPost, "/startupz", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rec.Code)
	}
}

func TestStartupzHandlerIncomplete(t *testing.T) {
	handler := NewStartupzHandlerWithState(func(*http.Request) bool { return false })
	req := httptest.NewRequest(http.MethodGet, "/startupz", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/problem+json" {
		t.Fatalf("expected application/problem+json, got %q", ct)
	}

	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode problem: %v", err)
	}
	if body["type"] != startupIncompleteProblemType {
		t.Fatalf("expected problem type %q, got %v", startupIncompleteProblemType, body["type"])
	}
}

func TestStartupzHandlerComplete(t *testing.T) {
	handler := NewStartupzHandlerWithState(func(*http.Request) bool { return true })
	req := httptest.NewRequest(http.MethodGet, "/startupz", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", rec.Code)
	}
	if cc := rec.Header().Get("Cache-Control"); cc != "no-store" {
		t.Fatalf("expected no-store cache header, got %q", cc)
	}
}
