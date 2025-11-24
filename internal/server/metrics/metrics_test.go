// SPDX-License-Identifier: AGPL-3.0-or-later
package metrics

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestPersistenceMetricsOutput(t *testing.T) {
	reg := NewRegistry()
	reg.RecordPersistenceLatency("idempotency_lookup", "hit", 5*time.Millisecond)
	reg.RecordPersistenceLatency("journal_append", "quota_exceeded", 12*time.Millisecond)
	reg.RecordPersistenceEviction("journal", 512)
	reg.RecordPersistenceEviction("idempotency", 128)

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rr := httptest.NewRecorder()
	reg.Handler().ServeHTTP(rr, req)

	if ct := rr.Header().Get("Content-Type"); !strings.Contains(ct, "text/plain") {
		t.Fatalf("expected text/plain content type, got %q", ct)
	}

	body := rr.Body.String()
	if !strings.Contains(body, `le="5",operation="idempotency_lookup",outcome="hit"} 1`) {
		t.Fatalf("expected latency bucket for idempotency lookup hit, got body:\n%s", body)
	}
	if !strings.Contains(body, `le="25",operation="journal_append",outcome="quota_exceeded"} 1`) {
		t.Fatalf("expected latency bucket for journal append quota exceeded, got body:\n%s", body)
	}
	if !strings.Contains(body, `flowd_persistence_evictions_total{kind="journal"} 1`) {
		t.Fatalf("expected journal eviction counter, got body:\n%s", body)
	}
	if !strings.Contains(body, `flowd_persistence_evictions_total{kind="idempotency"} 1`) {
		t.Fatalf("expected idempotency eviction counter, got body:\n%s", body)
	}
	if !strings.Contains(body, `flowd_persistence_eviction_bytes_total{kind="idempotency"} 128`) {
		t.Fatalf("expected idempotency eviction bytes counter, got body:\n%s", body)
	}
}

func TestSSEMetricsOutput(t *testing.T) {
	reg := NewRegistry()
	reg.RecordSSEActiveDelta("sse", 2)
	reg.RecordSSEActiveDelta("sse", -1)
	reg.RecordSSEActiveDelta("websocket", 3)
	reg.RecordSSEResumeAttempt()
	reg.RecordSSEResumeAttempt()
	reg.RecordSSECursorExpired()

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rr := httptest.NewRecorder()
	reg.Handler().ServeHTTP(rr, req)

	body := rr.Body.String()
	if !strings.Contains(body, `flowd_sse_active_streams{transport="sse"} 1`) {
		t.Fatalf("expected active SSE count for sse transport, got body:\n%s", body)
	}
	if !strings.Contains(body, `flowd_sse_active_streams{transport="websocket"} 3`) {
		t.Fatalf("expected active SSE count for websocket transport, got body:\n%s", body)
	}
	if !strings.Contains(body, `flowd_sse_resume_total 2`) {
		t.Fatalf("expected resume total counter, got body:\n%s", body)
	}
	if !strings.Contains(body, `flowd_sse_cursor_expired_total 1`) {
		t.Fatalf("expected cursor expired counter, got body:\n%s", body)
	}
}
