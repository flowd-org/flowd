package handlers

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/flowd-org/flowd/internal/coredb"
)

func TestReadyzHandlerMethodNotAllowed(t *testing.T) {
	handler := NewReadyzHandlerWithChecks(nil, nil)
	req := httptest.NewRequest(http.MethodPost, "/readyz", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rec.Code)
	}
}

func TestReadyzHandlerCoreDBNotReady(t *testing.T) {
	handler := NewReadyzHandlerWithChecks(
		func(*http.Request) error { return errors.New("boom") },
		func(*http.Request) (coredb.StorageStats, error) {
			return coredb.StorageStats{OK: true}, nil
		},
	)
	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
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
	if body["type"] != readyzNotReadyProblemType {
		t.Fatalf("expected problem type %q, got %v", readyzNotReadyProblemType, body["type"])
	}
}

func TestReadyzHandlerStorageError(t *testing.T) {
	handler := NewReadyzHandlerWithChecks(
		func(*http.Request) error { return nil },
		func(*http.Request) (coredb.StorageStats, error) {
			return coredb.StorageStats{}, errors.New("storage error")
		},
	)
	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", rec.Code)
	}
}

func TestReadyzHandlerStorageNotHealthy(t *testing.T) {
	handler := NewReadyzHandlerWithChecks(
		func(*http.Request) error { return nil },
		func(*http.Request) (coredb.StorageStats, error) {
			return coredb.StorageStats{OK: false}, nil
		},
	)
	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", rec.Code)
	}
}

func TestReadyzHandlerOK(t *testing.T) {
	handler := NewReadyzHandlerWithChecks(
		func(*http.Request) error { return nil },
		func(*http.Request) (coredb.StorageStats, error) {
			return coredb.StorageStats{OK: true}, nil
		},
	)
	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", rec.Code)
	}
	if cc := rec.Header().Get("Cache-Control"); cc != "no-store" {
		t.Fatalf("expected no-store cache header, got %q", cc)
	}
}
