// SPDX-License-Identifier: AGPL-3.0-or-later

package authz

import (
	"net/http"
	"strings"
)

const (
	ScopeJobsRead     = "jobs:read"
	ScopeRunsRead     = "runs:read"
	ScopeRunsWrite    = "runs:write"
	ScopeEventsRead   = "events:read"
	ScopeSourcesRead  = "sources:read"
	ScopeSourcesWrite = "sources:write"
	ScopeRuleYRead    = "ruley:read"
	ScopeRuleYWrite   = "ruley:write"
)

// RequiredScopes returns the scope set required to access the given method/path.
func RequiredScopes(method, path string) []string {
	switch method {
	case http.MethodGet:
		switch {
		case path == "/jobs":
			return []string{ScopeJobsRead}
		case path == "/runs":
			return []string{ScopeRunsRead}
		case strings.HasPrefix(path, "/runs/") && strings.HasSuffix(path, "/events"):
			return []string{ScopeRunsRead, ScopeEventsRead}
		case strings.HasPrefix(path, "/runs/") && strings.HasSuffix(path, "/events.ndjson"):
			return []string{ScopeRunsRead, ScopeEventsRead}
		case strings.HasPrefix(path, "/runs/"):
			return []string{ScopeRunsRead}
		case path == "/sources":
			return []string{ScopeSourcesRead}
		case strings.HasPrefix(path, "/sources/"):
			return []string{ScopeSourcesRead}
		case path == "/events":
			return []string{ScopeEventsRead}
		case strings.HasPrefix(path, "/kv/"):
			return []string{ScopeRuleYRead}
		case path == "/health/storage":
			return []string{ScopeJobsRead}
		}
	case http.MethodPost:
		switch {
		case path == "/plans":
			return []string{ScopeJobsRead}
		case path == "/runs":
			return []string{ScopeRunsWrite}
		case path == "/sources":
			return []string{ScopeSourcesWrite}
		case strings.HasPrefix(path, "/kv/"):
			return []string{ScopeRuleYWrite}
		}
	case http.MethodDelete:
		switch {
		case strings.HasPrefix(path, "/sources/"):
			return []string{ScopeSourcesWrite}
		case strings.HasPrefix(path, "/kv/"):
			return []string{ScopeRuleYWrite}
		}
	case http.MethodPut:
		if strings.HasPrefix(path, "/kv/") {
			return []string{ScopeRuleYWrite}
		}
	}
	return nil
}
