// SPDX-License-Identifier: AGPL-3.0-or-later
package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/flowd-org/flowd/internal/configloader"
	"github.com/flowd-org/flowd/internal/engine"
	"github.com/spf13/cobra"
)

func NewPlanCmd(root *cobra.Command) *cobra.Command {
	var asJSON bool
	var profile string
	c := &cobra.Command{
		Use:   ":plan <job>",
		Short: "Preview plan for a job (no execution)",
		Args: func(cmd *cobra.Command, args []string) error {
			if len(args) < 1 {
				return errors.New("requires job path, e.g., 'foo' or 'foo bar'")
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			jobPath := args
			// Resolve command for job path
			// Cobra expects a single string; join with space and use root.Find
			query := strings.Join(jobPath, " ")
			target, _, err := root.Find(append([]string{}, jobPath...))
			if err != nil || target == nil {
				return fmt.Errorf("job not found: %s", query)
			}
			scriptDir := ""
			if target.Annotations != nil {
				scriptDir = target.Annotations["scriptDir"]
			}
			if scriptDir == "" {
				return fmt.Errorf("job has no scriptDir metadata: %s", query)
			}

			cfg, err := configloader.LoadConfig(scriptDir)
			if err != nil {
				return err
			}

			spec := cfg.ArgSpec
			plan := engine.BuildPlan(target.CommandPath(), cfg, spec, nil)
			// Resolve profile precedence for CLI plan: flag > env > default
			if profile == "" {
				if env := os.Getenv("FLWD_PROFILE"); env != "" {
					profile = env
				}
			}
			if profile == "" {
				profile = "secure"
			}
			plan.SecurityProfile = strings.ToLower(profile)

			if asJSON {
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				return enc.Encode(plan)
			}
			// Human summary (minimal)
			fmt.Printf("Job: %s\n", plan.JobID)
			if plan.ExecutorPreview != nil {
				if interp, ok := plan.ExecutorPreview["interpreter"].(string); ok && interp != "" {
					fmt.Printf("Interpreter: %s\n", interp)
				}
			}
			fmt.Println("Args:")
			if spec == nil || len(spec.Args) == 0 {
				fmt.Println("  (none)")
			} else {
				for _, a := range spec.Args {
					req := ""
					if a.Required {
						req = " (required)"
					}
					suffix := ""
					if a.Secret || a.Format == "secret" {
						suffix = " [secret]"
					}
					fmt.Printf("  - %s: %s%s%s\n", a.Name, a.Type, req, suffix)
				}
			}
			if len(plan.ResolvedArgs) > 0 {
				fmt.Println("Resolved Args:")
				keys := make([]string, 0, len(plan.ResolvedArgs))
				for k := range plan.ResolvedArgs {
					keys = append(keys, k)
				}
				sort.Strings(keys)
				for _, k := range keys {
					fmt.Printf("  - %s: %v\n", k, plan.ResolvedArgs[k])
				}
			}
			return nil
		},
	}
	c.Flags().BoolVar(&asJSON, "json", false, "Output plan as JSON")
	c.Flags().StringVar(&profile, "profile", "", "Security profile (secure|permissive|disabled); overrides FLWD_PROFILE")
	return c
}
