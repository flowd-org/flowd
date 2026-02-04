package sse

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	defaultKeepAliveInterval = 15 * time.Second
	defaultBufferSize        = 1000
	defaultRetention         = 5 * time.Minute
	flowdEventName           = "flowd"
)

// Event represents an SSE payload delivered to subscribers.
type Event struct {
	ID        string
	Event     string
	Data      string
	Timestamp time.Time
	RunID     string
	Tenant    string
}

// Config controls Hub behaviour.
type Config struct {
	KeepAliveInterval time.Duration
	MaxBufferSize     int
	Retention         time.Duration
}

// Hub multiplexes run events to SSE subscribers.
type Hub struct {
	cfg   Config
	mu    sync.RWMutex
	runs  map[string]*runStream
	nowFn func() time.Time
}

// Subscription represents an active SSE stream.
type Subscription struct {
	C    <-chan []byte
	stop context.CancelFunc
}

// New creates a Hub with defaults.
func New(cfg Config) *Hub {
	if cfg.KeepAliveInterval <= 0 {
		cfg.KeepAliveInterval = defaultKeepAliveInterval
	}
	if cfg.MaxBufferSize <= 0 {
		cfg.MaxBufferSize = defaultBufferSize
	}
	if cfg.Retention <= 0 {
		cfg.Retention = defaultRetention
	}
	return &Hub{
		cfg:   cfg,
		runs:  make(map[string]*runStream),
		nowFn: time.Now,
	}
}

// Publish records the event and broadcasts it to subscribers.
func (h *Hub) Publish(runID string, ev Event) {
	if ev.Timestamp.IsZero() {
		ev.Timestamp = h.nowFn()
	}
	if ev.RunID == "" {
		ev.RunID = runID
	}

	stream := h.getOrCreateStream(runID)
	stored := stream.add(ev, h.cfg.MaxBufferSize, h.cfg.Retention, h.nowFn())
	stream.broadcast(formatEvent(stored))
}

// Subscribe registers a subscriber for a run and replays buffered events after the provided lastEventID.
func (h *Hub) Subscribe(ctx context.Context, runID, lastEventID string) *Subscription {
	stream := h.getOrCreateStream(runID)
	bufSize := 32
	if h.cfg.MaxBufferSize > bufSize {
		bufSize = h.cfg.MaxBufferSize
	}
	ch := make(chan []byte, bufSize)
	subCtx, cancel := context.WithCancel(ctx)
	stream.addSubscriber(subCtx, ch, h.cfg.KeepAliveInterval, h.nowFn)
	stream.replay(ch, lastEventID)
	return &Subscription{
		C:    ch,
		stop: cancel,
	}
}

// Close terminates the subscription.
func (s *Subscription) Close() {
	if s.stop != nil {
		s.stop()
	}
}

func (h *Hub) getOrCreateStream(runID string) *runStream {
	h.mu.Lock()
	defer h.mu.Unlock()
	stream, ok := h.runs[runID]
	if !ok {
		stream = newRunStream()
		h.runs[runID] = stream
	}
	return stream
}

type runStream struct {
	mu          sync.RWMutex
	events      []Event
	subscribers map[*subscriber]struct{}
	seq         int64
}

type subscriber struct {
	ctx        context.Context
	ch         chan<- []byte
	keepAlive  time.Duration
	keepTicker *time.Ticker
	nowFn      func() time.Time
}

func newRunStream() *runStream {
	return &runStream{
		events:      make([]Event, 0),
		subscribers: make(map[*subscriber]struct{}),
	}
}

func (rs *runStream) add(ev Event, maxSize int, retention time.Duration, now time.Time) Event {
	rs.mu.Lock()
	defer rs.mu.Unlock()

	rs.seq++
	if ev.ID == "" {
		ev.ID = fmt.Sprintf("%d", rs.seq)
	}
	if ev.Timestamp.IsZero() {
		ev.Timestamp = now.UTC()
	}
	rs.events = append(rs.events, ev)

	// prune retention
	cutoff := now.Add(-retention)
	idx := 0
	for idx < len(rs.events) && rs.events[idx].Timestamp.Before(cutoff) {
		idx++
	}
	if idx > 0 {
		rs.events = append([]Event{}, rs.events[idx:]...)
	}

	if len(rs.events) > maxSize {
		rs.events = rs.events[len(rs.events)-maxSize:]
	}
	return ev
}

func (rs *runStream) addSubscriber(ctx context.Context, ch chan<- []byte, keepAlive time.Duration, nowFn func() time.Time) {
	if nowFn == nil {
		nowFn = time.Now
	}
	sub := &subscriber{
		ctx:       ctx,
		ch:        ch,
		keepAlive: keepAlive,
		nowFn:     nowFn,
	}
	rs.mu.Lock()
	rs.subscribers[sub] = struct{}{}
	rs.mu.Unlock()

	go sub.run(func() {
		rs.removeSubscriber(sub)
	})
}

func (rs *runStream) removeSubscriber(sub *subscriber) {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	delete(rs.subscribers, sub)
}

func (rs *runStream) replay(ch chan<- []byte, lastID string) {
	rs.mu.RLock()
	defer rs.mu.RUnlock()
	if lastID == "" {
		for _, ev := range rs.events {
			ch <- formatEvent(ev)
		}
		return
	}
	start := 0
	found := false
	for i, ev := range rs.events {
		if ev.ID == lastID {
			start = i + 1
			found = true
			break
		}
	}
	if !found {
		return
	}
	for _, ev := range rs.events[start:] {
		ch <- formatEvent(ev)
	}
}

func (rs *runStream) broadcast(payload []byte) {
	rs.mu.RLock()
	defer rs.mu.RUnlock()
	for sub := range rs.subscribers {
		select {
		case sub.ch <- payload:
		default:
			// drop if slow; keep stream responsive
		}
	}
}

func (s *subscriber) run(onClose func()) {
	defer func() {
		if s.keepTicker != nil {
			s.keepTicker.Stop()
		}
		if onClose != nil {
			onClose()
		}
		close(s.ch)
	}()

	if s.keepAlive > 0 {
		s.keepTicker = time.NewTicker(s.keepAlive)
		defer s.keepTicker.Stop()
	}

	if s.keepTicker == nil {
		<-s.ctx.Done()
		return
	}

	for {
		select {
		case <-s.ctx.Done():
			return
		case <-s.keepTicker.C:
			select {
			case s.ch <- FormatHeartbeat(s.nowFn()):
			default:
			}
		}
	}
}

func formatEvent(ev Event) []byte {
	payload, err := EncodeEvent(ev)
	if err != nil {
		payload = []byte("event: " + flowdEventName + "\n" + "retry: 3000\n" + "data: {}\n\n")
	}
	return payload
}

type envelope struct {
	Seq    int64     `json:"seq"`
	TS     time.Time `json:"ts"`
	Type   string    `json:"type"`
	RunID  string    `json:"run_id,omitempty"`
	Tenant string    `json:"tenant,omitempty"`
	Data   any       `json:"data,omitempty"`
}

// EncodeEvent formats the event using the Core SoT v1.0.1 envelope and SSE framing.
func EncodeEvent(ev Event) ([]byte, error) {
	if ev.Timestamp.IsZero() {
		ev.Timestamp = time.Now().UTC()
	}
	seq := parseEventSeq(ev.ID)
	data := dataValue(ev.Data)
	if data == nil && ev.Data != "" {
		data = ev.Data
	}
	payload, err := json.Marshal(envelope{
		Seq:    seq,
		TS:     ev.Timestamp.UTC(),
		Type:   ev.Event,
		RunID:  ev.RunID,
		Tenant: ev.Tenant,
		Data:   data,
	})
	if err != nil {
		return nil, err
	}
	return formatPayload(ev.ID, payload), nil
}

// FormatHeartbeat returns the SSE heartbeat comment payload.
func FormatHeartbeat(ts time.Time) []byte {
	if ts.IsZero() {
		ts = time.Now().UTC()
	}
	return []byte(fmt.Sprintf(":hb %s\n\n", ts.UTC().Format(time.RFC3339)))
}

func parseEventSeq(id string) int64 {
	id = strings.TrimSpace(id)
	if id == "" {
		return 0
	}
	seq, err := strconv.ParseInt(id, 10, 64)
	if err != nil {
		return 0
	}
	return seq
}

func dataValue(input string) any {
	input = strings.TrimSpace(input)
	if input == "" {
		return nil
	}
	var raw json.RawMessage
	if err := json.Unmarshal([]byte(input), &raw); err != nil {
		return input
	}
	return raw
}

func formatPayload(id string, payload []byte) []byte {
	var builder strings.Builder
	if strings.TrimSpace(id) != "" {
		builder.WriteString("id: ")
		builder.WriteString(strings.TrimSpace(id))
		builder.WriteByte('\n')
	}
	builder.WriteString("event: ")
	builder.WriteString(flowdEventName)
	builder.WriteByte('\n')
	builder.WriteString("retry: 3000\n")
	for _, line := range strings.Split(string(payload), "\n") {
		builder.WriteString("data: ")
		builder.WriteString(line)
		builder.WriteByte('\n')
	}
	builder.WriteByte('\n')
	return []byte(builder.String())
}
