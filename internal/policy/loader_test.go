// SPDX-License-Identifier: AGPL-3.0-or-later
package policy_test

import (
	"os"
	"path/filepath"
	"testing"

	policy "github.com/flowd-org/flowd/internal/policy"

	yaml "gopkg.in/yaml.v3"
)

func TestLoadFile_EmptyPath(t *testing.T) {
	_, err := policy.LoadFile("")
	if err == nil {
		t.Fatal("expected error for empty path")
	}
	if err.Error() != "missing policy file path" {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestLoadFile_UnreadablePath(t *testing.T) {
	_, err := policy.LoadFile("/nonexistent/path/policy.yaml")
	if err == nil {
		t.Fatal("expected error for unreadable path")
	}
	if !contains(err.Error(), "read policy file") {
		t.Errorf("expected 'read policy file' in error: %v", err)
	}
}

func TestLoadFile_InvalidYAML(t *testing.T) {
	tmpFile := filepath.Join(t.TempDir(), "invalid.yaml")
	if err := os.WriteFile(tmpFile, []byte("invalid: yaml: ["), 0o600); err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}

	_, err := policy.LoadFile(tmpFile)
	if err == nil {
		t.Fatal("expected error for invalid YAML")
	}
	if !contains(err.Error(), "parse policy file") {
		t.Errorf("expected 'parse policy file' in error: %v", err)
	}
}

func TestLoadFile_InvalidVerifySignatures(t *testing.T) {
	tmpDir := t.TempDir()
	bundle := &policy.Bundle{
		VerifySignatures: stringPtr("invalid_value"),
	}
	data, err := yaml.Marshal(bundle)
	if err != nil {
		t.Fatalf("failed to marshal bundle: %v", err)
	}
	tmpFile := filepath.Join(tmpDir, "policy.yaml")
	if err := os.WriteFile(tmpFile, data, 0o600); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}

	_, err = policy.LoadFile(tmpFile)
	if err == nil {
		t.Fatal("expected error for invalid verify_signatures")
	}
	if !contains(err.Error(), "invalid verify_signatures") {
		t.Errorf("expected 'invalid verify_signatures' in error: %v", err)
	}
}

func TestLoadFromEnvOrDefault_WithEnvSet(t *testing.T) {
	tmpDir := t.TempDir()
	bundle := &policy.Bundle{}
	data, err := yaml.Marshal(bundle)
	if err != nil {
		t.Fatalf("failed to marshal bundle: %v", err)
	}
	tmpFile := filepath.Join(tmpDir, "policy.yaml")
	if err := os.WriteFile(tmpFile, data, 0o600); err != nil {
		t.Fatalf("failed to write temp file: %v", err)
	}

	oldVal, hadVal := os.LookupEnv("FLWD_POLICY_FILE")
	t.Cleanup(func() {
		var err error
		if hadVal {
			err = os.Setenv("FLWD_POLICY_FILE", oldVal)
		} else {
			err = os.Unsetenv("FLWD_POLICY_FILE")
		}
		if err != nil {
			t.Fatalf("restore FLWD_POLICY_FILE: %v", err)
		}
	})
	if err := os.Setenv("FLWD_POLICY_FILE", tmpFile); err != nil {
		t.Fatalf("set FLWD_POLICY_FILE: %v", err)
	}

	b, path, err := policy.LoadFromEnvOrDefault()
	if err != nil {
		t.Fatalf("LoadFromEnvOrDefault error: %v", err)
	}
	if b == nil {
		t.Fatal("expected bundle")
	}
	if path != tmpFile {
		t.Errorf("expected path %s, got %s", tmpFile, path)
	}
}

func TestLoadFromEnvOrDefault_WithEnvUnset_NoDefault(t *testing.T) {
	oldVal, hadVal := os.LookupEnv("FLWD_POLICY_FILE")
	t.Cleanup(func() {
		var err error
		if hadVal {
			err = os.Setenv("FLWD_POLICY_FILE", oldVal)
		} else {
			err = os.Unsetenv("FLWD_POLICY_FILE")
		}
		if err != nil {
			t.Fatalf("restore FLWD_POLICY_FILE: %v", err)
		}
	})
	if err := os.Unsetenv("FLWD_POLICY_FILE"); err != nil {
		t.Fatalf("unset FLWD_POLICY_FILE: %v", err)
	}

	b, path, err := policy.LoadFromEnvOrDefault()
	if err != nil {
		t.Fatalf("LoadFromEnvOrDefault error: %v", err)
	}
	if b != nil || path != "" {
		t.Errorf("expected (nil, \"\", nil), got (%v, %q)", b, path)
	}
}

func TestLoadFromEnvOrDefault_WithDefaultFile(t *testing.T) {
	tmpDir := t.TempDir()
	bundle := &policy.Bundle{}
	data, _ := yaml.Marshal(bundle)

	oldCwd, _ := os.Getwd()
	defer func() {
		if err := os.Chdir(oldCwd); err != nil {
			t.Fatalf("restore working directory: %v", err)
		}
	}()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("change working directory: %v", err)
	}

	defaultFile := filepath.Join(tmpDir, "flwd.policy.yaml")
	os.WriteFile(defaultFile, data, 0o600)

	b, path, err := policy.LoadFromEnvOrDefault()
	if err != nil {
		t.Fatalf("LoadFromEnvOrDefault error: %v", err)
	}
	if b == nil {
		t.Fatal("expected bundle")
	}
	if !contains(path, "flwd.policy.yaml") {
		t.Errorf("expected default file in path: %s", path)
	}
}

func TestNormalizeAllowedRegistries(t *testing.T) {
	tmpDir := t.TempDir()
	bundle := &policy.Bundle{
		AllowedRegistries: []string{"EXAMPLE.COM", "GITHUB.COM"},
	}
	data, _ := yaml.Marshal(bundle)
	tmpFile := filepath.Join(tmpDir, "policy.yaml")
	os.WriteFile(tmpFile, data, 0o600)

	b, err := policy.LoadFile(tmpFile)
	if err != nil {
		t.Fatalf("LoadFile error: %v", err)
	}

	expected := []string{"example.com", "github.com"}
	for i, reg := range b.AllowedRegistries {
		if reg != expected[i] {
			t.Errorf("expected registry %s, got %s", expected[i], reg)
		}
	}
}

func stringPtr(s string) *string {
	return &s
}

func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
