package sse

import (
	"context"
	"encoding/json"
	"fmt"
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

	h.Publish("run-1", Event{Event: "run.started", Data: `{"status":"queued"}`})

	select {
	case payload := <-sub.C:
		if got := string(payload); got == "" || !strings.HasPrefix(got, "id: 1\n") {
			t.Fatalf("expected payload with id 1, got %q", got)
		}
		if !strings.Contains(string(payload), "event: flowd") {
			t.Fatalf("expected flowd envelope, got %q", payload)
		}
		if !strings.Contains(string(payload), "retry: 3000") {
			t.Fatalf("expected retry directive, got %q", payload)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for event")
	}
}

func TestHubReplayFromLastEventID(t *testing.T) {
	h := New(Config{KeepAliveInterval: 0})
	h.nowFn = func() time.Time { return time.Unix(0, 0) }

	h.Publish("run-2", Event{ID: "1", Event: "run.started", Data: "{}"})
	h.Publish("run-2", Event{ID: "2", Event: "step.output", Data: `{"msg":"hello"}`})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sub := h.Subscribe(ctx, "run-2", "1")
	defer sub.Close()

	select {
	case payload := <-sub.C:
		if want := "id: 2\n"; string(payload)[:len(want)] != want {
			t.Fatalf("expected replay starting at id 2, got %q", payload)
		}
		if !strings.Contains(string(payload), "\"type\":\"step.output\"") {
			t.Fatalf("expected step.output type in payload, got %q", payload)
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
		if !strings.HasPrefix(string(payload), ":hb ") {
			t.Fatalf("expected heartbeat payload, got %q", payload)
		}
		if _, err := parseHeartbeatTimestamp(string(payload)); err != nil {
			t.Fatalf("expected RFC3339 heartbeat timestamp, got %q: %v", payload, err)
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("timeout waiting for keep-alive")
	}
}

func TestEncodeEventEnvelope(t *testing.T) {
	ts := time.Date(2026, 1, 10, 10, 0, 15, 0, time.UTC)
	payload, err := EncodeEvent(Event{
		ID:        "12",
		Event:     "run.started",
		Data:      `{"status":"queued"}`,
		Timestamp: ts,
		RunID:     "run-123",
		Tenant:    "tenant-a",
	})
	if err != nil {
		t.Fatalf("unexpected encode error: %v", err)
	}
	text := string(payload)
	if !strings.Contains(text, "event: flowd") {
		t.Fatalf("expected flowd envelope, got %q", text)
	}
	if !strings.Contains(text, "retry: 3000") {
		t.Fatalf("expected retry directive, got %q", text)
	}
	if !strings.HasPrefix(text, "id: 12\n") {
		t.Fatalf("expected id 12 prefix, got %q", text)
	}

	dataJSON := extractSSEData(text)
	var got struct {
		Seq    int64           `json:"seq"`
		TS     time.Time       `json:"ts"`
		Type   string          `json:"type"`
		RunID  string          `json:"run_id"`
		Tenant string          `json:"tenant"`
		Data   json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal([]byte(dataJSON), &got); err != nil {
		t.Fatalf("failed to unmarshal data envelope: %v (data=%q)", err, dataJSON)
	}
	if got.Seq != 12 {
		t.Fatalf("expected seq 12, got %d", got.Seq)
	}
	if !got.TS.Equal(ts) {
		t.Fatalf("expected timestamp %s, got %s", ts.Format(time.RFC3339), got.TS.Format(time.RFC3339))
	}
	if got.Type != "run.started" {
		t.Fatalf("expected type run.started, got %q", got.Type)
	}
	if got.RunID != "run-123" {
		t.Fatalf("expected run_id run-123, got %q", got.RunID)
	}
	if got.Tenant != "tenant-a" {
		t.Fatalf("expected tenant tenant-a, got %q", got.Tenant)
	}
}

func parseHeartbeatTimestamp(payload string) (time.Time, error) {
	payload = strings.TrimSpace(payload)
	if !strings.HasPrefix(payload, ":hb ") {
		return time.Time{}, fmt.Errorf("missing :hb prefix")
	}
	stamp := strings.TrimPrefix(payload, ":hb ")
	stamp = strings.TrimSpace(stamp)
	return time.Parse(time.RFC3339, stamp)
}

func extractSSEData(payload string) string {
	var builder strings.Builder
	for _, line := range strings.Split(payload, "\n") {
		if strings.HasPrefix(line, "data: ") {
			builder.WriteString(strings.TrimPrefix(line, "data: "))
		}
	}
	return builder.String()
}
