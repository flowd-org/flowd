package events

import "strings"

const secretToken = "[secret]"

func SecretToken() string { return secretToken }

// RedactSecrets returns a copy of the args map with secret values replaced by [secret].
func RedactSecrets(values map[string]interface{}, secretNames map[string]struct{}) map[string]interface{} {
	if len(secretNames) == 0 || len(values) == 0 {
		return values
	}
	copy := make(map[string]interface{}, len(values))
	for k, v := range values {
		if _, ok := secretNames[k]; ok {
			copy[k] = secretToken
		} else {
			copy[k] = v
		}
	}
	return copy
}

func NewLineRedactor(secretValues []string) func(string) string {
	if len(secretValues) == 0 {
		return nil
	}
	filtered := make([]string, 0, len(secretValues))
	for _, val := range secretValues {
		if val != "" {
			filtered = append(filtered, val)
		}
	}
	if len(filtered) == 0 {
		return nil
	}
	return func(line string) string {
		for _, secret := range filtered {
			line = strings.ReplaceAll(line, secret, secretToken)
		}
		return line
	}
}

func isSecret(format string, secret bool) bool {
	if secret {
		return true
	}
	return format == "secret"
}
