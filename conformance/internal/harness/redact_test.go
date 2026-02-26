package harness

import (
	"strings"
	"testing"
)

func TestRedactSecrets_BearerToken(t *testing.T) {
	token := "secret-token-123"
	input := "Authorization: Bearer " + token

	result := RedactSecrets(input, token)

	if strings.Contains(result, token) {
		t.Errorf("RedactSecrets() did not redact token: %s", result)
	}

	if !strings.Contains(result, "Authorization: Bearer [REDACTED]") {
		t.Errorf("RedactSecrets() did not produce expected output: %s", result)
	}
}

func TestRedactSecrets_RawToken(t *testing.T) {
	token := "secret-token-456"
	input := "token=" + token + "&other=data"

	result := RedactSecrets(input, token)

	if strings.Contains(result, token) {
		t.Errorf("RedactSecrets() did not redact raw token: %s", result)
	}

	if !strings.Contains(result, "[REDACTED]") {
		t.Errorf("RedactSecrets() did not produce [REDACTED]: %s", result)
	}
}

func TestRedactSecrets_MultipleTokens(t *testing.T) {
	token1 := "token1"
	token2 := "token2"
	input := "auth1: Bearer " + token1 + ", auth2: Bearer " + token2

	result := RedactSecrets(input, token1, token2)

	if strings.Contains(result, token1) || strings.Contains(result, token2) {
		t.Errorf("RedactSecrets() did not redact all tokens: %s", result)
	}

	// The function replaces raw tokens first, then redacts Authorization headers.
	// When a token appears in both places, it gets replaced once as raw and once in header.
	// Since we have 2 tokens in headers and 2 raw occurrences, we get 2 redactions total
	// (one per token - the header redaction happens after raw replacement).
	count := strings.Count(result, "[REDACTED]")
	if count != 2 { // 2 tokens redacted (one per unique token)
		t.Errorf("RedactSecrets() expected 2 [REDACTED], got %d", count)
	}
}

func TestRedactSecrets_EmptySecret(t *testing.T) {
	token := ""
	input := "some text without secrets"

	result := RedactSecrets(input, token)

	if result != input {
		t.Errorf("RedactSecrets() modified input when secret is empty: %s", result)
	}
}

func TestRedactTokenInLine(t *testing.T) {
	token := "my-secret-token"
	line := "Using token: my-secret-token for auth"

	result := RedactTokenInLine(line, token)

	if strings.Contains(result, token) {
		t.Errorf("RedactTokenInLine() did not redact token: %s", result)
	}

	expected := "Using token: [REDACTED] for auth"
	if result != expected {
		t.Errorf("RedactTokenInLine() = %q, want %q", result, expected)
	}
}

func TestRedactTokenInLine_EmptyToken(t *testing.T) {
	token := ""
	line := "some text without token"

	result := RedactTokenInLine(line, token)

	if result != line {
		t.Errorf("RedactTokenInLine() modified line when token is empty: %s", result)
	}
}

func TestRedactSecrets_MultipleAuthorizationHeaders(t *testing.T) {
	token := "secret-token-789"
	input := "First: Authorization: Bearer " + token + "\nSecond: Authorization: Bearer " + token

	result := RedactSecrets(input, token)

	if strings.Contains(result, token) {
		t.Errorf("RedactSecrets() did not redact token: %s", result)
	}

	if !strings.Contains(result, "First: Authorization: Bearer [REDACTED]") {
		t.Errorf("RedactSecrets() did not redact first header: %s", result)
	}

	if !strings.Contains(result, "Second: Authorization: Bearer [REDACTED]") {
		t.Errorf("RedactSecrets() did not redact second header: %s", result)
	}
}

func TestRedactSecrets_PreservesPrefixSuffix(t *testing.T) {
	token := "my-secret"
	input := "prefix: " + token + " suffix"

	result := RedactSecrets(input, token)

	// Token should be redacted, but prefix/suffix should remain
	expected := "prefix: [REDACTED] suffix"
	if result != expected {
		t.Errorf("RedactSecrets() = %q, want %q", result, expected)
	}

	// Ensure surrounding text is unchanged
	if !strings.HasPrefix(result, "prefix: ") || !strings.HasSuffix(result, " suffix") {
		t.Errorf("RedactSecrets() did not preserve prefix/suffix: %s", result)
	}
}

func TestRedactTokenInLine_MultipleOccurrences(t *testing.T) {
	token := "token-abc"
	line := "Token: " + token + ", another: " + token + ", end"

	result := RedactTokenInLine(line, token)

	// All occurrences should be redacted
	if strings.Contains(result, token) {
		t.Errorf("RedactTokenInLine() did not redact all occurrences: %s", result)
	}

	count := strings.Count(result, "[REDACTED]")
	if count != 2 {
		t.Errorf("RedactTokenInLine() expected 2 [REDACTED], got %d", count)
	}
}

func TestRedactSecrets_AuthorizationHeaderVariants(t *testing.T) {
	token := "secret-token-xyz"

	tests := []struct {
		name   string
		input  string
		expect string
	}{
		{
			name:   "lowercase authorization and bearer",
			input:  "authorization: bearer " + token,
			expect: "authorization: bearer [REDACTED]",
		},
		{
			name:   "mixed case authorization and bearer",
			input:  "Authorization: Bearer " + token,
			expect: "Authorization: Bearer [REDACTED]",
		},
		{
			name:   "uppercase authorization and bearer",
			input:  "AUTHORIZATION: BEARER " + token,
			expect: "AUTHORIZATION: BEARER [REDACTED]",
		},
		{
			name:   "authorization with space before colon",
			input:  "Authorization : Bearer " + token,
			expect: "Authorization : Bearer [REDACTED]",
		},
		{
			name:   "authorization with tab before colon",
			input:  "Authorization\t: Bearer " + token,
			expect: "Authorization\t: Bearer [REDACTED]",
		},
		{
			name:   "authorization with multiple spaces before token",
			input:  "Authorization:    Bearer " + token,
			expect: "Authorization:    Bearer [REDACTED]",
		},
		{
			name:   "authorization with tab before token",
			input:  "Authorization:\t\tBearer " + token,
			expect: "Authorization:\t\tBearer [REDACTED]",
		},
		{
			name:   "authorization with mixed whitespace before token",
			input:  "Authorization: \t Bearer " + token,
			expect: "Authorization: \t Bearer [REDACTED]",
		},
		{
			name:   "multiple headers with variants",
			input:  "First: authorization: bearer " + token + "\nSecond: Authorization : Bearer " + token,
			expect: "First: authorization: bearer [REDACTED]\nSecond: Authorization : Bearer [REDACTED]",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := RedactSecrets(tt.input, token)

			if strings.Contains(result, token) {
				t.Errorf("RedactSecrets() did not redact token: %s", result)
			}

			if !strings.Contains(result, "[REDACTED]") {
				t.Errorf("RedactSecrets() did not produce [REDACTED]: %s", result)
			}

			// Check the redacted header matches expected format (without token)
			if !strings.HasPrefix(result, tt.expect) && !strings.Contains(result, "\n"+tt.expect) {
				// Check if result contains the expected redacted header
				found := false
				lines := strings.Split(result, "\n")
				for _, line := range lines {
					if strings.TrimSpace(line) == strings.TrimSpace(tt.expect) {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("RedactSecrets() expected header %q, got %q", tt.expect, result)
				}
			}
		})
	}
}
