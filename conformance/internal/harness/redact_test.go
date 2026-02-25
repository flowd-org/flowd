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
