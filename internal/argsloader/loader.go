// SPDX-License-Identifier: AGPL-3.0-or-later
package argsloader

import (
	"fmt"
	"strings"

	"github.com/flowd-org/flowd/internal/configloader"
	"github.com/flowd-org/flowd/internal/types"
	"github.com/spf13/cobra"
)

// AttachFlags inspects the job config ArgSpec and registers Cobra flags accordingly.
// Backwards compatible with legacy `arguments:` map via configloader.
func AttachFlags(cmd *cobra.Command, dirPath string) error {
	cfg, err := configloader.LoadConfig(dirPath)
	if err != nil {
		// If config missing, skip silently as before
		return nil
	}

	var spec *types.ArgSpec
	if cfg.ArgSpec != nil && len(cfg.ArgSpec.Args) > 0 {
		spec = cfg.ArgSpec
	} else {
		// If still nil, nothing to attach
		return nil
	}

	for _, a := range spec.Args {
		name := a.Name
		desc := a.Description
		switch a.Type {
		case "string":
			def, _ := a.Default.(string)
			cmd.Flags().String(name, def, desc)
		case "boolean":
			def, _ := a.Default.(bool)
			cmd.Flags().Bool(name, def, desc)
		case "integer":
			// YAML int can be decoded as int/float64; attempt best-effort
			var ival int
			switch v := a.Default.(type) {
			case int:
				ival = v
			case int64:
				ival = int(v)
			case float64:
				ival = int(v)
			}
			cmd.Flags().Int(name, ival, desc)
		case "array":
			// Phase 1 supports array<string> only
			cmd.Flags().StringArray(name, nil, desc)
		case "object":
			// Accept repeated k=v pairs; engine parses into map according to value_type (string in Phase 1)
			cmd.Flags().StringArray(name, nil, desc)
		default:
			return fmt.Errorf("unsupported arg type %q for %s", a.Type, name)
		}

		if a.Required {
			_ = cmd.MarkFlagRequired(name)
		}
		if len(a.Enum) > 0 && a.Type == "string" {
			choices := append([]string{}, a.Enum...)
			_ = cmd.RegisterFlagCompletionFunc(name, func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
				var out []string
				for _, c := range choices {
					if strings.HasPrefix(c, toComplete) {
						out = append(out, c)
					}
				}
				return out, cobra.ShellCompDirectiveNoFileComp
			})
		}
	}

	return nil
}
