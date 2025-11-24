// SPDX-License-Identifier: AGPL-3.0-or-later
package verify

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

// BundleVerifier validates a policy bundle reference before use.
type BundleVerifier interface {
	Verify(ctx context.Context, ref string) error
}

// CosignBundleVerifier invokes `cosign verify --keyless` for bundle references.
type CosignBundleVerifier struct {
	Command ExecCommander
}

// NewCosignBundleVerifier constructs a verifier that shells out to cosign.
func NewCosignBundleVerifier() *CosignBundleVerifier {
	return &CosignBundleVerifier{
		Command: exec.CommandContext,
	}
}

// Verify executes `cosign verify --keyless <ref>` treating any non-zero exit
// status as a verification failure.
func (v *CosignBundleVerifier) Verify(ctx context.Context, ref string) error {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return errors.New("bundle reference is required")
	}
	command := v.Command
	if command == nil {
		command = exec.CommandContext
	}
	cmd := command(ctx, "cosign", "verify", "--keyless", ref)
	output, err := cmd.CombinedOutput()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			reason := strings.TrimSpace(string(output))
			if reason == "" {
				reason = exitErr.Error()
			}
			return fmt.Errorf("cosign verify failed: %s", reason)
		}
		return fmt.Errorf("cosign execute: %w", err)
	}
	return nil
}
