// SPDX-License-Identifier: AGPL-3.0-or-later

package logctx

import (
	"context"
	"log/slog"
)

type loggerKey struct{}

var ctxLoggerKey = &loggerKey{}

// WithLogger stores the logger in the context for downstream consumers.
func WithLogger(ctx context.Context, logger *slog.Logger) context.Context {
	if ctx == nil || logger == nil {
		return ctx
	}
	return context.WithValue(ctx, ctxLoggerKey, logger)
}

// Logger extracts a logger from context, if present.
func Logger(ctx context.Context) *slog.Logger {
	if ctx == nil {
		return nil
	}
	logger, _ := ctx.Value(ctxLoggerKey).(*slog.Logger)
	return logger
}

// WrapLogger returns a logger that applies the supplied scrubber to message and string attributes.
// If logger is nil or scrub is nil, it returns the original logger.
func WrapLogger(logger *slog.Logger, scrub func(string) string) *slog.Logger {
	if logger == nil || scrub == nil {
		return logger
	}
	return slog.New(&scrubHandler{next: logger.Handler(), scrub: scrub})
}

type scrubHandler struct {
	next  slog.Handler
	scrub func(string) string
}

func (h *scrubHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.next.Enabled(ctx, level)
}

func (h *scrubHandler) Handle(ctx context.Context, r slog.Record) error {
	if h == nil || h.scrub == nil {
		return h.next.Handle(ctx, r)
	}
	msg := h.scrub(r.Message)
	copy := slog.NewRecord(r.Time, r.Level, msg, r.PC)
	r.Attrs(func(attr slog.Attr) bool {
		copy.AddAttrs(scrubAttr(attr, h.scrub))
		return true
	})
	return h.next.Handle(ctx, copy)
}

func (h *scrubHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	if len(attrs) == 0 {
		return h
	}
	clean := make([]slog.Attr, 0, len(attrs))
	for _, attr := range attrs {
		clean = append(clean, scrubAttr(attr, h.scrub))
	}
	return &scrubHandler{next: h.next.WithAttrs(clean), scrub: h.scrub}
}

func (h *scrubHandler) WithGroup(name string) slog.Handler {
	return &scrubHandler{next: h.next.WithGroup(name), scrub: h.scrub}
}

func scrubAttr(attr slog.Attr, scrub func(string) string) slog.Attr {
	if scrub == nil {
		return attr
	}
	attr.Value = scrubValue(attr.Value, scrub)
	return attr
}

func scrubValue(val slog.Value, scrub func(string) string) slog.Value {
	if scrub == nil {
		return val
	}
	switch val.Kind() {
	case slog.KindString:
		return slog.StringValue(scrub(val.String()))
	case slog.KindAny:
		if s, ok := val.Any().(string); ok {
			return slog.StringValue(scrub(s))
		}
		return val
	case slog.KindGroup:
		attrs := val.Group()
		if len(attrs) == 0 {
			return val
		}
		clean := make([]slog.Attr, 0, len(attrs))
		for _, attr := range attrs {
			clean = append(clean, scrubAttr(attr, scrub))
		}
		return slog.GroupValue(clean...)
	default:
		return val
	}
}
