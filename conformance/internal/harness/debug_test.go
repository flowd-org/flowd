package harness

import (
	"strings"
	"testing"
)

func TestRedactSecrets_AuthorizationHeaderWithExtraWhitespace(t *testing.T) {
	token := "secret-token-xyz"
	input := "Authorization:    Bearer " + token

	result := RedactSecrets(input, token)

	// The actual behavior preserves "Bearer" when redacting the token
	expected := "Authorization:    Bearer [REDACTED]"
	if result != expected {
		t.Errorf("RedactSecrets() = %q, want %q", result, expected)
	}

	if strings.Contains(result, token) {
		t.Errorf("RedactSecrets() did not redact token: %s", result)
	}
}

func TestRedactAuthorizationHeader_Direct(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		expect string
	}{
		{
			name:   "direct test with [REDACTED]",
			input:  "Authorization:    Bearer [REDACTED]",
			expect: "Authorization:    Bearer [REDACTED]",
		},
		{
			name:   "lowercase authorization and bearer",
			input:  "authorization: bearer token123",
			expect: "authorization: bearer [REDACTED]",
		},
		{
			name:   "mixed case authorization and bearer",
			input:  "Authorization: Bearer token123",
			expect: "Authorization: Bearer [REDACTED]",
		},
		{
			name:   "uppercase authorization and bearer",
			input:  "AUTHORIZATION: BEARER token123",
			expect: "AUTHORIZATION: BEARER [REDACTED]",
		},
		{
			name:   "authorization with space before colon",
			input:  "Authorization : Bearer token123",
			expect: "Authorization : Bearer [REDACTED]",
		},
		{
			name:   "authorization with tab before colon",
			input:  "Authorization\t: Bearer token123",
			expect: "Authorization\t: Bearer [REDACTED]",
		},
		{
			name:   "authorization with multiple spaces before token",
			input:  "Authorization:    Bearer token123",
			expect: "Authorization:    Bearer [REDACTED]",
		},
		{
			name:   "authorization with tab before token",
			input:  "Authorization:\t\tBearer token123",
			expect: "Authorization:\t\tBearer [REDACTED]",
		},
		{
			name:   "authorization with mixed whitespace before token",
			input:  "Authorization: \t Bearer token123",
			expect: "Authorization: \t Bearer [REDACTED]",
		},
		{
			name:   "no authorization header",
			input:  "some other text",
			expect: "some other text",
		},
		{
			name:   "empty input",
			input:  "",
			expect: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := redactAuthorizationHeader(tt.input)
			if result != tt.expect {
				t.Errorf("redactAuthorizationHeader() = %q, want %q", result, tt.expect)
			}
		})
	}
}
