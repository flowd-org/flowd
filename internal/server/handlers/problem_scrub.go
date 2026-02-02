// SPDX-License-Identifier: AGPL-3.0-or-later
package handlers

import (
	"net/http"

	"github.com/flowd-org/flowd/internal/engine"
	"github.com/flowd-org/flowd/internal/server/response"
	"github.com/flowd-org/flowd/internal/types"
)

func scrubProblemDetail(detail string, scrubber *persistenceScrubber, binding *engine.Binding, spec *types.ArgSpec, args map[string]any) string {
	if detail == "" {
		return detail
	}
	if scrubber == nil {
		scrubber = newProblemScrubber(spec, args, binding, nil)
	}
	return scrubString(detail, scrubber)
}

func scrubProblemResponse(prob *response.Problem, scrubber *persistenceScrubber, binding *engine.Binding, spec *types.ArgSpec, args map[string]any) response.Problem {
	if prob == nil {
		return response.New(http.StatusInternalServerError, "internal error")
	}
	if scrubber == nil {
		scrubber = newProblemScrubber(spec, args, binding, nil)
	}
	copy := *prob
	if copy.Title != "" {
		copy.Title = scrubString(copy.Title, scrubber)
	}
	if copy.Detail != "" {
		copy.Detail = scrubString(copy.Detail, scrubber)
	}
	if len(copy.Ext) > 0 {
		copy.Ext = scrubProblemExtensions(copy.Ext, scrubber)
	}
	return copy
}

func scrubProblemExtensions(ext map[string]any, scrubber *persistenceScrubber) map[string]any {
	if len(ext) == 0 {
		return ext
	}
	out := make(map[string]any, len(ext))
	for key, value := range ext {
		out[key] = scrubAny(value, scrubber)
	}
	return out
}
