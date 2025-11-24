// SPDX-License-Identifier: AGPL-3.0-or-later
package verify

import "context"

// Result captures the outcome of an image verification attempt.
type Result struct {
	Verified bool
	Reason   string
}

// ImageVerifier verifies container images according to the configured trust policy.
type ImageVerifier interface {
	Verify(ctx context.Context, image string) (Result, error)
}
