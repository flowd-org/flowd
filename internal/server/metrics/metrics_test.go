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
	reg.RecordSSEStreamStart("sse")
	reg.RecordSSEStreamEnd("sse")
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
	if !strings.Contains(body, `flowd_sse_stream_start_total{transport="sse"} 1`) {
		t.Fatalf("expected stream start counter, got body:\n%s", body)
	}
	if !strings.Contains(body, `flowd_sse_stream_end_total{transport="sse"} 1`) {
		t.Fatalf("expected stream end counter, got body:\n%s", body)
	}
}

func TestIdempotencyMetricsOutput(t *testing.T) {
	reg := NewRegistry()
	reg.RecordIdempotencyLookup("hit")
	reg.RecordIdempotencyLookup("miss")
	reg.RecordIdempotencyReplay()
	reg.RecordIdempotencyConflict()
	reg.RecordIdempotencyInFlight()

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rr := httptest.NewRecorder()
	reg.Handler().ServeHTTP(rr, req)

	body := rr.Body.String()
	if !strings.Contains(body, `flowd_idempotency_lookup_total{outcome="hit"} 1`) {
		t.Fatalf("expected idempotency hit lookup counter, got body:\n%s", body)
	}
	if !strings.Contains(body, `flowd_idempotency_lookup_total{outcome="miss"} 1`) {
		t.Fatalf("expected idempotency miss lookup counter, got body:\n%s", body)
	}
	if !strings.Contains(body, "flowd_idempotency_replay_total 1\n") {
		t.Fatalf("expected idempotency replay counter, got body:\n%s", body)
	}
	if !strings.Contains(body, "flowd_idempotency_conflict_total 1\n") {
		t.Fatalf("expected idempotency conflict counter, got body:\n%s", body)
	}
	if !strings.Contains(body, "flowd_idempotency_inflight_total 1\n") {
		t.Fatalf("expected idempotency in-flight counter, got body:\n%s", body)
	}
}

func TestRunAndSecretMetricsOutput(t *testing.T) {
	reg := NewRegistry()
	reg.RecordRunStarted()
	reg.RecordRunFinished("completed")
	reg.RecordSecretHandleCreated(2)
	reg.RecordSecretHandleCleanup(2, true)
	reg.RecordSecretHandleCleanup(1, false)
	reg.RecordSecretContainerRejected()

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rr := httptest.NewRecorder()
	reg.Handler().ServeHTTP(rr, req)

	body := rr.Body.String()
	if !strings.Contains(body, "flowd_runs_started_total 1\n") {
		t.Fatalf("expected run started counter, got body:\n%s", body)
	}
	if !strings.Contains(body, `flowd_runs_finished_total{status="completed"} 1`) {
		t.Fatalf("expected run finished counter, got body:\n%s", body)
	}
	if !strings.Contains(body, "flowd_secret_handles_created_total 2\n") {
		t.Fatalf("expected secret handles created counter, got body:\n%s", body)
	}
	if !strings.Contains(body, "flowd_secret_handles_cleaned_total 3\n") {
		t.Fatalf("expected secret handles cleaned counter, got body:\n%s", body)
	}
	if !strings.Contains(body, "flowd_secret_handle_errors_total 1\n") {
		t.Fatalf("expected secret handle errors counter, got body:\n%s", body)
	}
	if !strings.Contains(body, "flowd_secret_container_rejected_total 1\n") {
		t.Fatalf("expected secret container rejected counter, got body:\n%s", body)
	}
}

func TestKVAndArtifactFailureMetricsOutput(t *testing.T) {
	reg := NewRegistry()
	reg.RecordKVQuotaExceeded("core_triggers")
	reg.RecordKVQuotaExceeded("CORE_TRIGGERS")
	reg.RecordArtifactWriteFailed("size_cap")
	reg.RecordArtifactWriteFailed("io_error")

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rr := httptest.NewRecorder()
	reg.Handler().ServeHTTP(rr, req)

	body := rr.Body.String()
	if !strings.Contains(body, `kv_quota_exceeded_total{namespace="core_triggers"} 2`) {
		t.Fatalf("expected kv quota counter for namespace, got body:\n%s", body)
	}
	if !strings.Contains(body, `artifacts_write_failed_total{reason="size_cap"} 1`) {
		t.Fatalf("expected artifact size cap failure counter, got body:\n%s", body)
	}
	if !strings.Contains(body, `artifacts_write_failed_total{reason="io_error"} 1`) {
		t.Fatalf("expected artifact io error failure counter, got body:\n%s", body)
	}
}

func TestNormalizeLabel(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"  HELLO  ", "hello"},
		{"World", "world"},
		{"  test  label  ", "test  label"},
		{"", ""},
		{"   ", ""},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := normalizeLabel(tt.input)
			if got != tt.expected {
				t.Errorf("normalizeLabel(%q) = %q; want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestEscapeHelp(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"simple help text", "simple help text"},
		{"help with \\ backslash", "help with \\\\ backslash"},
		{"line1\nline2", "line1\nline2"},
		{"multiple \\ and more", "multiple \\\\ and more"},
		{"", ""},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := escapeHelp(tt.input)
			if got != tt.expected {
				t.Errorf("escapeHelp(%q) = %q; want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestSortedKeysUint(t *testing.T) {
	m := map[string]uint64{
		"zebra":  3,
		"apple":  1,
		"banana": 2,
	}
	keys := sortedKeysUint(m)
	if len(keys) != 3 {
		t.Fatalf("expected 3 keys, got %d", len(keys))
	}
	if keys[0] != "apple" || keys[1] != "banana" || keys[2] != "zebra" {
		t.Errorf("keys not sorted: %v", keys)
	}
}

func TestSortedKeysInt64(t *testing.T) {
	m := map[string]int64{
		"z": 3,
		"a": 1,
		"b": 2,
	}
	keys := sortedKeysInt64(m)
	if len(keys) != 3 {
		t.Fatalf("expected 3 keys, got %d", len(keys))
	}
	if keys[0] != "a" || keys[1] != "b" || keys[2] != "z" {
		t.Errorf("keys not sorted: %v", keys)
	}
}

func TestSortedKeysString(t *testing.T) {
	m := map[string]string{
		"z": "zebra",
		"a": "apple",
		"b": "banana",
	}
	keys := sortedKeys(m)
	if len(keys) != 3 {
		t.Fatalf("expected 3 keys, got %d", len(keys))
	}
	if keys[0] != "a" || keys[1] != "b" || keys[2] != "z" {
		t.Errorf("keys not sorted: %v", keys)
	}
}

func TestLabelsToString(t *testing.T) {
	labels := map[string]string{
		"b": "beta",
		"a": "alpha",
		"c": "gamma",
	}
	result := labelsToString(labels)
	expected := "{a=\"alpha\",b=\"beta\",c=\"gamma\"}"
	if result != expected {
		t.Errorf("labelsToString = %q; want %q", result, expected)
	}
}

func TestLabelsToStringEmpty(t *testing.T) {
	result := labelsToString(map[string]string{})
	if result != "" {
		t.Errorf("labelsToString(empty) = %q; want empty string", result)
	}
}

func TestSetBuildInfo(t *testing.T) {
	reg := NewRegistry()
	reg.SetBuildInfo(map[string]string{
		"version": "test-version",
		"commit":  "abc123",
	})
	if reg.buildInfoLabels["version"] != "test-version" || reg.buildInfoLabels["commit"] != "abc123" {
		t.Errorf("build info not set correctly")
	}
}

func TestRecordHTTP(t *testing.T) {
	reg := NewRegistry()
	reg.RecordHTTP("/api/test", http.MethodGet, 200, 5*time.Millisecond)
	reg.RecordHTTP("/api/test", http.MethodPost, 404, 10*time.Millisecond)

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rr := httptest.NewRecorder()
	reg.Handler().ServeHTTP(rr, req)

	body := rr.Body.String()
	if !strings.Contains(body, `method="GET",route="/api/test",code="200"`) {
		t.Fatalf("expected HTTP metrics for GET /api/test 200, got body:\n%s", body)
	}
}

func TestRecordSecurityProfileGauge(t *testing.T) {
	reg := NewRegistry()
	reg.RecordSecurityProfileGauge("strict")

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rr := httptest.NewRecorder()
	reg.Handler().ServeHTTP(rr, req)

	body := rr.Body.String()
	if !strings.Contains(body, `flwd_security_profile{profile="strict"} 1`) {
		t.Errorf("expected security profile gauge, got body:\n%s", body)
	}
}

func TestRecordPolicyDenial(t *testing.T) {
	reg := NewRegistry()
	reg.RecordPolicyDenial("unauthorized")
	reg.RecordPolicyDenial("forbidden")

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rr := httptest.NewRecorder()
	reg.Handler().ServeHTTP(rr, req)

	body := rr.Body.String()
	if !strings.Contains(body, `flwd_policy_denials_total{reason="unauthorized"} 1`) {
		t.Errorf("expected policy denial counter, got body:\n%s", body)
	}
}

func TestRecordContainerRun(t *testing.T) {
	reg := NewRegistry()
	reg.RecordContainerRun(250 * time.Millisecond)

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rr := httptest.NewRecorder()
	reg.Handler().ServeHTTP(rr, req)

	body := rr.Body.String()
	if !strings.Contains(body, "flwd_container_runs_total 1") {
		t.Errorf("expected container run counter, got body:\n%s", body)
	}
}

func TestRecordContainerPull(t *testing.T) {
	reg := NewRegistry()
	reg.RecordContainerPull(30 * time.Second)

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rr := httptest.NewRecorder()
	reg.Handler().ServeHTTP(rr, req)

	body := rr.Body.String()
	if !strings.Contains(body, "flwd_container_pulls_total 1") {
		t.Errorf("expected container pull counter, got body:\n%s", body)
	}
}

func TestRecordSourceAdded(t *testing.T) {
	reg := NewRegistry()
	reg.RecordSourceAdded("github")
	reg.RecordSourceAdded("GITHUB")

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rr := httptest.NewRecorder()
	reg.Handler().ServeHTTP(rr, req)

	body := rr.Body.String()
	if !strings.Contains(body, `flwd_sources_added_total{type="github"} 2`) {
		t.Errorf("expected sources added counter, got body:\n%s", body)
	}
}

func TestRecordAddonManifestInvalid(t *testing.T) {
	reg := NewRegistry()
	reg.RecordAddonManifestInvalid()

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rr := httptest.NewRecorder()
	reg.Handler().ServeHTTP(rr, req)

	body := rr.Body.String()
	if !strings.Contains(body, "flwd_addon_manifest_invalid_total 1") {
		t.Errorf("expected addon manifest invalid counter, got body:\n%s", body)
	}
}

func TestSourceAddedTotals(t *testing.T) {
	reg := NewRegistry()
	reg.RecordSourceAdded("github")
	reg.RecordSourceAdded("gitlab")

	totals := reg.SourceAddedTotals()
	if totals["github"] != 1 || totals["gitlab"] != 1 {
		t.Errorf("expected source totals, got %v", totals)
	}
}

func TestAddonManifestInvalidTotal(t *testing.T) {
	reg := NewRegistry()
	reg.RecordAddonManifestInvalid()

	total := reg.AddonManifestInvalidTotal()
	if total != 1 {
		t.Errorf("expected invalid manifest total 1, got %d", total)
	}
}

func TestContainerPullsTotal(t *testing.T) {
	reg := NewRegistry()
	reg.RecordContainerPull(10 * time.Second)

	total := reg.ContainerPullsTotal()
	if total != 1 {
		t.Errorf("expected container pulls total 1, got %d", total)
	}
}

func TestHandlerReturnsMetricsText(t *testing.T) {
	reg := NewRegistry()
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rr := httptest.NewRecorder()
	reg.Handler().ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rr.Code)
	}
	if !strings.Contains(rr.Header().Get("Content-Type"), "text/plain") {
		t.Errorf("expected text/plain content type, got %s", rr.Header().Get("Content-Type"))
	}
}
