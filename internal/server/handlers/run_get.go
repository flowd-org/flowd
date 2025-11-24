package handlers

import (
	"net/http"
	"strings"

	"github.com/flowd-org/flowd/internal/server/response"
	"github.com/flowd-org/flowd/internal/server/runstore"
)

// NewRunGetHandler returns an HTTP handler for GET /runs/{id}.
func NewRunGetHandler(store *runstore.Store) http.Handler {
	if store == nil {
		store = runstore.New()
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

		run, ok := store.Get(id)
		if !ok {
			response.Write(w, response.New(http.StatusNotFound, "run not found"))
			return
		}

		writeRunPayload(w, payloadFromStore(run), http.StatusOK)
	})
}
