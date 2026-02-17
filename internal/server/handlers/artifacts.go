package handlers

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/flowd-org/flowd/internal/artifacts"
	"github.com/flowd-org/flowd/internal/coredb"
	"github.com/flowd-org/flowd/internal/server/requestctx"
	"github.com/flowd-org/flowd/internal/server/response"
)

// ArtifactsConfig configures artifact download handling.
type ArtifactsConfig struct {
	MetadataStore *coredb.ArtifactStore
	ByteStore     *artifacts.Store
}

// NewArtifactsHandler returns an HTTP handler for GET /artifacts/{artifact_id}.
func NewArtifactsHandler(cfg ArtifactsConfig) http.Handler {
	return &artifactsHandler{meta: cfg.MetadataStore, bytes: cfg.ByteStore}
}

type artifactsHandler struct {
	meta  *coredb.ArtifactStore
	bytes *artifacts.Store
}

// safeLogError returns a safe error string and attributes for logging.
// If the error is an os.PathError, it omits the path to avoid leaking filesystem paths.
func safeLogError(err error) (code string, attrs []any) {
	var pe *os.PathError
	if errors.As(err, &pe) {
		return "artifact_path_error", []any{
			slog.String("op", pe.Op),
			slog.String("error", pe.Err.Error()),
		}
	}
	return "artifact_internal_error", []any{slog.String("error", err.Error())}
}

func (h *artifactsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		response.Write(w, response.New(http.StatusMethodNotAllowed, "method not allowed"))
		return
	}
	if h == nil || h.meta == nil || h.bytes == nil {
		response.Write(w, response.New(http.StatusServiceUnavailable, "artifact storage unavailable"))
		return
	}

	artifactID := strings.TrimPrefix(r.URL.Path, "/artifacts/")
	if artifactID == "" || strings.Contains(artifactID, "/") {
		response.Write(w, response.New(http.StatusNotFound, "artifact not found"))
		return
	}

	record, found, err := h.meta.Get(r.Context(), artifactID)
	if err != nil {
		if errors.Is(err, coredb.ErrArtifactInvalidID) {
			response.Write(w, response.New(http.StatusNotFound, "artifact not found"))
			return
		}
		code, attrs := safeLogError(err)
		if logger := requestctx.Logger(r.Context()); logger != nil {
			attrs = append(attrs,
				slog.String("code", code),
				slog.String("artifact_id", artifactID),
			)
			logger.Error("artifact.lookup.failed", attrs...)
		}
		response.Write(w, response.New(http.StatusInternalServerError, "artifact lookup failed"))
		return
	}
	if !found {
		response.Write(w, response.New(http.StatusNotFound, "artifact not found"))
		return
	}

	resolvedTenant, prob := resolveTenant(r.Context(), "")
	if prob != nil {
		response.Write(w, *prob)
		return
	}
	normalizedTenant := strings.TrimSpace(record.Tenant)
	if normalizedTenant == "" {
		normalizedTenant = defaultTenant
	}
	if normalizedTenant != resolvedTenant {
		response.Write(w, response.New(http.StatusNotFound, "artifact not found"))
		return
	}

	f, err := h.bytes.Open(record.ArtifactID)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			response.Write(w, response.New(http.StatusNotFound, "artifact not found"))
			return
		}
		code, attrs := safeLogError(err)
		if logger := requestctx.Logger(r.Context()); logger != nil {
			attrs = append(attrs,
				slog.String("code", code),
				slog.String("artifact_id", record.ArtifactID),
			)
			logger.Error("artifact.open.failed", attrs...)
		}
		response.Write(w, response.New(http.StatusInternalServerError, "artifact read failed"))
		return
	}
	defer f.Close()

	contentType := strings.TrimSpace(record.ContentType)
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	w.Header().Set("Content-Type", contentType)
	if record.SizeBytes >= 0 {
		w.Header().Set("Content-Length", strconv.FormatInt(record.SizeBytes, 10))
	}
	w.WriteHeader(http.StatusOK)
	written, copyErr := io.Copy(w, f)
	if copyErr != nil {
		reason := artifactDownloadStreamFailureReason(copyErr, r.Context().Err())
		if logger := requestctx.Logger(r.Context()); logger != nil {
			attrs := []any{
				slog.String("code", "artifact/download-stream-failed"),
				slog.String("reason", reason),
				slog.String("artifact_id", record.ArtifactID),
				slog.Int64("written_bytes", written),
			}
			if resolvedTenant != "" {
				attrs = append(attrs, slog.String("tenant", resolvedTenant))
			}
			if record.SizeBytes >= 0 {
				attrs = append(attrs, slog.Int64("expected_bytes", record.SizeBytes))
			}
			// Sanitize the error attribute to avoid leaking filesystem paths
			_, safeAttrs := safeLogError(copyErr)
			attrs = append(attrs, safeAttrs...)

			switch reason {
			case "context_canceled":
				logger.Info("artifact.download.stream_failed", attrs...)
			default:
				logger.Warn("artifact.download.stream_failed", attrs...)
			}
		}
	}
}

func artifactDownloadStreamFailureReason(copyErr error, ctxErr error) string {
	switch {
	case errors.Is(copyErr, context.Canceled) || errors.Is(ctxErr, context.Canceled):
		return "context_canceled"
	case errors.Is(copyErr, context.DeadlineExceeded) || errors.Is(ctxErr, context.DeadlineExceeded):
		return "context_deadline"
	default:
		return "io_error"
	}
}
