package sse

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"
)

const (
	defaultKeepAliveInterval = 15 * time.Second
	defaultBufferSize        = 1000
	defaultRetention         = 5 * time.Minute
)

// Event represents an SSE payload delivered to subscribers.
type Event struct {
	ID        string
	Event     string
	Data      string
	Timestamp time.Time
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

	stream := h.getOrCreateStream(runID)
	stored := stream.add(ev, h.cfg.MaxBufferSize, h.cfg.Retention, h.nowFn())
	stream.broadcast(formatEvent(stored))
}

// Subscribe registers a subscriber for a run and replays buffered events after the provided lastEventID.
func (h *Hub) Subscribe(ctx context.Context, runID, lastEventID string) *Subscription {
	stream := h.getOrCreateStream(runID)
	ch := make(chan []byte, 32)
	subCtx, cancel := context.WithCancel(ctx)
	stream.addSubscriber(subCtx, ch, h.cfg.KeepAliveInterval)
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

func (rs *runStream) addSubscriber(ctx context.Context, ch chan<- []byte, keepAlive time.Duration) {
	sub := &subscriber{
		ctx:       ctx,
		ch:        ch,
		keepAlive: keepAlive,
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
	for i, ev := range rs.events {
		if ev.ID == lastID {
			start = i + 1
			break
		}
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
			case s.ch <- []byte(":keep-alive\n\n"):
			default:
			}
		}
	}
}

func formatEvent(ev Event) []byte {
	var builder strings.Builder
	if ev.ID != "" {
		builder.WriteString("id: ")
		builder.WriteString(ev.ID)
		builder.WriteByte('\n')
	}
	if ev.Event != "" {
		builder.WriteString("event: ")
		builder.WriteString(ev.Event)
		builder.WriteByte('\n')
	}
	for _, line := range strings.Split(ev.Data, "\n") {
		builder.WriteString("data: ")
		builder.WriteString(line)
		builder.WriteByte('\n')
	}
	builder.WriteByte('\n')
	return []byte(builder.String())
}
