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
// It handles case-insensitive header names, optional spaces around colon,
// and multiple spaces/tabs before the token value.
func redactAuthorizationHeader(s string) string {
	// Split into lines to process each line separately
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		// Find "authorization" (case-insensitive) at the start of the line or after whitespace
		lowerLine := strings.ToLower(line)
		idx := strings.Index(lowerLine, "authorization")
		if idx < 0 {
			continue
		}

		// Check that "authorization" is not part of a larger word
		if idx > 0 {
			prevChar := line[idx-1]
			if (prevChar >= 'a' && prevChar <= 'z') || (prevChar >= 'A' && prevChar <= 'Z') ||
				(prevChar >= '0' && prevChar <= '9') || prevChar == '_' {
				continue
			}
		}

		// Find the colon after "authorization"
		afterAuth := line[idx+len("authorization"):]
		colonIdx := strings.Index(afterAuth, ":")
		if colonIdx < 0 {
			continue
		}

		// Validate whitespace between "authorization" and ":"
		prefixPart := afterAuth[:colonIdx]
		if strings.TrimSpace(prefixPart) != "" {
			continue
		}

		// Find "bearer" after the colon
		afterColon := afterAuth[colonIdx+1:]
		lowerAfterColon := strings.ToLower(afterColon)
		bearerIdx := strings.Index(lowerAfterColon, "bearer")
		if bearerIdx < 0 {
			continue
		}

		// Check that "bearer" is not part of a larger word
		if bearerIdx > 0 {
			prevChar := afterColon[bearerIdx-1]
			if (prevChar >= 'a' && prevChar <= 'z') || (prevChar >= 'A' && prevChar <= 'Z') ||
				(prevChar >= '0' && prevChar <= '9') || prevChar == '_' {
				continue
			}
		}

		// Validate whitespace between ":" and "bearer"
		middlePart := afterColon[:bearerIdx]
		if strings.TrimSpace(middlePart) != "" {
			continue
		}

		// Find the token start (after "bearer" + optional whitespace)
		tokenStart := bearerIdx + len("bearer")
		for tokenStart < len(afterColon) && (afterColon[tokenStart] == ' ' || afterColon[tokenStart] == '\t') {
			tokenStart++
		}

		// Find the end of the token
		tokenEnd := tokenStart
		for tokenEnd < len(afterColon) && afterColon[tokenEnd] != ' ' && afterColon[tokenEnd] != '\n' && afterColon[tokenEnd] != '\r' {
			tokenEnd++
		}

		// Reconstruct the line - replace "Bearer <token>" with "[REDACTED]"
		// The prefix should be "Authorization: [whitespace] Bearer" (without the token)
		// authHeaderPrefix includes everything up to and including "Bearer" + whitespace before token
		// The correct end position is idx + len("authorization") + colonIdx + 1 + tokenStart
		// (colonIdx accounts for the position of ":" in afterAuth, tokenStart is from afterColon)
		authHeaderPrefix := line[idx : idx+len("authorization")+colonIdx+1+tokenStart]
		// Avoid double redaction by checking if token was already redacted
		tokenPart := afterColon[tokenStart:tokenEnd]
		if tokenPart == "[REDACTED]" || strings.Contains(tokenPart, "[REDACTED]") {
			// Token already redacted, nothing to do
			continue
		}
		// Replace the entire "Bearer <token>" portion with just "[REDACTED]"
		lines[i] = line[:idx] + authHeaderPrefix + "[REDACTED]" + afterColon[tokenEnd:]
	}

	return strings.Join(lines, "\n")
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
