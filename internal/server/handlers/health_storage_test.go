package handlers

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/flowd-org/flowd/internal/coredb"
)

func TestStorageHealthHandlerOK(t *testing.T) {
	handler := &storageHealthHandler{
		stats: func(r *http.Request) (coredb.StorageStats, error) {
			return coredb.StorageStats{
				Driver:          "sqlite",
				OK:              true,
				BytesUsed:       1024,
				MaxBytes:        4096,
				JournalBytes:    128,
				JournalMaxBytes: 1 << 20,
				SchemaVersion:   1,
			}, nil
		},
	}

	req := httptest.NewRequest(http.MethodGet, "/health/storage", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Fatalf("expected application/json, got %q", ct)
	}
}

func TestStorageHealthHandlerDegraded(t *testing.T) {
	handler := &storageHealthHandler{
		stats: func(r *http.Request) (coredb.StorageStats, error) {
			return coredb.StorageStats{}, errors.New("simulated failure")
		},
	}

	req := httptest.NewRequest(http.MethodGet, "/health/storage", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", rec.Code)
	}
}

func TestStorageHealthHandlerMethodNotAllowed(t *testing.T) {
	handler := NewStorageHealthHandler(nil)
	req := httptest.NewRequest(http.MethodPost, "/health/storage", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rec.Code)
	}
}
