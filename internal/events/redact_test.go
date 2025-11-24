package events

import "testing"

func TestRedactSecrets(t *testing.T) {
	vals := map[string]interface{}{"a": "1", "secret": "value"}
	secrets := map[string]struct{}{"secret": {}}
	redacted := RedactSecrets(vals, secrets)
	if redacted["secret"] != secretToken {
		t.Fatalf("expected secret redacted, got %v", redacted["secret"])
	}
	if redacted["a"] != "1" {
		t.Fatalf("expected non secret preserved")
	}
}

func TestNewLineRedactor(t *testing.T) {
	redactor := NewLineRedactor([]string{"token"})
	if redactor == nil {
		t.Fatalf("expected redactor")
	}
	line := redactor("value token here")
	if line != "value [secret] here" {
		t.Fatalf("expected redaction, got %s", line)
	}
	if NewLineRedactor(nil) != nil {
		t.Fatalf("expected nil redactor for empty secrets")
	}
}
