package handlers //nolint:testpackage // intentionally in-package to exercise unexported scrub helpers.

import (
	"net/http"
	"testing"

	"github.com/flowd-org/flowd/internal/events"
	"github.com/flowd-org/flowd/internal/server/response"
)

func TestScrubProblemDetail_UsesScrubber(t *testing.T) {
	scrubber := &persistenceScrubber{secretValues: []string{"sekret"}}

	result := scrubProblemDetail("contains sekret", scrubber, nil, nil, nil)
	if result != events.SecretToken() {
		t.Errorf("expected SecretToken(), got %q", result)
	}
}

func TestScrubProblemResponse_ScrubsTitleDetailAndExtensions(t *testing.T) {
	prob := response.New(400, "sekret", response.WithDetail("sekret"), response.WithExtension("x", "sekret"))
	scrubber := &persistenceScrubber{secretValues: []string{"sekret"}}

	out := scrubProblemResponse(&prob, scrubber, nil, nil, nil)

	if out.Title != events.SecretToken() {
		t.Errorf("expected Title redacted to SecretToken(), got %q", out.Title)
	}
	if out.Detail != events.SecretToken() {
		t.Errorf("expected Detail redacted to SecretToken(), got %q", out.Detail)
	}
	if ext, ok := out.Ext["x"]; !ok || ext != events.SecretToken() {
		t.Errorf("expected Ext['x'] redacted to SecretToken(), got %v", out.Ext)
	}
}

func TestScrubProblemDetail_EmptyDetailReturnsEmpty(t *testing.T) {
	result := scrubProblemDetail("", nil, nil, nil, nil)
	if result != "" {
		t.Errorf("expected empty string for empty input, got %q", result)
	}
}

func TestScrubProblemResponse_NilReturnsInternalServerError(t *testing.T) {
	out := scrubProblemResponse(nil, nil, nil, nil, nil)
	if out.Status != http.StatusInternalServerError {
		t.Errorf("expected Status=%d, got %d", http.StatusInternalServerError, out.Status)
	}
	if out.Title != "internal error" {
		t.Errorf("expected Title='internal error', got %q", out.Title)
	}
}

func TestScrubProblemExtensions_EmptyMapReturnsEmpty(t *testing.T) {
	out := scrubProblemExtensions(nil, nil)
	if out != nil {
		t.Errorf("expected nil for nil input, got %v", out)
	}

	out = scrubProblemExtensions(map[string]any{}, nil)
	if len(out) != 0 {
		t.Errorf("expected empty map for empty input, got %v", out)
	}
}
