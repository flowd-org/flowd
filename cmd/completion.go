// SPDX-License-Identifier: AGPL-3.0-or-later
package cmd

import (
    "os"

    "github.com/spf13/cobra"
)

func NewCompletionCmd(root *cobra.Command) *cobra.Command {
    return &cobra.Command{
        Use:       "completion [bash|zsh|fish|powershell]",
        Short:     "Generate shell completions",
        Args:      cobra.ExactValidArgs(1),
        ValidArgs: []string{"bash", "zsh", "fish", "powershell"},
        RunE: func(cmd *cobra.Command, args []string) error {
            switch args[0] {
            case "bash":
                return root.GenBashCompletion(os.Stdout)
            case "zsh":
                return root.GenZshCompletion(os.Stdout)
            case "fish":
                return root.GenFishCompletion(os.Stdout, true)
            case "powershell":
                return root.GenPowerShellCompletion(os.Stdout)
            }
            return nil
        },
    }
}
