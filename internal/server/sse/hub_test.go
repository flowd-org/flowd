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

// TestParseEventSeq tests the parseEventSeq helper function.
func TestParseEventSeq(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected int64
	}{
		{"empty string", "", 0},
		{"whitespace only", "   ", 0},
		{"valid seq", "123", 123},
		{"large seq", "999999999999", 999999999999},
		{"invalid string", "abc", 0},
		{"mixed alphanumeric", "12abc", 0},
		{"negative", "-5", -5},
		{"with leading zeros", "007", 7},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseEventSeq(tt.input)
			if got != tt.expected {
				t.Errorf("parseEventSeq(%q) = %d; want %d", tt.input, got, tt.expected)
			}
		})
	}
}

// TestDataValue tests the dataValue helper function.
func TestDataValue(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected any
	}{
		{"empty string", "", nil},
		{"whitespace only", "   ", nil},
		{"plain text", "hello world", "hello world"},
		{"json int", "42", json.RawMessage("42")},
		{"json float", "3.14", json.RawMessage("3.14")},
		{"json bool true", "true", json.RawMessage("true")},
		{"json bool false", "false", json.RawMessage("false")},
		{"json null", "null", json.RawMessage("null")},
		{"json array", "[1,2,3]", json.RawMessage("[1,2,3]")},
		{"json object", `{"key":"value"}`, json.RawMessage(`{"key":"value"}`)},
		{"quoted string", `"hello"`, json.RawMessage(`"hello"`)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := dataValue(tt.input)
			if !compareDataValue(got, tt.expected) {
				t.Errorf("dataValue(%q) = %v (type %T); want %v (type %T)", tt.input, got, got, tt.expected, tt.expected)
			}
		})
	}
}

func compareDataValue(a, b any) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	// Handle json.RawMessage comparison
	am, okA := a.(json.RawMessage)
	bm, okB := b.(json.RawMessage)
	if okA && okB {
		return string(am) == string(bm)
	}
	// For int64 vs float64 comparison with numbers
	if av, ok := a.(int64); ok {
		if bv, ok := b.(float64); ok {
			return av == int64(bv)
		}
		if bv, ok := b.(int64); ok {
			return av == bv
		}
	}
	return a == b
}

// TestHubRetentionTrimming tests that old events are pruned based on retention.
func TestHubRetentionTrimming(t *testing.T) {
	h := New(Config{
		KeepAliveInterval: 0,
		MaxBufferSize:     100,
		Retention:         time.Millisecond * 100,
	})
	baseTime := time.Unix(100, 0)
	h.nowFn = func() time.Time { return baseTime }

	// Add events at different times
	h.Publish("run-ret", Event{ID: "1", Event: "e1", Data: "{}"})
	h.nowFn = func() time.Time { return baseTime.Add(time.Millisecond * 50) }
	h.Publish("run-ret", Event{ID: "2", Event: "e2", Data: "{}"})
	h.nowFn = func() time.Time { return baseTime.Add(time.Millisecond * 150) }
	h.Publish("run-ret", Event{ID: "3", Event: "e3", Data: "{}"})

	// Subscribe should only get events after retention cutoff (event 2 and 3)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sub := h.Subscribe(ctx, "run-ret", "")
	var received []string
	for i := 0; i < 2; i++ {
		select {
		case payload := <-sub.C:
			received = append(received, extractSSEData(string(payload)))
		case <-time.After(time.Second):
			t.Fatal("timeout waiting for events")
		}
	}

	if len(received) != 2 {
		t.Fatalf("expected 2 events after retention pruning, got %d", len(received))
	}

	for i, wantType := range []string{"e2", "e3"} {
		var env struct {
			Type string `json:"type"`
			Seq  int64  `json:"seq"`
		}
		if err := json.Unmarshal([]byte(received[i]), &env); err != nil {
			t.Fatalf("failed to parse replayed retention event %d: %v (payload=%q)", i, err, received[i])
		}
		if env.Type != wantType {
			t.Fatalf("expected retention replay[%d] type %q, got %q", i, wantType, env.Type)
		}
	}

	select {
	case payload := <-sub.C:
		t.Fatalf("expected no extra replay after retained events, got %q", payload)
	case <-time.After(100 * time.Millisecond):
		// expected: no third replayed event
	}
}

// TestHubBufferSizeTrimming tests that events are trimmed when buffer exceeds MaxBufferSize.
func TestHubBufferSizeTrimming(t *testing.T) {
	h := New(Config{
		KeepAliveInterval: 0,
		MaxBufferSize:     3,
		Retention:         time.Hour,
	})
	baseTime := time.Unix(0, 0)
	h.nowFn = func() time.Time { return baseTime }

	for i := 1; i <= 5; i++ {
		h.Publish("run-buf", Event{ID: fmt.Sprintf("%d", i), Event: "e", Data: "{}"})
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sub := h.Subscribe(ctx, "run-buf", "")
	var received []string
	for i := 0; i < 3; i++ {
		select {
		case payload := <-sub.C:
			received = append(received, extractSSEData(string(payload)))
		case <-time.After(time.Second):
			t.Fatalf("timeout waiting for events, got %d so far", len(received))
		}
	}

	if len(received) != 3 {
		t.Fatalf("expected 3 events after buffer trimming, got %d", len(received))
	}

	for i, wantSeq := range []int64{3, 4, 5} {
		var env struct {
			Type string `json:"type"`
			Seq  int64  `json:"seq"`
		}
		if err := json.Unmarshal([]byte(received[i]), &env); err != nil {
			t.Fatalf("failed to parse replayed buffer event %d: %v (payload=%q)", i, err, received[i])
		}
		if env.Type != "e" {
			t.Fatalf("expected buffer replay[%d] type %q, got %q", i, "e", env.Type)
		}
		if env.Seq != wantSeq {
			t.Fatalf("expected buffer replay[%d] seq %d, got %d", i, wantSeq, env.Seq)
		}
	}

	select {
	case payload := <-sub.C:
		t.Fatalf("expected no extra replay after trimmed tail, got %q", payload)
	case <-time.After(100 * time.Millisecond):
		// expected: no fourth replayed event
	}
}

// TestHubReplayInvalidLastEventID tests replay behavior with invalid lastEventID.
func TestHubReplayInvalidLastEventID(t *testing.T) {
	h := New(Config{KeepAliveInterval: 0})
	baseTime := time.Unix(0, 0)
	h.nowFn = func() time.Time { return baseTime }

	h.Publish("run-replay", Event{ID: "1", Event: "e1", Data: "{}"})
	h.Publish("run-replay", Event{ID: "2", Event: "e2", Data: "{}"})

	// Request replay from non-existent ID - should return nothing
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sub := h.Subscribe(ctx, "run-replay", "999")

	select {
	case <-sub.C:
		t.Fatal("expected no events for invalid lastEventID")
	case <-time.After(100 * time.Millisecond):
		// Expected - no events
	}
}

// TestHubReplayExpiredCursor tests replay behavior when cursor is too old (beyond retention).
func TestHubReplayExpiredCursor(t *testing.T) {
	h := New(Config{
		KeepAliveInterval: 0,
		Retention:         time.Millisecond * 100,
	})
	baseTime := time.Unix(0, 0)
	h.nowFn = func() time.Time { return baseTime }

	h.Publish("run-exp", Event{ID: "1", Event: "e1", Data: "{}"})
	h.nowFn = func() time.Time { return baseTime.Add(time.Millisecond * 150) }
	h.Publish("run-exp", Event{ID: "2", Event: "e2", Data: "{}"})

	// Request replay from event 1 which should be expired and not found
	// The replay logic returns nothing when lastID is not in the buffer
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sub := h.Subscribe(ctx, "run-exp", "1")

	select {
	case <-sub.C:
		t.Fatal("expected no events for expired cursor (event 1 was pruned)")
	case <-time.After(100 * time.Millisecond):
		// Expected - no events since event 1 was pruned
	}
}

// TestSubscriptionCloseIdempotent tests that Close() can be called multiple times safely.
func TestSubscriptionCloseIdempotent(t *testing.T) {
	h := New(Config{KeepAliveInterval: 0})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sub := h.Subscribe(ctx, "run-close", "")
	defer sub.Close()

	// Call Close multiple times - should not panic
	sub.Close()
	sub.Close()
}

// TestFormatHeartbeat tests the FormatHeartbeat function.
func TestFormatHeartbeat(t *testing.T) {
	ts := time.Date(2026, 1, 15, 12, 30, 45, 0, time.UTC)
	payload := FormatHeartbeat(ts)

	if !strings.HasPrefix(string(payload), ":hb ") {
		t.Fatalf("expected :hb prefix, got %q", payload)
	}
	if !strings.HasSuffix(string(payload), "\n\n") {
		t.Fatalf("expected trailing \\n\\n, got %q", payload)
	}

	// Verify timestamp is parseable
	gotTime, err := time.Parse(time.RFC3339, strings.TrimSpace(strings.TrimPrefix(string(payload), ":hb ")))
	if err != nil {
		t.Fatalf("heartbeat timestamp not RFC3339: %v", err)
	}
	if !gotTime.Equal(ts.UTC()) {
		t.Errorf("expected timestamp %s, got %s", ts.Format(time.RFC3339), gotTime.Format(time.RFC3339))
	}
}

// TestFormatHeartbeatAutoTimestamp tests FormatHeartbeat with zero time.
func TestFormatHeartbeatAutoTimestamp(t *testing.T) {
	payload := FormatHeartbeat(time.Time{})
	if !strings.HasPrefix(string(payload), ":hb ") {
		t.Fatalf("expected :hb prefix, got %q", payload)
	}
}

// TestEncodeEventEnvelopeMinimalPayload tests EncodeEvent with a minimal valid payload.
func TestEncodeEventEnvelopeMinimalPayload(t *testing.T) {
	// This test verifies the normal encode path in formatEvent.
	ev := Event{ID: "1", Event: "test", Data: `{"valid":true}`}
	payload, err := EncodeEvent(ev)
	if err != nil {
		t.Fatalf("unexpected encode error: %v", err)
	}
	if !strings.Contains(string(payload), "event: flowd") {
		t.Fatalf("expected flowd event, got %q", payload)
	}
}

// TestHubPublishEmptyRunID tests Publish with empty RunID.
func TestHubPublishBackfillsEventRunID(t *testing.T) {
	h := New(Config{KeepAliveInterval: 0})
	baseTime := time.Unix(0, 0)
	h.nowFn = func() time.Time { return baseTime }

	// Publish without specifying runID parameter
	h.Publish("run-empty", Event{Event: "e1", Data: "{}"})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	sub := h.Subscribe(ctx, "run-empty", "")

	select {
	case payload := <-sub.C:
		if !strings.Contains(string(payload), `"type":"e1"`) {
			t.Fatalf("expected event type e1 in payload, got %q", payload)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for event")
	}
}

// TestHubMultipleSubscribers tests multiple subscribers receive the same events.
func TestHubMultipleSubscribers(t *testing.T) {
	h := New(Config{KeepAliveInterval: 0})
	baseTime := time.Unix(0, 0)
	h.nowFn = func() time.Time { return baseTime }

	ctx1, cancel1 := context.WithCancel(context.Background())
	defer cancel1()
	sub1 := h.Subscribe(ctx1, "run-multi", "")

	// Publish first event
	h.Publish("run-multi", Event{ID: "1", Event: "e1", Data: "{}"})

	// Add second subscriber after first event
	ctx2, cancel2 := context.WithCancel(context.Background())
	defer cancel2()
	sub2 := h.Subscribe(ctx2, "run-multi", "")

	// Publish second event - both subscribers should receive it
	h.Publish("run-multi", Event{ID: "2", Event: "e2", Data: "{}"})

	readTypes := func(ch <-chan []byte, n int, who string) []string {
		got := make([]string, 0, n)
		for i := 0; i < n; i++ {
			select {
			case payload := <-ch:
				var env struct {
					Type string `json:"type"`
				}
				data := extractSSEData(string(payload))
				if err := json.Unmarshal([]byte(data), &env); err != nil {
					t.Fatalf("failed to parse %s replay event %d: %v (payload=%q)", who, i, err, payload)
				}
				got = append(got, env.Type)
			case <-time.After(time.Second):
				t.Fatalf("timeout waiting for %s event %d/%d", who, i+1, n)
			}
		}
		return got
	}

	sub1Types := readTypes(sub1.C, 2, "sub1")
	sub2Types := readTypes(sub2.C, 2, "sub2")

	if fmt.Sprintf("%v", sub1Types) != "[e1 e2]" {
		t.Fatalf("expected sub1 to receive [e1 e2], got %v", sub1Types)
	}
	if fmt.Sprintf("%v", sub2Types) != "[e1 e2]" {
		t.Fatalf("expected sub2 to receive [e1 e2], got %v", sub2Types)
	}

	select {
	case payload := <-sub1.C:
		t.Fatalf("expected no extra replay for sub1, got %q", payload)
	case <-time.After(100 * time.Millisecond):
	}
	select {
	case payload := <-sub2.C:
		t.Fatalf("expected no extra replay for sub2, got %q", payload)
	case <-time.After(100 * time.Millisecond):
	}
}

// TestHubBroadcastDrop tests that slow subscribers don't block the stream.
func TestHubBroadcastDrop(t *testing.T) {
	h := New(Config{KeepAliveInterval: 0})
	baseTime := time.Unix(0, 0)
	h.nowFn = func() time.Time { return baseTime }

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create a subscriber with small buffer that won't read
	ch := make(chan []byte, 1)
	subCtx, subCancel := context.WithCancel(ctx)
	defer subCancel()
	stream := h.getOrCreateStream("run-drop")
	stream.addSubscriber(subCtx, ch, 0, h.nowFn)

	// Publish many events quickly - should drop some
	for i := 0; i < 10; i++ {
		h.Publish("run-drop", Event{ID: fmt.Sprintf("%d", i+1), Event: "e", Data: "{}"})
	}

	// Only the first few should be received due to buffer size
	receivedCount := 0
	for i := 0; i < 10; i++ {
		select {
		case <-ch:
			receivedCount++
		default:
			// No more events available
		}
	}

	if receivedCount == 0 {
		t.Fatal("expected at least some events to be received")
	}
}

// TestHubConcurrentAccess tests that concurrent access is safe.
func TestHubConcurrentAccess(t *testing.T) {
	h := New(Config{
		KeepAliveInterval: 0,
		MaxBufferSize:     100,
		Retention:         time.Hour,
	})

	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func(runID string) {
			for j := 0; j < 50; j++ {
				h.Publish(runID, Event{Event: "e", Data: "{}"})
			}
			done <- true
		}(fmt.Sprintf("run-concurrent-%d", i))
	}

	for i := 0; i < 10; i++ {
		select {
		case <-done:
		case <-time.After(time.Second):
			t.Fatalf("timeout waiting for goroutine %d", i)
		}
	}
}
