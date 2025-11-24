package authz

import "testing"

func TestRequiredScopes(t *testing.T) {
	tests := []struct {
		method string
		path   string
		want   []string
	}{
		{method: "GET", path: "/jobs", want: []string{ScopeJobsRead}},
		{method: "POST", path: "/plans", want: []string{ScopeJobsRead}},
		{method: "POST", path: "/runs", want: []string{ScopeRunsWrite}},
		{method: "GET", path: "/runs", want: []string{ScopeRunsRead}},
		{method: "GET", path: "/runs/run-123", want: []string{ScopeRunsRead}},
		{method: "GET", path: "/runs/run-123/events", want: []string{ScopeRunsRead, ScopeEventsRead}},
		{method: "GET", path: "/runs/run-123/events.ndjson", want: []string{ScopeRunsRead, ScopeEventsRead}},
		{method: "GET", path: "/sources", want: []string{ScopeSourcesRead}},
		{method: "GET", path: "/sources/main", want: []string{ScopeSourcesRead}},
		{method: "POST", path: "/sources", want: []string{ScopeSourcesWrite}},
		{method: "DELETE", path: "/sources/main", want: []string{ScopeSourcesWrite}},
		{method: "GET", path: "/events", want: []string{ScopeEventsRead}},
		{method: "GET", path: "/health/storage", want: []string{ScopeJobsRead}},
	}

	for _, tc := range tests {
		got := RequiredScopes(tc.method, tc.path)
		if len(got) != len(tc.want) {
			t.Fatalf("RequiredScopes(%s, %s) len = %d, want %d", tc.method, tc.path, len(got), len(tc.want))
		}
		for i, scope := range got {
			if scope != tc.want[i] {
				t.Fatalf("RequiredScopes(%s, %s)[%d] = %s, want %s", tc.method, tc.path, i, scope, tc.want[i])
			}
		}
	}
}
