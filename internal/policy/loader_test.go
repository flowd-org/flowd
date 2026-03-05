// SPDX-License-Identifier: AGPL-3.0-or-later
package policy

import (
	"os"
	"path/filepath"
	"testing"

	yaml "gopkg.in/yaml.v3"
)

func TestLoadFile_EmptyPath(t *testing.T) {
	_, err := LoadFile("")
	if err == nil {
		t.Fatal("expected error for empty path")
	}
	if err.Error() != "missing policy file path" {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestLoadFile_UnreadablePath(t *testing.T) {
	_, err := LoadFile("/nonexistent/path/policy.yaml")
	if err == nil {
		t.Fatal("expected error for unreadable path")
	}
	if !contains(err.Error(), "read policy file") {
		t.Errorf("expected 'read policy file' in error: %v", err)
	}
}

func TestLoadFile_InvalidYAML(t *testing.T) {
	tmpFile := filepath.Join(t.TempDir(), "invalid.yaml")
	os.WriteFile(tmpFile, []byte("invalid: yaml: ["), 0o600)

	_, err := LoadFile(tmpFile)
	if err == nil {
		t.Fatal("expected error for invalid YAML")
	}
	if !contains(err.Error(), "parse policy file") {
		t.Errorf("expected 'parse policy file' in error: %v", err)
	}
}

func TestLoadFile_InvalidVerifySignatures(t *testing.T) {
	tmpDir := t.TempDir()
	bundle := &Bundle{
		VerifySignatures: stringPtr("invalid_value"),
	}
	data, _ := yaml.Marshal(bundle)
	tmpFile := filepath.Join(tmpDir, "policy.yaml")
	os.WriteFile(tmpFile, data, 0o600)

	_, err := LoadFile(tmpFile)
	if err == nil {
		t.Fatal("expected error for invalid verify_signatures")
	}
	if !contains(err.Error(), "invalid verify_signatures") {
		t.Errorf("expected 'invalid verify_signatures' in error: %v", err)
	}
}

func TestLoadFromEnvOrDefault_WithEnvSet(t *testing.T) {
	tmpDir := t.TempDir()
	bundle := &Bundle{}
	data, _ := yaml.Marshal(bundle)
	tmpFile := filepath.Join(tmpDir, "policy.yaml")
	os.WriteFile(tmpFile, data, 0o600)

	oldVal := os.Getenv("FLWD_POLICY_FILE")
	defer os.Setenv("FLWD_POLICY_FILE", oldVal)
	os.Setenv("FLWD_POLICY_FILE", tmpFile)

	b, path, err := LoadFromEnvOrDefault()
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
	oldVal := os.Getenv("FLWD_POLICY_FILE")
	defer os.Setenv("FLWD_POLICY_FILE", oldVal)
	os.Unsetenv("FLWD_POLICY_FILE")

	b, path, err := LoadFromEnvOrDefault()
	if err != nil {
		t.Fatalf("LoadFromEnvOrDefault error: %v", err)
	}
	if b != nil || path != "" {
		t.Errorf("expected (nil, \"\", nil), got (%v, %q)", b, path)
	}
}

func TestLoadFromEnvOrDefault_WithDefaultFile(t *testing.T) {
	tmpDir := t.TempDir()
	bundle := &Bundle{}
	data, _ := yaml.Marshal(bundle)

	oldCwd, _ := os.Getwd()
	defer os.Chdir(oldCwd)
	os.Chdir(tmpDir)

	defaultFile := filepath.Join(tmpDir, "flwd.policy.yaml")
	os.WriteFile(defaultFile, data, 0o600)

	b, path, err := LoadFromEnvOrDefault()
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
	bundle := &Bundle{
		AllowedRegistries: []string{"EXAMPLE.COM", "GITHUB.COM"},
	}
	data, _ := yaml.Marshal(bundle)
	tmpFile := filepath.Join(tmpDir, "policy.yaml")
	os.WriteFile(tmpFile, data, 0o600)

	b, err := LoadFile(tmpFile)
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
