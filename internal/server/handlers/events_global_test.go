package handlers

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/flowd-org/flowd/internal/server/runstore"
	"github.com/flowd-org/flowd/internal/server/sse"
)

func TestEventsHandlerGlobalStream(t *testing.T) {
	store := runstore.New()
	store.Create(runstore.Run{ID: "run-1", JobID: "demo", Status: "queued", StartedAt: time.Unix(0, 0)})
	runHub := sse.New(sse.Config{KeepAliveInterval: time.Hour})
	globalHub := sse.New(sse.Config{KeepAliveInterval: time.Hour})

	handler := NewEventsHandler(EventsConfig{RunStore: store, RunHub: runHub, GlobalHub: globalHub})

	ctx, cancel := context.WithCancel(context.Background())
	req := httptest.NewRequest(http.MethodGet, "/events/stream", nil).WithContext(ctx)
	rec := httptest.NewRecorder()

	done := make(chan struct{})
	go func() {
		handler.ServeHTTP(rec, req)
		close(done)
	}()

	time.Sleep(10 * time.Millisecond)
	globalHub.Publish("global", WrapGlobalEvent("run-1", sse.Event{ID: "1", Event: "run.started", Data: "{}"}))
	time.Sleep(10 * time.Millisecond)
	cancel()
	<-done

	body := rec.Body.Bytes()
	if len(body) == 0 {
		t.Fatalf("expected SSE payload")
	}
	if !bytes.Contains(body, []byte("\"type\":\"run.started\"")) {
		t.Fatalf("expected run.started in stream, got %q", body)
	}
	if !bytes.Contains(body, []byte("run-1")) {
		t.Fatalf("expected run_id in stream")
	}
}

func TestEventsHandlerRunScopedQuery(t *testing.T) {
	store := runstore.New()
	store.Create(runstore.Run{ID: "run-2", JobID: "demo", Status: "queued", StartedAt: time.Unix(0, 0)})
	runHub := sse.New(sse.Config{KeepAliveInterval: time.Hour})
	globalHub := sse.New(sse.Config{KeepAliveInterval: time.Hour})
	handler := NewEventsHandler(EventsConfig{RunStore: store, RunHub: runHub, GlobalHub: globalHub})

	ctx, cancel := context.WithCancel(context.Background())
	req := httptest.NewRequest(http.MethodGet, "/events/stream?run_id=run-2", nil).WithContext(ctx)
	rec := httptest.NewRecorder()
	done := make(chan struct{})
	go func() {
		handler.ServeHTTP(rec, req)
		close(done)
	}()

	time.Sleep(10 * time.Millisecond)
	runHub.Publish("run-2", sse.Event{ID: "1", Event: "run.started", Data: "{}"})
	time.Sleep(10 * time.Millisecond)
	cancel()
	<-done

	if !bytes.Contains(rec.Body.Bytes(), []byte("\"type\":\"run.started\"")) {
		t.Fatalf("expected run.started event for run_id filter")
	}
}

func TestEventsHandlerGlobalStreamResume(t *testing.T) {
	tests := []struct {
		name string
		path string
	}{
		{name: "stream", path: "/events/stream"},
		{name: "alias", path: "/events"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := runstore.New()
			runHub := sse.New(sse.Config{KeepAliveInterval: time.Hour})
			globalHub := sse.New(sse.Config{KeepAliveInterval: time.Hour})
			journal := newTestJournal(t)
			sink := NewJournalEventSink(journal, EventSinkFunc(func(runID string, ev sse.Event) {
				globalHub.Publish("global", WrapGlobalEvent(runID, ev))
			}))

			sink.Publish("run-1", sse.Event{Event: "run.started", Data: "{\"msg\":\"one\"}"})
			sink.Publish("run-2", sse.Event{Event: "step.output", Data: "{\"msg\":\"two\"}"})

			handler := NewEventsHandler(EventsConfig{
				RunStore:  store,
				RunHub:    runHub,
				GlobalHub: globalHub,
				Journal:   journal,
			})

			ctx, cancel := context.WithCancel(context.Background())
			req := httptest.NewRequest(http.MethodGet, tt.path, nil).WithContext(ctx)
			req.Header.Set("Last-Event-ID", "1")
			rec := httptest.NewRecorder()

			done := make(chan struct{})
			go func() {
				handler.ServeHTTP(rec, req)
				close(done)
			}()

			time.Sleep(10 * time.Millisecond)
			cancel()
			<-done

			body := rec.Body.String()
			if !strings.Contains(body, "retry: 3000") {
				t.Fatalf("expected retry directive, got %q", body)
			}
			if !strings.Contains(body, "event: flowd") {
				t.Fatalf("expected flowd event envelope, got %q", body)
			}
			if strings.Count(body, "id: 2") != 1 {
				t.Fatalf("expected single replay of event id 2, got %q", body)
			}
			if !strings.Contains(body, "\"type\":\"step.output\"") {
				t.Fatalf("expected step.output event, got %q", body)
			}
			if !strings.Contains(body, "\"run_id\":\"run-2\"") {
				t.Fatalf("expected run_id in stream, got %q", body)
			}
		})
	}
}

func TestEventsHandlerGlobalStreamStaleCursor(t *testing.T) {
	tests := []struct {
		name string
		path string
	}{
		{name: "stream", path: "/events/stream"},
		{name: "alias", path: "/events"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := runstore.New()
			runHub := sse.New(sse.Config{KeepAliveInterval: time.Hour})
			globalHub := sse.New(sse.Config{KeepAliveInterval: time.Hour})
			journal := newTestJournalWithLimit(t, 20)
			sink := NewJournalEventSink(journal, EventSinkFunc(func(runID string, ev sse.Event) {
				globalHub.Publish("global", WrapGlobalEvent(runID, ev))
			}))

			sink.Publish("run-old", sse.Event{Event: "step.output", Data: "{\"msg\":\"old\"}"})
			sink.Publish("run-new", sse.Event{Event: "step.output", Data: "{\"msg\":\"new\"}"})

			handler := NewEventsHandler(EventsConfig{
				RunStore:  store,
				RunHub:    runHub,
				GlobalHub: globalHub,
				Journal:   journal,
			})

			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			req.Header.Set("Last-Event-ID", "1")
			rec := httptest.NewRecorder()

			handler.ServeHTTP(rec, req)
			if rec.Code != http.StatusGone {
				t.Fatalf("expected 410 Gone, got %d", rec.Code)
			}
			if ct := rec.Header().Get("Content-Type"); ct != "application/problem+json" {
				t.Fatalf("expected problem+json content type, got %q", ct)
			}
			if !strings.Contains(rec.Body.String(), "https://flowd.org/problems/sse/stale-cursor") {
				t.Fatalf("expected stale-cursor problem type, got %q", rec.Body.String())
			}
		})
	}
}

func TestEventsHandlerGlobalStreamInvalidLastEventID(t *testing.T) {
	tests := []struct {
		name string
		path string
	}{
		{name: "stream", path: "/events/stream"},
		{name: "alias", path: "/events"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Run("non-integer", func(t *testing.T) {
				store := runstore.New()
				runHub := sse.New(sse.Config{KeepAliveInterval: time.Hour})
				globalHub := sse.New(sse.Config{KeepAliveInterval: time.Hour})
				handler := NewEventsHandler(EventsConfig{RunStore: store, RunHub: runHub, GlobalHub: globalHub})

				req := httptest.NewRequest(http.MethodGet, tt.path, nil)
				req.Header.Set("Last-Event-ID", "not-a-number")
				rec := httptest.NewRecorder()

				handler.ServeHTTP(rec, req)
				if rec.Code != http.StatusBadRequest {
					t.Fatalf("expected 400 Bad Request, got %d", rec.Code)
				}
			})

			t.Run("beyond-latest", func(t *testing.T) {
				store := runstore.New()
				runHub := sse.New(sse.Config{KeepAliveInterval: time.Hour})
				globalHub := sse.New(sse.Config{KeepAliveInterval: time.Hour})
				journal := newTestJournal(t)
				sink := NewJournalEventSink(journal, EventSinkFunc(func(runID string, ev sse.Event) {
					globalHub.Publish("global", WrapGlobalEvent(runID, ev))
				}))

				sink.Publish("run-1", sse.Event{Event: "run.started", Data: "{}"})

				handler := NewEventsHandler(EventsConfig{
					RunStore:  store,
					RunHub:    runHub,
					GlobalHub: globalHub,
					Journal:   journal,
				})

				req := httptest.NewRequest(http.MethodGet, tt.path, nil)
				req.Header.Set("Last-Event-ID", "2")
				rec := httptest.NewRecorder()

				handler.ServeHTTP(rec, req)
				if rec.Code != http.StatusBadRequest {
					t.Fatalf("expected 400 Bad Request, got %d", rec.Code)
				}
			})
		})
	}
}
