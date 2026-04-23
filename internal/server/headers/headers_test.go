// SPDX-License-Identifier: AGPL-3.0-or-later
package headers_test

import (
	"testing"

	"github.com/flowd-org/flowd/internal/server/headers"
)

func TestDiscoveryErrors(t *testing.T) {
	if headers.DiscoveryErrors != "X-Flowd-Discovery-Errors" {
		t.Errorf("expected DiscoveryErrors to be 'X-Flowd-Discovery-Errors', got '%s'", headers.DiscoveryErrors)
	}
}
