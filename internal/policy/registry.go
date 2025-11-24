// SPDX-License-Identifier: AGPL-3.0-or-later
package policy

import (
	"errors"
	"strings"
)

// RegistryFromImage extracts the registry host[:port] portion from an OCI image reference.
// When the reference does not include an explicit registry, docker.io is assumed.
func RegistryFromImage(image string) (string, error) {
	image = strings.TrimSpace(image)
	if image == "" {
		return "", errors.New("empty image reference")
	}
	if strings.Contains(image, "://") {
		return "", errors.New("image reference must not include scheme")
	}
	if strings.HasPrefix(image, "/") {
		return "", errors.New("image reference must not start with slash")
	}

	parts := strings.Split(image, "/")
	if len(parts) == 0 {
		return "", errors.New("invalid image reference")
	}
	candidate := parts[0]
	if candidate == "" {
		return "", errors.New("invalid image reference")
	}
	if strings.Contains(candidate, ".") || strings.Contains(candidate, ":") || candidate == "localhost" {
		return strings.ToLower(candidate), nil
	}
	return "docker.io", nil
}

// RegistryAllowed reports whether the registry host is present in the allow-list.
func RegistryAllowed(registry string, allowed []string) bool {
	if len(allowed) == 0 {
		return true
	}
	registry = strings.ToLower(strings.TrimSpace(registry))
	for _, entry := range allowed {
		if registry == strings.ToLower(strings.TrimSpace(entry)) {
			return true
		}
	}
	return false
}
