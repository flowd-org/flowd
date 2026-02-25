package report

import "testing"

func TestRedactTokens_Basic(t *testing.T) {
	// Basic test - the function should handle Authorization headers
	input := "Authorization: Bearer abc123"
	got := redactTokens(input)
	if got != "Authorization: Bearer [REDACTED]" {
		t.Errorf("redactTokens() = %q, want %q", got, "Authorization: Bearer [REDACTED]")
	}
}
