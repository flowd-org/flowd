// SPDX-License-Identifier: AGPL-3.0-or-later

package handlers

import (
	"encoding/json"
	"net/http"
	"sort"

	"github.com/flowd-org/flowd/internal/server/buildinfo"
	"github.com/flowd-org/flowd/internal/server/response"
)

type capabilitiesCore struct {
	Version     string `json:"version"`
	SpecVersion string `json:"spec_version"`
	AppID       string `json:"app_id"`
}

type capabilitiesExtension struct {
	Name     string `json:"name"`
	Version  string `json:"version"`
	Compiled bool   `json:"compiled"`
	Enabled  bool   `json:"enabled"`
}

type capabilitiesResponse struct {
	Core       capabilitiesCore        `json:"core"`
	Extensions []capabilitiesExtension `json:"extensions"`
}

type capabilitiesHandler struct {
	version           string
	compiledExtension map[string]bool
}

// NewCapabilitiesHandler returns an HTTP handler for GET /capabilities.
func NewCapabilitiesHandler(extensionEnabled map[string]bool) http.Handler {
	compiled := map[string]bool{
		"export": extensionEnabled["export"],
	}
	return &capabilitiesHandler{
		version:           buildinfo.Version(),
		compiledExtension: compiled,
	}
}

func (h *capabilitiesHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		response.Write(w, response.New(http.StatusMethodNotAllowed, "method not allowed"))
		return
	}

	names := make([]string, 0, len(h.compiledExtension))
	for name := range h.compiledExtension {
		names = append(names, name)
	}
	sort.Strings(names)

	extensions := make([]capabilitiesExtension, 0, len(names))
	for _, name := range names {
		extensions = append(extensions, capabilitiesExtension{
			Name:     name,
			Version:  h.version,
			Compiled: true,
			Enabled:  h.compiledExtension[name],
		})
	}

	payload := capabilitiesResponse{
		Core: capabilitiesCore{
			Version:     h.version,
			SpecVersion: buildinfo.CoreSpecVersion,
			AppID:       buildinfo.CoreAppID,
		},
		Extensions: extensions,
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(payload)
}
