package harness

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

// RedactSecrets replaces all occurrences of the provided secrets in the input string.
// It also redacts Authorization: Bearer <token> patterns.
func RedactSecrets(s string, secrets ...string) string {
	result := s
	for _, secret := range secrets {
		if secret != "" {
			result = strings.ReplaceAll(result, secret, "[REDACTED]")
		}
	}
	// Also redact Authorization: Bearer <token> patterns
	result = redactAuthorizationHeader(result)
	return result
}

// redactAuthorizationHeader removes Bearer tokens from Authorization headers.
func redactAuthorizationHeader(s string) string {
	// Replace "Authorization: Bearer <token>" pattern
	prefix := "Authorization: Bearer "
	idx := strings.Index(s, prefix)
	if idx == -1 {
		return s
	}

	end := idx + len(prefix)
	// Find end of token (space, newline, or end of string)
	for end < len(s) && s[end] != ' ' && s[end] != '\n' && s[end] != '\r' {
		end++
	}

	return s[:idx] + prefix + "[REDACTED]" + s[end:]
}

// RedactTokenInLine is a helper that redacts a specific token from a line of text.
func RedactTokenInLine(line, token string) string {
	if token == "" {
		return line
	}
	return strings.ReplaceAll(line, token, "[REDACTED]")
}

// computeSHA256 computes the SHA256 hash of the canonical JSON body.
func ComputeSHA256(body []byte) string {
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}
