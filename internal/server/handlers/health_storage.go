// SPDX-License-Identifier: AGPL-3.0-or-later

package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/flowd-org/flowd/internal/coredb"
	"github.com/flowd-org/flowd/internal/server/response"
)

type storageStatsFunc func(r *http.Request) (coredb.StorageStats, error)

type storageHealthHandler struct {
	stats storageStatsFunc
}

// NewStorageHealthHandler returns an HTTP handler for GET /health/storage.
func NewStorageHealthHandler(db *coredb.DB) http.Handler {
	return &storageHealthHandler{
		stats: func(r *http.Request) (coredb.StorageStats, error) {
			return coredb.CollectStorageStats(r.Context(), db)
		},
	}
}

func (h *storageHealthHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		response.Write(w, response.New(http.StatusMethodNotAllowed, "method not allowed"))
		return
	}

	stats, err := h.stats(r)
	if err != nil {
		response.Write(w, response.New(http.StatusServiceUnavailable, "storage degraded",
			response.WithType("https://flowd.dev/problems/storage-degraded"),
			response.WithDetail(err.Error()),
		))
		return
	}

	if !stats.OK {
		response.Write(w, response.New(http.StatusServiceUnavailable, "storage degraded",
			response.WithType("https://flowd.dev/problems/storage-degraded"),
			response.WithExtension("driver", stats.Driver),
			response.WithExtension("bytes_used", stats.BytesUsed),
			response.WithExtension("max_bytes", stats.MaxBytes),
			response.WithExtension("eviction_active", stats.EvictionActive),
			response.WithExtension("schema_version", stats.SchemaVersion),
		))
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(stats)
}
