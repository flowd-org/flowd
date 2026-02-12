package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"runtime"
	"testing"
	"time"
)

func TestLimitsHandlerMethodNotAllowed(t *testing.T) {
	handler := NewLimitsHandler()
	req := httptest.NewRequest(http.MethodPost, "/limits", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rec.Code)
	}
}

func TestLimitsHandlerResponseDefaultsAndShape(t *testing.T) {
	handler := NewLimitsHandler()
	req := httptest.NewRequest(http.MethodGet, "/limits", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Fatalf("expected application/json, got %q", ct)
	}
	if cc := rec.Header().Get("Cache-Control"); cc != "no-store" {
		t.Fatalf("expected no-store cache header, got %q", cc)
	}

	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body: %v", err)
	}

	if got := body["algorithm"]; got != limitsAlgorithmDefault {
		t.Fatalf("expected algorithm %q, got %v", limitsAlgorithmDefault, got)
	}
	expectedConcurrency := defaultConcurrency(runtime.GOMAXPROCS(0))
	if got := int(body["concurrency"].(float64)); got != expectedConcurrency {
		t.Fatalf("expected concurrency %d for current GOMAXPROCS, got %d", expectedConcurrency, got)
	}
	if got := int(body["queue_max_depth"].(float64)); got != limitsQueueMaxDepthDefault {
		t.Fatalf("expected queue_max_depth %d, got %d", limitsQueueMaxDepthDefault, got)
	}
	if got := body["backpressure_mode"]; got != limitsBackpressureModeDefault {
		t.Fatalf("expected backpressure_mode %q, got %v", limitsBackpressureModeDefault, got)
	}

	queueStats, ok := body["queue_stats"].(map[string]any)
	if !ok {
		t.Fatalf("expected queue_stats object, got %T", body["queue_stats"])
	}
	if got := int(queueStats["len"].(float64)); got != 0 {
		t.Fatalf("expected queue_stats.len=0, got %d", got)
	}
	if got := int(queueStats["enqueued"].(float64)); got != 0 {
		t.Fatalf("expected queue_stats.enqueued=0, got %d", got)
	}
	if got := int(queueStats["dequeued"].(float64)); got != 0 {
		t.Fatalf("expected queue_stats.dequeued=0, got %d", got)
	}
	if got := int(queueStats["dropped"].(float64)); got != 0 {
		t.Fatalf("expected queue_stats.dropped=0, got %d", got)
	}

	updatedAt, ok := body["updated_at"].(string)
	if !ok {
		t.Fatalf("expected updated_at string, got %T", body["updated_at"])
	}
	if _, err := time.Parse(time.RFC3339, updatedAt); err != nil {
		t.Fatalf("expected RFC3339 updated_at, got %q (%v)", updatedAt, err)
	}

	if _, exists := body["per_tenant_weights"]; exists {
		t.Fatal("expected per_tenant_weights to be omitted when not implemented")
	}
	if _, exists := body["per_tenant_concurrency"]; exists {
		t.Fatal("expected per_tenant_concurrency to be omitted when not implemented")
	}
	if _, exists := body["drain_timeout_ms"]; exists {
		t.Fatal("expected drain_timeout_ms to be omitted when not implemented")
	}
}

func TestDefaultConcurrencyUsesTwiceGOMAXPROCSWithMinFour(t *testing.T) {
	if got := defaultConcurrency(1); got != 4 {
		t.Fatalf("expected min concurrency 4, got %d", got)
	}
	if got := defaultConcurrency(3); got != 6 {
		t.Fatalf("expected concurrency 6 for gomaxprocs=3, got %d", got)
	}
}
