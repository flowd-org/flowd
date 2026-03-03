package handlers

import (
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
