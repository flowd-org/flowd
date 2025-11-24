// SPDX-License-Identifier: AGPL-3.0-or-later

package tracing

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/flowd-org/flowd/internal/server/requestctx"
)

// Attribute represents a key/value pair attached to a span.
type Attribute struct {
	Key   string
	Value any
}

// Well-known attribute keys used across persistence tracing.
const (
	AttrPersistDriver   = "persist.driver"
	AttrPersistOp       = "persist.op"
	AttrPersistKeyspace = "persist.keyspace"
	AttrRunID           = "run_id"
)

// String returns a string attribute.
func String(key, value string) Attribute {
	return Attribute{Key: key, Value: value}
}

// Int returns an integer attribute.
func Int(key string, value int) Attribute {
	return Attribute{Key: key, Value: value}
}

// Int64 returns an int64 attribute.
func Int64(key string, value int64) Attribute {
	return Attribute{Key: key, Value: value}
}

// PersistDriver returns an attribute describing the persistence driver.
func PersistDriver(value string) Attribute {
	return String(AttrPersistDriver, value)
}

// PersistOp returns an attribute describing the persistence operation.
func PersistOp(value string) Attribute {
	return String(AttrPersistOp, value)
}

// PersistKeyspace returns an attribute describing the persistence keyspace.
func PersistKeyspace(value string) Attribute {
	return String(AttrPersistKeyspace, value)
}

// RunID returns an attribute describing the run identifier.
func RunID(value string) Attribute {
	if value == "" {
		return Attribute{}
	}
	return String(AttrRunID, value)
}

type spanKey struct{}

// Span represents a lightweight tracing span backed by structured logging.
type Span struct {
	name   string
	start  time.Time
	logger *slog.Logger

	mu    sync.Mutex
	attrs map[string]any
	err   error
	ended bool
}

// Start begins a new span anchored to the supplied context.
// The returned context currently stores the span and can be forwarded to child operations.
func Start(ctx context.Context, name string, attrs ...Attribute) (context.Context, *Span) {
	if ctx == nil {
		ctx = context.Background()
	}
	logger := requestctx.Logger(ctx)
	if logger == nil {
		logger = slog.Default()
	}

	span := &Span{
		name:   name,
		start:  time.Now(),
		logger: logger,
		attrs:  make(map[string]any),
	}
	span.SetAttributes(attrs...)

	ctx = context.WithValue(ctx, spanKey{}, span)
	return ctx, span
}

// FromContext extracts a span from the supplied context, if present.
func FromContext(ctx context.Context) *Span {
	span, _ := ctx.Value(spanKey{}).(*Span)
	return span
}

// SetAttributes appends new attributes to the span.
func (s *Span) SetAttributes(attrs ...Attribute) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, attr := range attrs {
		if attr.Key == "" {
			continue
		}
		s.attrs[attr.Key] = attr.Value
	}
}

// RecordError records the supplied error against the span.
func (s *Span) RecordError(err error) {
	if s == nil || err == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.err = err
}

// End completes the span and emits a structured log line with duration and attributes.
func (s *Span) End() {
	if s == nil {
		return
	}
	s.mu.Lock()
	if s.ended {
		s.mu.Unlock()
		return
	}
	duration := time.Since(s.start)
	err := s.err
	attrs := make(map[string]any, len(s.attrs)+2)
	for k, v := range s.attrs {
		attrs[k] = v
	}
	s.ended = true
	s.mu.Unlock()

	attrs["span"] = s.name
	attrs["duration_ms"] = float64(duration.Microseconds()) / 1000.0

	var logAttrs []any
	for k, v := range attrs {
		logAttrs = append(logAttrs, slog.Any(k, v))
	}
	if err != nil {
		logAttrs = append(logAttrs, slog.String("error", err.Error()))
		s.logger.Error("trace.span_end", logAttrs...)
		return
	}
	s.logger.Debug("trace.span_end", logAttrs...)
}

// EndWithError records the supplied error (if any) and ends the span.
func EndWithError(span *Span, err error, attrs ...Attribute) {
	if span == nil {
		return
	}
	if len(attrs) > 0 {
		span.SetAttributes(attrs...)
	}
	if err != nil {
		span.RecordError(err)
	}
	span.End()
}

// End observes the error pointer (if non-nil) and finalises the span.
func End(span *Span, errPtr *error, attrs ...Attribute) {
	if span == nil {
		return
	}
	if len(attrs) > 0 {
		span.SetAttributes(attrs...)
	}
	if errPtr != nil && *errPtr != nil {
		span.RecordError(*errPtr)
	}
	span.End()
}
