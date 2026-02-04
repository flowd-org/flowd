package handlers

import (
	"net/http"
	"strings"

	"github.com/flowd-org/flowd/internal/coredb"
	"github.com/flowd-org/flowd/internal/server/response"
	"github.com/flowd-org/flowd/internal/server/runstore"
)

// RunGetConfig configures the run GET handler.
type RunGetConfig struct {
	Store *runstore.Store
	DB    *coredb.DB
}

// NewRunGetHandler returns an HTTP handler for GET /runs/{id}.
func NewRunGetHandler(cfg RunGetConfig) http.Handler {
	store := cfg.Store
	if store == nil {
		store = runstore.New()
	}
	var dbRuns *coredb.RunStore
	if cfg.DB != nil {
		dbRuns = coredb.NewRunStore(cfg.DB)
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			response.Write(w, response.New(http.StatusMethodNotAllowed, "method not allowed"))
			return
		}

		id := strings.TrimPrefix(r.URL.Path, "/runs/")
		if id == "" || strings.Contains(id, "/") {
			response.Write(w, response.New(http.StatusNotFound, "run not found"))
			return
		}

		if dbRuns != nil {
			record, ok, err := dbRuns.Get(r.Context(), id)
			if err != nil {
				response.Write(w, response.New(http.StatusInternalServerError, "run lookup failed", response.WithDetail(err.Error())))
				return
			}
			if ok {
				writeRunPayload(w, payloadFromRecord(record), http.StatusOK)
				return
			}
		}
		run, ok := store.Get(id)
		if !ok {
			response.Write(w, response.New(http.StatusNotFound, "run not found"))
			return
		}
		writeRunPayload(w, payloadFromStore(run), http.StatusOK)
	})
}
