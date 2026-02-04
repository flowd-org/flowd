// SPDX-License-Identifier: AGPL-3.0-or-later
package engine

import (
	"strings"

	"github.com/flowd-org/flowd/internal/types"
)

// ExecutorMode returns the effective execution mode based on config.
// Defaults to "shell" when no explicit mode is configured.
func ExecutorMode(cfg *types.Config) string {
	if cfg == nil {
		return "shell"
	}
	mode := strings.ToLower(strings.TrimSpace(cfg.Executor))
	interp := strings.ToLower(strings.TrimSpace(cfg.Interpreter))
	if strings.HasPrefix(interp, "container:") && mode == "" {
		mode = "container"
	}
	if mode == "" {
		mode = "shell"
	}
	return mode
}
