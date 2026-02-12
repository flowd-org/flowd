package handlers

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/flowd-org/flowd/internal/coredb"
	"github.com/flowd-org/flowd/internal/server/response"
)

const (
	readyzNotReadyProblemType = "https://flowd.org/problems/not-ready"
	readyzPingTimeout         = 2 * time.Second
	readyzStorageTimeout      = 2 * time.Second
)

type coreDBReadyFunc func(*http.Request) error
type readyzStorageStatsFunc func(*http.Request) (coredb.StorageStats, error)

type readyzHandler struct {
	coredbReady  coreDBReadyFunc
	storageStats readyzStorageStatsFunc
}

// NewReadyzHandler returns an HTTP handler for GET /readyz.
func NewReadyzHandler(db *coredb.DB) http.Handler {
	return NewReadyzHandlerWithChecks(
		func(r *http.Request) error {
			if db == nil || db.SQL() == nil {
				return errors.New("coredb is not initialised")
			}
			ctx, cancel := context.WithTimeout(r.Context(), readyzPingTimeout)
			defer cancel()
			return db.SQL().PingContext(ctx)
		},
		func(r *http.Request) (coredb.StorageStats, error) {
			return coredb.CollectStorageStats(r.Context(), db)
		},
	)
}

// NewReadyzHandlerWithChecks returns an HTTP handler for GET /readyz using injected readiness checks.
func NewReadyzHandlerWithChecks(coredbReady coreDBReadyFunc, storageStats readyzStorageStatsFunc) http.Handler {
	if coredbReady == nil {
		coredbReady = func(*http.Request) error { return nil }
	}
	if storageStats == nil {
		storageStats = func(*http.Request) (coredb.StorageStats, error) {
			return coredb.StorageStats{OK: true}, nil
		}
	}
	return &readyzHandler{coredbReady: coredbReady, storageStats: storageStats}
}

func (h *readyzHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		response.Write(w, response.New(http.StatusMethodNotAllowed, "method not allowed"))
		return
	}

	if err := h.coredbReady(r); err != nil {
		response.Write(w, response.New(http.StatusServiceUnavailable, "not ready",
			response.WithType(readyzNotReadyProblemType),
			response.WithDetail("core database is not ready"),
			response.WithExtension("checks", map[string]string{"coredb": "failed", "storage": "unknown"}),
		))
		return
	}

	storageCtx, cancel := context.WithTimeout(r.Context(), readyzStorageTimeout)
	defer cancel()
	stats, err := h.storageStats(r.WithContext(storageCtx))
	if err != nil {
		response.Write(w, response.New(http.StatusServiceUnavailable, "not ready",
			response.WithType(readyzNotReadyProblemType),
			response.WithDetail("storage health check failed"),
			response.WithExtension("checks", map[string]string{"coredb": "ok", "storage": "failed"}),
		))
		return
	}

	if !stats.OK {
		response.Write(w, response.New(http.StatusServiceUnavailable, "not ready",
			response.WithType(readyzNotReadyProblemType),
			response.WithDetail("storage is not healthy"),
			response.WithExtension("checks", map[string]string{"coredb": "ok", "storage": "failed"}),
		))
		return
	}

	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusNoContent)
}
