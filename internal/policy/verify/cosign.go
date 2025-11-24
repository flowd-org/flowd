// SPDX-License-Identifier: AGPL-3.0-or-later
package verify

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

// ExecCommander spawns the underlying cosign command. Extracted for tests.
type ExecCommander func(ctx context.Context, name string, args ...string) *exec.Cmd

// CosignVerifier invokes the external cosign CLI in keyless mode.
type CosignVerifier struct {
	Command ExecCommander
}

// NewCosignVerifier returns a verifier that shells out to `cosign verify --keyless`.
func NewCosignVerifier() *CosignVerifier {
	return &CosignVerifier{
		Command: exec.CommandContext,
	}
}

// Verify runs `cosign verify --keyless <image>`. A non-zero exit status is treated
// as a verification failure (Verified=false) with the combined output captured as
// the reason. Startup failures (e.g., cosign binary missing) surface as errors.
func (v *CosignVerifier) Verify(ctx context.Context, image string) (Result, error) {
	image = strings.TrimSpace(image)
	if image == "" {
		return Result{}, errors.New("image reference is required")
	}
	command := v.Command
	if command == nil {
		command = exec.CommandContext
	}
	cmd := command(ctx, "cosign", "verify", "--keyless", image)
	output, err := cmd.CombinedOutput()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			reason := strings.TrimSpace(string(output))
			if reason == "" {
				reason = exitErr.Error()
			}
			return Result{Verified: false, Reason: reason}, nil
		}
		return Result{}, fmt.Errorf("cosign execute: %w", err)
	}
	return Result{Verified: true}, nil
}
