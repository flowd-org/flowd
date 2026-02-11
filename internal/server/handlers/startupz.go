// SPDX-License-Identifier: AGPL-3.0-or-later

package handlers

import (
	"net/http"

	"github.com/flowd-org/flowd/internal/server/response"
)

const startupIncompleteProblemType = "https://flowd.org/problems/startup-incomplete"

type startupStateFunc func(*http.Request) bool

type startupzHandler struct {
	isStartupComplete startupStateFunc
}

// NewStartupzHandler returns an HTTP handler for GET /startupz.
func NewStartupzHandler() http.Handler {
	return NewStartupzHandlerWithState(func(*http.Request) bool { return true })
}

// NewStartupzHandlerWithState returns an HTTP handler for GET /startupz using an injected startup state check.
func NewStartupzHandlerWithState(isStartupComplete startupStateFunc) http.Handler {
	if isStartupComplete == nil {
		isStartupComplete = func(*http.Request) bool { return true }
	}
	return &startupzHandler{isStartupComplete: isStartupComplete}
}

func (h *startupzHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		response.Write(w, response.New(http.StatusMethodNotAllowed, "method not allowed"))
		return
	}

	if !h.isStartupComplete(r) {
		response.Write(w, response.New(http.StatusServiceUnavailable, "startup incomplete",
			response.WithType(startupIncompleteProblemType),
			response.WithDetail("server startup sequence is not complete"),
		))
		return
	}

	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusNoContent)
}
