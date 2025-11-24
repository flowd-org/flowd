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
	sink.Publish("run-123", sse.Event{Event: "run.start", Data: "{}"})
	time.Sleep(10 * time.Millisecond)
	cancel()

	<-done
	body := rec.Body.String()
	if !strings.Contains(body, "event: run.start") {
		t.Fatalf("expected run.start event in body, got %q", body)
	}
	if !strings.Contains(body, "retry: 2000") {
		t.Fatalf("expected retry directive in body, got %q", body)
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

	sink.Publish("run-456", sse.Event{Event: "run.start", Data: "{}"})
	sink.Publish("run-456", sse.Event{Event: "step.log", Data: "{\"msg\":\"hello\"}"})

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

func TestRunEventsHandlerReplayWithoutLastID(t *testing.T) {
	store := runstore.New()
	store.Create(runstore.Run{ID: "run-789", JobID: "demo", Status: "queued", StartedAt: time.Unix(0, 0)})
	hub := sse.New(sse.Config{KeepAliveInterval: time.Hour})
	journal := newTestJournal(t)
	sink := NewJournalEventSink(journal, EventSinkFunc(func(runID string, ev sse.Event) {
		hub.Publish(runID, ev)
	}))

	sink.Publish("run-789", sse.Event{Event: "run.start", Data: "{}"})
	sink.Publish("run-789", sse.Event{Event: "step.log", Data: "{\"msg\":\"world\"}"})

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
	if !strings.Contains(body, "run.start") || !strings.Contains(body, "step.log") {
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

	sink.Publish("run-expired", sse.Event{Event: "step.log", Data: "{\"msg\":\"old\"}"})
	sink.Publish("run-expired", sse.Event{Event: "step.log", Data: "{\"msg\":\"new\"}"})

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
