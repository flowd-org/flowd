package harness

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
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

// CanonicalJSON returns the canonical JSON bytes for a value.
// It marshals the value, unmarshals into a generic structure to normalize
// key ordering and whitespace, then marshals again to produce deterministic output.
func CanonicalJSON(v any) ([]byte, error) {
	// First marshal to get raw JSON
	raw, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	// Unmarshal into generic structure to normalize
	var normalized any
	if err := json.Unmarshal(raw, &normalized); err != nil {
		return nil, err
	}
	// Marshal again with stable formatting
	return json.Marshal(normalized)
}

// ComputeSHA256 computes the SHA256 hash of the canonical JSON body.
func ComputeSHA256(body []byte) string {
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}
