// SPDX-License-Identifier: AGPL-3.0-or-later
package policy

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	yaml "gopkg.in/yaml.v3"
)

// LoadFile loads a policy bundle from the given path.
func LoadFile(path string) (*Bundle, error) {
	if path == "" {
		return nil, errors.New("missing policy file path")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read policy file: %w", err)
	}
	var b Bundle
	if err := yaml.Unmarshal(data, &b); err != nil {
		return nil, fmt.Errorf("parse policy file: %w", err)
	}
	if err := validate(&b); err != nil {
		return nil, err
	}
	return &b, nil
}

// LoadFromEnvOrDefault attempts to load from FLWD_POLICY_FILE, falling back to
// ./flwd.policy.yaml if present. Returns (nil, "", nil) when no file exists.
func LoadFromEnvOrDefault() (*Bundle, string, error) {
	path := os.Getenv("FLWD_POLICY_FILE")
	if path == "" {
		// try default in working directory
		candidate := filepath.Clean("flwd.policy.yaml")
		if _, err := os.Stat(candidate); err == nil {
			path = candidate
		}
	}
	if path == "" {
		return nil, "", nil
	}
	b, err := LoadFile(path)
	return b, path, err
}

func validate(b *Bundle) error {
	if b == nil {
		return nil
	}
	if b.VerifySignatures != nil {
		if _, ok := NormalizeVerifySignatures(*b.VerifySignatures); !ok {
			return fmt.Errorf("invalid verify_signatures: %q", *b.VerifySignatures)
		}
	}
	// Normalize allowed registries to lowercase hosts (keep order).
	for i := range b.AllowedRegistries {
		b.AllowedRegistries[i] = lower(b.AllowedRegistries[i])
	}
	return nil
}
