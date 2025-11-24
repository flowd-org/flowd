package sse

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestHubPublishSubscribeReplay(t *testing.T) {
	h := New(Config{
		KeepAliveInterval: 0,
		MaxBufferSize:     10,
		Retention:         time.Minute,
	})
	fake := time.Unix(0, 0)
	h.nowFn = func() time.Time { return fake }

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sub := h.Subscribe(ctx, "run-1", "")
	defer sub.Close()

	h.Publish("run-1", Event{Event: "run.start", Data: `{"status":"queued"}`})

	select {
	case payload := <-sub.C:
		if got := string(payload); got == "" || !strings.HasPrefix(got, "id: 1\n") {
			t.Fatalf("expected payload with id 1, got %q", got)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for event")
	}
}

func TestHubReplayFromLastEventID(t *testing.T) {
	h := New(Config{KeepAliveInterval: 0})
	h.nowFn = func() time.Time { return time.Unix(0, 0) }

	h.Publish("run-2", Event{ID: "1", Event: "run.start", Data: "{}"})
	h.Publish("run-2", Event{ID: "2", Event: "step.log", Data: `{"msg":"hello"}`})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sub := h.Subscribe(ctx, "run-2", "1")
	defer sub.Close()

	select {
	case payload := <-sub.C:
		if want := "id: 2\n"; string(payload)[:len(want)] != want {
			t.Fatalf("expected replay starting at id 2, got %q", payload)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for replay")
	}
}

func TestHubKeepAlive(t *testing.T) {
	h := New(Config{KeepAliveInterval: 10 * time.Millisecond})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sub := h.Subscribe(ctx, "run-3", "")
	defer sub.Close()

	select {
	case payload := <-sub.C:
		if string(payload) != ":keep-alive\n\n" {
			t.Fatalf("expected keep-alive payload, got %q", payload)
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("timeout waiting for keep-alive")
	}
}
