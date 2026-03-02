package report

import (
	"strings"
	"testing"
)

func TestRedactTokens_Basic(t *testing.T) {
	// Basic test - the function should handle Authorization headers
	input := "Authorization: Bearer abc123"
	got := redactTokens(input)
	if got != "Authorization: Bearer [REDACTED]" {
		t.Errorf("redactTokens() = %q, want %q", got, "Authorization: Bearer [REDACTED]")
	}
}

func TestRedactTokens_MultiHeaderJSONValid(t *testing.T) {
	token := "bearer-token-xyz"
	// redactTokens only handles "Authorization: Bearer " patterns, not JSON
	// This test verifies it preserves JSON structure when processing Authorization headers
	input := `{
  "Authorization": "Bearer ` + token + `",
  "Authorization-Alt": "Bearer ` + token + `"
}`

	got := redactTokens(input)

	// The function should redact Authorization headers (Bearer token pattern)
	expected := `{
  "Authorization": "Bearer [REDACTED]",
  "Authorization-Alt": "Bearer [REDACTED]"
}`
	if got != expected {
		t.Errorf("redactTokens() = %q, want %q", got, expected)
	}

	// JSON should remain structurally valid (basic check: balanced braces/brackets)
	openBraces := strings.Count(got, "{")
	closeBraces := strings.Count(got, "}")
	openBrackets := strings.Count(got, "[")
	closeBrackets := strings.Count(got, "]")

	if openBraces != closeBraces || openBrackets != closeBrackets {
		t.Errorf("redactTokens() produced invalid JSON structure: %s", got)
	}

	// Token should be redacted
	if strings.Contains(got, token) {
		t.Errorf("redactTokens() did not redact token: %s", got)
	}
}
