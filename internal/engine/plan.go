// SPDX-License-Identifier: AGPL-3.0-or-later
package engine

import (
	"strings"

	"github.com/flowd-org/flowd/internal/events"
	"github.com/flowd-org/flowd/internal/types"
)

// BuildPlan produces a minimal Plan object for previews and artifacts.
// Secrets are redacted (replaced with "[secret]").
func BuildPlan(jobID string, cfg *types.Config, spec *types.ArgSpec, bind *Binding) types.Plan {
	plan := types.Plan{JobID: jobID}
	if spec != nil {
		plan.EffectiveArgSpec = *spec
	}
	if cfg != nil {
		if plan.ExecutorPreview == nil {
			plan.ExecutorPreview = map[string]interface{}{}
		}
		if cfg.Interpreter != "" {
			plan.ExecutorPreview["interpreter"] = cfg.Interpreter
		}
		if cfg.Executor != "" {
			plan.ExecutorPreview["executor"] = strings.ToLower(cfg.Executor)
		}
		if strings.HasPrefix(cfg.Interpreter, "container:") {
			plan.ExecutorPreview["container_image"] = strings.TrimPrefix(cfg.Interpreter, "container:")
		}
	}

	if bind != nil && spec != nil {
		resolved := map[string]interface{}{}
		for _, arg := range spec.Args {
			if val, ok := bind.Values[arg.Name]; ok {
				resolved[arg.Name] = val
			}
		}
		if len(bind.SecretNames) > 0 {
			resolved = events.RedactSecrets(resolved, bind.SecretNames)
		}
		if len(resolved) > 0 {
			plan.ResolvedArgs = resolved
		}
	}

	return plan
}
