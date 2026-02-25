package report

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// WriteJSON writes the report to a JSON file with stable indentation.
func WriteJSON(path string, r Report) error {
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal report: %w", err)
	}

	// Redact any tokens in the JSON (defensive)
	redacted := redactTokens(string(data))

	if err := os.WriteFile(path, []byte(redacted), 0644); err != nil {
		return fmt.Errorf("failed to write JSON file: %w", err)
	}

	return nil
}

// redactTokens replaces any Bearer token patterns in the JSON string.
func redactTokens(s string) string {
	// This is a defensive redaction - in practice, tokens should not be in the report
	// but we redact any Authorization header patterns just in case
	// Handle both "Authorization: Bearer <token>" (header format) and JSON "key": "Bearer <token>" formats
	const headerPrefix = "Authorization: Bearer "
	const headerReplacement = headerPrefix + "[REDACTED]"
	const jsonPrefix = `": "Bearer `
	const jsonReplacement = `": "Bearer [REDACTED]`

	var result strings.Builder
	result.Grow(len(s))

	i := 0
	for i < len(s) {
		// Check for header format at current position
		if i+len(headerPrefix) <= len(s) && s[i:i+len(headerPrefix)] == headerPrefix {
			result.WriteString(headerReplacement)
			i += len(headerPrefix)
			// Skip token until space, newline, or carriage return
			for i < len(s) && s[i] != ' ' && s[i] != '\n' && s[i] != '\r' {
				i++
			}
			continue
		}

		// Check for JSON format at current position (look for ": "Bearer <token>")
		if i+len(jsonPrefix) <= len(s) && s[i:i+len(jsonPrefix)] == jsonPrefix {
			result.WriteString(jsonReplacement)
			i += len(jsonPrefix)
			// Skip token until closing quote
			for i < len(s) && s[i] != '"' {
				i++
			}
			continue
		}

		// No match, copy character and advance
		result.WriteByte(s[i])
		i++
	}

	return result.String()
}
