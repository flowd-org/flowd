package handlers

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
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
	req := httptest.NewRequest(http.MethodGet, "/events", nil).WithContext(ctx)
	rec := httptest.NewRecorder()

	done := make(chan struct{})
	go func() {
		handler.ServeHTTP(rec, req)
		close(done)
	}()

	time.Sleep(10 * time.Millisecond)
	globalHub.Publish("global", WrapGlobalEvent("run-1", sse.Event{ID: "1", Event: "run.start", Data: "{}"}))
	time.Sleep(10 * time.Millisecond)
	cancel()
	<-done

	body := rec.Body.Bytes()
	if len(body) == 0 {
		t.Fatalf("expected SSE payload")
	}
	if !bytes.Contains(body, []byte("run.start")) {
		t.Fatalf("expected run.start in stream, got %q", body)
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
	req := httptest.NewRequest(http.MethodGet, "/events?run_id=run-2", nil).WithContext(ctx)
	rec := httptest.NewRecorder()
	done := make(chan struct{})
	go func() {
		handler.ServeHTTP(rec, req)
		close(done)
	}()

	time.Sleep(10 * time.Millisecond)
	runHub.Publish("run-2", sse.Event{ID: "1", Event: "run.start", Data: "{}"})
	time.Sleep(10 * time.Millisecond)
	cancel()
	<-done

	if !bytes.Contains(rec.Body.Bytes(), []byte("run.start")) {
		t.Fatalf("expected run.start event for run_id filter")
	}
}
