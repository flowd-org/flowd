// SPDX-License-Identifier: AGPL-3.0-or-later
// SPDX-License-Identifier: AGPL-3.0-or-later

package logctx

import (
	"context"
	"strings"
	"testing"

	"log/slog"
)

type testHandler struct {
	records []slog.Record
}

func (h *testHandler) Enabled(context.Context, slog.Level) bool { return true }

func (h *testHandler) Handle(_ context.Context, r slog.Record) error {
	copy := slog.NewRecord(r.Time, r.Level, r.Message, r.PC)
	r.Attrs(func(attr slog.Attr) bool {
		copy.AddAttrs(attr)
		return true
	})
	h.records = append(h.records, copy)
	return nil
}

func (h *testHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	clone := &testHandler{records: append([]slog.Record{}, h.records...)}
	return &testHandlerWithAttrs{base: clone, attrs: attrs}
}

func (h *testHandler) WithGroup(string) slog.Handler { return h }

type testHandlerWithAttrs struct {
	base  *testHandler
	attrs []slog.Attr
}

func (h *testHandlerWithAttrs) Enabled(ctx context.Context, level slog.Level) bool {
	return h.base.Enabled(ctx, level)
}

func (h *testHandlerWithAttrs) Handle(ctx context.Context, r slog.Record) error {
	copy := slog.NewRecord(r.Time, r.Level, r.Message, r.PC)
	for _, attr := range h.attrs {
		copy.AddAttrs(attr)
	}
	r.Attrs(func(attr slog.Attr) bool {
		copy.AddAttrs(attr)
		return true
	})
	h.base.records = append(h.base.records, copy)
	return nil
}

func (h *testHandlerWithAttrs) WithAttrs(attrs []slog.Attr) slog.Handler {
	combined := append([]slog.Attr{}, h.attrs...)
	combined = append(combined, attrs...)
	return &testHandlerWithAttrs{base: h.base, attrs: combined}
}

func (h *testHandlerWithAttrs) WithGroup(name string) slog.Handler {
	return h.base.WithGroup(name)
}

func TestWrapLoggerScrubsMessageAndAttrs(t *testing.T) {
	handler := &testHandler{}
	logger := slog.New(handler)

	scrubbed := WrapLogger(logger, func(msg string) string {
		return strings.ReplaceAll(msg, "secret", "$$REDACTED$$")
	})

	group := slog.Group("meta",
		slog.String("detail", "secret-value"),
		slog.String("ok", "visible"),
	)

	scrubbed.Info("secret message", slog.String("error", "secret path"), group)

	if len(handler.records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(handler.records))
	}
	rec := handler.records[0]
	if rec.Message != "$$REDACTED$$ message" {
		t.Fatalf("expected scrubbed message, got %q", rec.Message)
	}

	attrs := map[string]string{}
	rec.Attrs(func(attr slog.Attr) bool {
		if attr.Value.Kind() == slog.KindString {
			attrs[attr.Key] = attr.Value.String()
		}
		if attr.Value.Kind() == slog.KindGroup {
			for _, g := range attr.Value.Group() {
				if g.Value.Kind() == slog.KindString {
					attrs["meta."+g.Key] = g.Value.String()
				}
			}
		}
		return true
	})

	if attrs["error"] != "$$REDACTED$$ path" {
		t.Fatalf("expected scrubbed error attr, got %q", attrs["error"])
	}
	if attrs["meta.detail"] != "$$REDACTED$$-value" {
		t.Fatalf("expected scrubbed group attr, got %q", attrs["meta.detail"])
	}
	if attrs["meta.ok"] != "visible" {
		t.Fatalf("expected non-secret attr preserved, got %q", attrs["meta.ok"])
	}
}
