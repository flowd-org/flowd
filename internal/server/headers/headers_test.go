// SPDX-License-Identifier: AGPL-3.0-or-later
package headers

import (
	"testing"
)

func TestDiscoveryErrors(t *testing.T) {
	if DiscoveryErrors != "X-Flowd-Discovery-Errors" {
		t.Errorf("expected DiscoveryErrors to be 'X-Flowd-Discovery-Errors', got '%s'", DiscoveryErrors)
	}
}
