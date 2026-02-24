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
	result := s
	// Replace "Authorization: Bearer <token>" patterns
	prefix := "Authorization: Bearer "
	idx := strings.Index(result, prefix)
	if idx != -1 {
		end := idx + len(prefix)
		for end < len(result) && result[end] != '"' && result[end] != ' ' && result[end] != '\n' && result[end] != '\r' {
			end++
		}
		result = result[:idx] + prefix + "\"[REDACTED]\"" + result[end:]
	}
	return result
}
