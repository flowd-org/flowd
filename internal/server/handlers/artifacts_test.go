package handlers

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/flowd-org/flowd/internal/artifacts"
	"github.com/flowd-org/flowd/internal/coredb"
	"github.com/flowd-org/flowd/internal/paths"
	"github.com/flowd-org/flowd/internal/server/requestctx"
)

type artifactCaptureHandler struct {
	records []capturedRecord
}

type capturedRecord struct {
	level slog.Level
	msg   string
	attrs map[string]any
}

func (h *artifactCaptureHandler) Enabled(context.Context, slog.Level) bool { return true }

func (h *artifactCaptureHandler) Handle(_ context.Context, r slog.Record) error {
	attrs := make(map[string]any)
	r.Attrs(func(attr slog.Attr) bool {
		attrs[attr.Key] = attr.Value.Any()
		return true
	})
	h.records = append(h.records, capturedRecord{level: r.Level, msg: r.Message, attrs: attrs})
	return nil
}

func (h *artifactCaptureHandler) WithAttrs([]slog.Attr) slog.Handler { return h }
func (h *artifactCaptureHandler) WithGroup(string) slog.Handler      { return h }

type flakyResponseWriter struct {
	header       http.Header
	writeCalls   int
	failOnCall   int
	failWithErr  error
	writtenBytes int
}

func (w *flakyResponseWriter) Header() http.Header {
	if w.header == nil {
		w.header = make(http.Header)
	}
	return w.header
}

func (w *flakyResponseWriter) WriteHeader(int) {}

func (w *flakyResponseWriter) Write(p []byte) (int, error) {
	w.writeCalls++
	if w.failOnCall > 0 && w.writeCalls >= w.failOnCall {
		if w.failWithErr != nil {
			return 0, w.failWithErr
		}
		return 0, errors.New("write failed")
	}
	w.writtenBytes += len(p)
	return len(p), nil
}

func TestArtifactsHandler_LogsStreamingFailures(t *testing.T) {
	const artifactID = "018f22b0-1234-7abc-8def-0123456789ab"

	dataDir := t.TempDir()
	prevDataDir := paths.DataDir()
	paths.SetDataDirOverride(dataDir)
	t.Cleanup(func() { paths.SetDataDirOverride(prevDataDir) })

	db, err := coredb.Open(context.Background(), coredb.Options{DataDir: dataDir})
	if err != nil {
		t.Fatalf("open coredb: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	metaStore := coredb.NewArtifactStore(db)
	byteStore := artifacts.NewStore(artifacts.Options{})
	if _, err := byteStore.Write(context.Background(), artifactID, strings.NewReader(strings.Repeat("a", 128<<10))); err != nil {
		t.Fatalf("write artifact bytes: %v", err)
	}
	if err := metaStore.Create(context.Background(), coredb.ArtifactRecord{
		ArtifactID:  artifactID,
		Tenant:      "acme",
		JobID:       "demo",
		RunID:       "run-1",
		Name:        "stdout",
		ContentType: "text/plain; charset=utf-8",
		SizeBytes:   128 << 10,
	}); err != nil {
		t.Fatalf("create artifact metadata: %v", err)
	}

	t.Run("context canceled copy error logs info", func(t *testing.T) {
		capturer := &artifactCaptureHandler{}
		logger := slog.New(capturer)
		ctx := requestctx.WithLogger(context.Background(), logger)
		ctx = requestctx.WithPrincipal(ctx, "tester")
		ctx = requestctx.WithTenant(ctx, "acme")
		req := httptest.NewRequest(http.MethodGet, "/artifacts/"+artifactID, nil).WithContext(ctx)

		w := &flakyResponseWriter{failOnCall: 2, failWithErr: context.Canceled}
		NewArtifactsHandler(ArtifactsConfig{MetadataStore: metaStore, ByteStore: byteStore}).ServeHTTP(w, req)

		if len(capturer.records) == 0 {
			t.Fatalf("expected at least one log record")
		}
		last := capturer.records[len(capturer.records)-1]
		if last.msg != "artifact.download.stream_failed" {
			t.Fatalf("unexpected log message %q", last.msg)
		}
		if last.level != slog.LevelInfo {
			t.Fatalf("expected info level, got %v", last.level)
		}
		if got, _ := last.attrs["code"].(string); got != "artifact/download-stream-failed" {
			t.Fatalf("expected code attr, got %v", last.attrs["code"])
		}
		if got, _ := last.attrs["reason"].(string); got != "context_canceled" {
			t.Fatalf("expected reason context_canceled, got %v", last.attrs["reason"])
		}
		if got, _ := last.attrs["artifact_id"].(string); got != artifactID {
			t.Fatalf("expected artifact_id %q, got %v", artifactID, last.attrs["artifact_id"])
		}
	})

	t.Run("generic copy error logs warn", func(t *testing.T) {
		capturer := &artifactCaptureHandler{}
		logger := slog.New(capturer)
		ctx := requestctx.WithLogger(context.Background(), logger)
		ctx = requestctx.WithPrincipal(ctx, "tester")
		ctx = requestctx.WithTenant(ctx, "acme")
		req := httptest.NewRequest(http.MethodGet, "/artifacts/"+artifactID, nil).WithContext(ctx)

		w := &flakyResponseWriter{failOnCall: 2, failWithErr: io.ErrUnexpectedEOF}
		NewArtifactsHandler(ArtifactsConfig{MetadataStore: metaStore, ByteStore: byteStore}).ServeHTTP(w, req)

		last := capturer.records[len(capturer.records)-1]
		if last.msg != "artifact.download.stream_failed" {
			t.Fatalf("unexpected log message %q", last.msg)
		}
		if last.level != slog.LevelWarn {
			t.Fatalf("expected warn level, got %v", last.level)
		}
		if got, _ := last.attrs["reason"].(string); got != "io_error" {
			t.Fatalf("expected reason io_error, got %v", last.attrs["reason"])
		}
	})
}
