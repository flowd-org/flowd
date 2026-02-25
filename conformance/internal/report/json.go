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
	const prefix = "Authorization: Bearer "
	const replacement = prefix + "[REDACTED]"

	var result strings.Builder
	result.Grow(len(s))

	start := 0
	for start < len(s) {
		idx := strings.Index(s[start:], prefix)
		if idx == -1 {
			result.WriteString(s[start:])
			break
		}

		result.WriteString(s[start : start+idx])
		result.WriteString(replacement)

		// Find end of token (space, newline, or end of string)
		tokenStart := start + idx + len(prefix)
		end := tokenStart
		for end < len(s) && s[end] != ' ' && s[end] != '\n' && s[end] != '\r' {
			end++
		}

		// Debug output
		_ = end
		if len(s) > 40 && end > 40 {
			_ = end
		}

		// The end position now points to the delimiter (space, newline, or end)
		// We want to keep everything from end onwards
		if end >= len(s) {
			break
		}
		start = end
	}

	return result.String()
}
