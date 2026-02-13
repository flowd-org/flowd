package handlers

import (
	"errors"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/flowd-org/flowd/internal/artifacts"
	"github.com/flowd-org/flowd/internal/coredb"
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
		response.Write(w, response.New(http.StatusInternalServerError, "artifact lookup failed", response.WithDetail(err.Error())))
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
	if record.Tenant != resolvedTenant {
		response.Write(w, response.New(http.StatusForbidden, "tenant mismatch",
			response.WithType(response.ProblemTypeTenantMismatch),
			response.WithExtension("code", response.ProblemCodeTenantMismatch),
			response.WithDetail("artifact is not accessible for this tenant"),
		))
		return
	}

	f, err := h.bytes.Open(record.ArtifactID)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			response.Write(w, response.New(http.StatusNotFound, "artifact not found"))
			return
		}
		response.Write(w, response.New(http.StatusInternalServerError, "artifact read failed", response.WithDetail(err.Error())))
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
	_, _ = io.Copy(w, f)
}
