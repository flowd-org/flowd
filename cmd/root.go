// SPDX-License-Identifier: AGPL-3.0-or-later
package cmd

import (
	"fmt"
	"os"

	"github.com/flowd-org/flowd/internal/paths"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "flwd",
	Short: "Modular shell CLI flwd",
}

func Execute() {
	if dataDir := os.Getenv("DATA_DIR"); dataDir != "" {
		paths.SetDataDirOverride(dataDir)
	}

	// Dynamically register commands based on scripts folder
	if err := RegisterScriptCommands(rootCmd, "scripts"); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	rootCmd.AddCommand(NewInternalCompleteCmd(rootCmd))
	rootCmd.AddCommand(NewCompletionCmd(rootCmd))
	rootCmd.AddCommand(initCmd)
	rootCmd.AddCommand(NewSourcesCmd())
	rootCmd.AddCommand(NewJobsCmd(rootCmd))
	rootCmd.AddCommand(NewPlanCmd(rootCmd))
	rootCmd.AddCommand(NewServeCmd())

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func addCommonFlags(cmd *cobra.Command) {
	cmd.PersistentFlags().Bool("dry-run", false, "Simulate execution")
	cmd.PersistentFlags().CountP("verbose", "v", "Increase verbosity")
	cmd.PersistentFlags().BoolP("quiet", "q", false, "Quiet mode")
	cmd.PersistentFlags().Bool("strict", false, "Fail fast on errors")
	cmd.PersistentFlags().String("on-error", "", "Override error policy (abort|continue|retry)")
	cmd.PersistentFlags().String("report", "", "Output report format (json|yaml)")
	cmd.PersistentFlags().String("report-file", "", "Write execution report to file (JSON/YAML format)")
	cmd.PersistentFlags().String("profile", "", "Security profile (secure|permissive|disabled); overrides FLWD_PROFILE")
}
