package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/flowd-org/flowd/internal/coredb"
	"github.com/flowd-org/flowd/internal/server/runstore"
	"github.com/flowd-org/flowd/internal/server/sse"
)

func TestRunEventsHandlerStreamsEvents(t *testing.T) {
	store := runstore.New()
	store.Create(runstore.Run{ID: "run-123", JobID: "demo", Status: "queued", StartedAt: time.Unix(0, 0)})
	hub := sse.New(sse.Config{KeepAliveInterval: time.Hour})
	journal := newTestJournal(t)
	sink := NewJournalEventSink(journal, EventSinkFunc(func(runID string, ev sse.Event) {
		hub.Publish(runID, ev)
	}))

	h := NewRunEventsHandler(store, hub, journal)

	ctx, cancel := context.WithCancel(context.Background())
	req := httptest.NewRequest(http.MethodGet, "/runs/run-123/events", nil).WithContext(ctx)
	rec := httptest.NewRecorder()

	done := make(chan struct{})
	go func() {
		h.ServeHTTP(rec, req)
		close(done)
	}()

	time.Sleep(10 * time.Millisecond)
	sink.Publish("run-123", sse.Event{Event: "run.started", Data: "{}"})
	time.Sleep(10 * time.Millisecond)
	cancel()

	<-done
	body := rec.Body.String()
	if !strings.Contains(body, "\"type\":\"run.started\"") {
		t.Fatalf("expected run.started event in body, got %q", body)
	}
	if !strings.Contains(body, "retry: 3000") {
		t.Fatalf("expected retry directive in body, got %q", body)
	}
	if !strings.Contains(body, "event: flowd") {
		t.Fatalf("expected flowd event envelope, got %q", body)
	}
}

func TestRunEventsHandlerReplayFromHeader(t *testing.T) {
	store := runstore.New()
	store.Create(runstore.Run{ID: "run-456", JobID: "demo", Status: "queued", StartedAt: time.Unix(0, 0)})
	hub := sse.New(sse.Config{KeepAliveInterval: time.Hour})
	journal := newTestJournal(t)
	sink := NewJournalEventSink(journal, EventSinkFunc(func(runID string, ev sse.Event) {
		hub.Publish(runID, ev)
	}))

	sink.Publish("run-456", sse.Event{Event: "run.started", Data: "{}"})
	sink.Publish("run-456", sse.Event{Event: "step.output", Data: "{\"msg\":\"hello\"}"})

	h := NewRunEventsHandler(store, hub, journal)
	req := httptest.NewRequest(http.MethodGet, "/runs/run-456/events", nil)
	req.Header.Set("Last-Event-ID", "1")
	rec := httptest.NewRecorder()

	ctx, cancel := context.WithCancel(context.Background())
	req = req.WithContext(ctx)
	done := make(chan struct{})
	go func() {
		h.ServeHTTP(rec, req)
		close(done)
	}()

	time.Sleep(10 * time.Millisecond)
	cancel()
	<-done

	body := rec.Body.String()
	if !strings.Contains(body, "id: 2") {
		t.Fatalf("expected replay of event id 2, got %q", body)
	}
	if strings.Count(body, "id: 2") != 1 {
		t.Fatalf("expected single replay of event id 2, got %q", body)
	}
}

func TestRunEventsHandlerResumeFromLastEventID(t *testing.T) {
	store := runstore.New()
	store.Create(runstore.Run{ID: "run-resume", JobID: "demo", Status: "queued", StartedAt: time.Unix(0, 0)})
	to := time.Hour
	hub := sse.New(sse.Config{KeepAliveInterval: to})
	journal := newTestJournal(t)
	sink := NewJournalEventSink(journal, EventSinkFunc(func(runID string, ev sse.Event) {
		hub.Publish(runID, ev)
	}))

	sink.Publish("run-resume", sse.Event{Event: "run.started", Data: "{}"})
	sink.Publish("run-resume", sse.Event{Event: "step.output", Data: "{\"msg\":\"hello\"}"})
	sink.Publish("run-resume", sse.Event{Event: "step.finished", Data: "{\"status\":\"ok\"}"})

	h := NewRunEventsHandler(store, hub, journal)
	req := httptest.NewRequest(http.MethodGet, "/runs/run-resume/events", nil)
	req.Header.Set("Last-Event-ID", "1")
	rec := httptest.NewRecorder()

	ctx, cancel := context.WithCancel(context.Background())
	req = req.WithContext(ctx)
	done := make(chan struct{})
	go func() {
		h.ServeHTTP(rec, req)
		close(done)
	}()

	time.Sleep(10 * time.Millisecond)
	cancel()
	<-done

	body := rec.Body.String()
	if strings.Contains(body, "id: 1") {
		t.Fatalf("expected resume to skip id 1, got %q", body)
	}
	if !strings.Contains(body, "id: 2") || !strings.Contains(body, "id: 3") {
		t.Fatalf("expected replay of ids 2 and 3, got %q", body)
	}
	if strings.Count(body, "id: 2") != 1 || strings.Count(body, "id: 3") != 1 {
		t.Fatalf("expected single replay of ids 2 and 3, got %q", body)
	}
}

func TestRunEventsHandlerReplayWithoutLastID(t *testing.T) {
	store := runstore.New()
	store.Create(runstore.Run{ID: "run-789", JobID: "demo", Status: "queued", StartedAt: time.Unix(0, 0)})
	hub := sse.New(sse.Config{KeepAliveInterval: time.Hour})
	journal := newTestJournal(t)
	sink := NewJournalEventSink(journal, EventSinkFunc(func(runID string, ev sse.Event) {
		hub.Publish(runID, ev)
	}))

	sink.Publish("run-789", sse.Event{Event: "run.started", Data: "{}"})
	sink.Publish("run-789", sse.Event{Event: "step.output", Data: "{\"msg\":\"world\"}"})

	h := NewRunEventsHandler(store, hub, journal)
	req := httptest.NewRequest(http.MethodGet, "/runs/run-789/events", nil)
	rec := httptest.NewRecorder()

	ctx, cancel := context.WithCancel(context.Background())
	req = req.WithContext(ctx)
	done := make(chan struct{})
	go func() {
		h.ServeHTTP(rec, req)
		close(done)
	}()

	time.Sleep(10 * time.Millisecond)
	cancel()
	<-done

	body := rec.Body.String()
	if !strings.Contains(body, "\"type\":\"run.started\"") || !strings.Contains(body, "\"type\":\"step.output\"") {
		t.Fatalf("expected replayed events, got %q", body)
	}
}

func TestRunEventsHandlerAllowsSubscribeBeforeRun(t *testing.T) {
	store := runstore.New()
	hub := sse.New(sse.Config{KeepAliveInterval: time.Millisecond * 10})
	journal := newTestJournal(t)
	h := NewRunEventsHandler(store, hub, journal)
	req := httptest.NewRequest(http.MethodGet, "/runs/pending-run/events", nil)
	rec := httptest.NewRecorder()

	ctx, cancel := context.WithCancel(context.Background())
	req = req.WithContext(ctx)
	done := make(chan struct{})
	go func() {
		h.ServeHTTP(rec, req)
		close(done)
	}()

	time.Sleep(30 * time.Millisecond)
	cancel()
	<-done

	body := rec.Body.String()
	if body == "" {
		t.Fatalf("expected initial SSE output for pre-subscribe, got empty body")
	}
}

func TestRunEventsHandlerReturns410ForExpiredCursor(t *testing.T) {
	store := runstore.New()
	hub := sse.New(sse.Config{})
	dirJournal := newTestJournalWithLimit(t, 20)
	sink := NewJournalEventSink(dirJournal, EventSinkFunc(func(runID string, ev sse.Event) {
		hub.Publish(runID, ev)
	}))

	sink.Publish("run-expired", sse.Event{Event: "step.output", Data: "{\"msg\":\"old\"}"})
	sink.Publish("run-expired", sse.Event{Event: "step.output", Data: "{\"msg\":\"new\"}"})

	h := NewRunEventsHandler(store, hub, dirJournal)
	req := httptest.NewRequest(http.MethodGet, "/runs/run-expired/events", nil)
	req.Header.Set("Last-Event-ID", "1")
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusGone {
		t.Fatalf("expected 410 Gone, got %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/problem+json" {
		t.Fatalf("expected problem+json content type, got %q", ct)
	}
	if !strings.Contains(rec.Body.String(), "cursor expired") {
		t.Fatalf("expected cursor expired problem, got %q", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "https://flowd.org/problems/sse/stale-cursor") {
		t.Fatalf("expected stale-cursor problem type, got %q", rec.Body.String())
	}
}

func TestRunEventsHandlerInvalidLastEventID(t *testing.T) {
	store := runstore.New()
	store.Create(runstore.Run{ID: "run-invalid", JobID: "demo", Status: "queued", StartedAt: time.Unix(0, 0)})

	t.Run("non-integer", func(t *testing.T) {
		hub := sse.New(sse.Config{KeepAliveInterval: time.Hour})
		journal := newTestJournal(t)
		h := NewRunEventsHandler(store, hub, journal)

		req := httptest.NewRequest(http.MethodGet, "/runs/run-invalid/events", nil)
		req.Header.Set("Last-Event-ID", "not-a-number")
		rec := httptest.NewRecorder()

		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 Bad Request, got %d", rec.Code)
		}
	})

	t.Run("beyond-latest", func(t *testing.T) {
		hub := sse.New(sse.Config{KeepAliveInterval: time.Hour})
		journal := newTestJournal(t)
		sink := NewJournalEventSink(journal, EventSinkFunc(func(runID string, ev sse.Event) {
			hub.Publish(runID, ev)
		}))

		sink.Publish("run-invalid", sse.Event{Event: "run.started", Data: "{}"})

		h := NewRunEventsHandler(store, hub, journal)
		req := httptest.NewRequest(http.MethodGet, "/runs/run-invalid/events", nil)
		req.Header.Set("Last-Event-ID", "2")
		rec := httptest.NewRecorder()

		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400 Bad Request, got %d", rec.Code)
		}
	})
}

func TestRunEventsHandlerStaleCursorAfterEviction(t *testing.T) {
	store := runstore.New()
	store.Create(runstore.Run{ID: "run-evict", JobID: "demo", Status: "queued", StartedAt: time.Unix(0, 0)})
	// Keepalive disabled to avoid background heartbeats in the response body.
	hub := sse.New(sse.Config{KeepAliveInterval: time.Hour})
	journal := newTestJournalWithLimit(t, 20)
	// Only persist to journal; live hub is unused for this stale-cursor path.
	sink := NewJournalEventSink(journal, EventSinkFunc(func(runID string, ev sse.Event) {}))

	sink.Publish("run-evict", sse.Event{Event: "step.output", Data: "{\"msg\":\"old\"}"})
	sink.Publish("run-evict", sse.Event{Event: "step.output", Data: "{\"msg\":\"new\"}"})

	h := NewRunEventsHandler(store, hub, journal)
	req := httptest.NewRequest(http.MethodGet, "/runs/run-evict/events", nil)
	req.Header.Set("Last-Event-ID", "1")
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusGone {
		t.Fatalf("expected 410 Gone, got %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/problem+json" {
		t.Fatalf("expected problem+json content type, got %q", ct)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "https://flowd.org/problems/sse/stale-cursor") {
		t.Fatalf("expected stale-cursor problem type, got %q", body)
	}
}

func newTestJournal(t *testing.T) *coredb.Journal {
	t.Helper()
	return newTestJournalWithLimit(t, 0)
}

func newTestJournalWithLimit(t *testing.T, limit int64) *coredb.Journal {
	t.Helper()
	db, err := coredb.Open(context.Background(), coredb.Options{DataDir: t.TempDir()})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() {
		_ = db.Close()
	})
	return coredb.NewJournal(db, limit)
}
