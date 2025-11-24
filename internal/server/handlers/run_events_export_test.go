package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/flowd-org/flowd/internal/server/runstore"
	"github.com/flowd-org/flowd/internal/server/sse"
)

func TestRunEventsExportHandlerStreamsNDJSON(t *testing.T) {
	store := runstore.New()
	store.Create(runstore.Run{ID: "run-export", JobID: "demo", Status: "completed", StartedAt: time.Now()})
	journal := newTestJournal(t)
	sink := NewJournalEventSink(journal, EventSinkFunc(func(runID string, ev sse.Event) {}))

	sink.Publish("run-export", sse.Event{Event: "run.start", Data: "{}"})
	sink.Publish("run-export", sse.Event{Event: "step.log", Data: "{\"msg\":\"hello\"}"})

	handler := NewRunEventsExportHandler(store, journal, true)
	req := httptest.NewRequest(http.MethodGet, "/runs/run-export/events.ndjson", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); ct != "text/x-ndjson" {
		t.Fatalf("expected text/x-ndjson, got %s", ct)
	}
	lines := strings.Split(strings.TrimSpace(rec.Body.String()), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 NDJSON lines, got %d (%s)", len(lines), rec.Body.String())
	}
	if !strings.Contains(lines[0], "\"event\":\"run.start\"") {
		t.Fatalf("expected run.start event, got %s", lines[0])
	}
	if !strings.Contains(lines[1], "\"event\":\"step.log\"") {
		t.Fatalf("expected step.log event, got %s", lines[1])
	}
}

func TestRunEventsExportHandlerDisabled(t *testing.T) {
	handler := NewRunEventsExportHandler(runstore.New(), newTestJournal(t), false)
	req := httptest.NewRequest(http.MethodGet, "/runs/run-export/events.ndjson", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 when disabled, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "extension unsupported") {
		t.Fatalf("expected extension unsupported problem, got %s", rec.Body.String())
	}
}

func TestRunEventsExportHandlerReturns410WhenNoEvents(t *testing.T) {
	store := runstore.New()
	store.Create(runstore.Run{ID: "run-missing", JobID: "demo", Status: "completed", StartedAt: time.Now()})
	journal := newTestJournal(t)

	handler := NewRunEventsExportHandler(store, journal, true)
	req := httptest.NewRequest(http.MethodGet, "/runs/run-missing/events.ndjson", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusGone {
		t.Fatalf("expected 410 when events evicted, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "cursor expired") {
		t.Fatalf("expected cursor expired detail, got %s", rec.Body.String())
	}
}
