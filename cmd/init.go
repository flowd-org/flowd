// SPDX-License-Identifier: AGPL-3.0-or-later
package cmd

import (
    "fmt"
    "os"
    "path/filepath"
    "strings"

    "github.com/spf13/cobra"
)

var initCmd = &cobra.Command{
    Use:   "init [bash|ps] <command[/subcommand/...]>",
    Short: "Initialize a new command/subcommand script structure",
    Args:  cobra.ExactArgs(2),
    RunE: func(cmd *cobra.Command, args []string) error {
        shell := strings.ToLower(args[0])
        subpath := args[1]

        switch shell {
        case "bash", "ps":
            // ok
        default:
            return fmt.Errorf("unsupported shell type: %s (use bash or ps)", shell)
        }

        ext := ".sh"
        if shell == "ps" {
            ext = ".ps1"
        }

        // target root: scripts/cmd[/subcmd]
        fullPath := filepath.Join("scripts", filepath.FromSlash(subpath))
        configPath := filepath.Join(fullPath, "config.d")
        varsPath := filepath.Join(configPath, "vars")
        libsPath := filepath.Join(configPath, "libs")

        // create all folders
        for _, p := range []string{varsPath, libsPath} {
            if err := os.MkdirAll(p, 0755); err != nil {
                return fmt.Errorf("creating %s: %w", p, err)
            }
        }

        // write config.yaml
        configYaml := `interpreter: /bin/bash
arguments:
  sample:
    type: string
    default: "demo"
    description: "Sample argument"
error_handling:
  policy: abort
`
        if shell == "ps" {
            configYaml = strings.Replace(configYaml, "/bin/bash", "pwsh", 1)
        }

        if err := os.WriteFile(filepath.Join(configPath, "config.yaml"), []byte(configYaml), 0644); err != nil {
            return fmt.Errorf("writing config.yaml: %w", err)
        }

        // write example vars file
        varsExample := "# example exported variable\nexport SAMPLE_VAR=\"hello\"\n"
        if shell == "ps" {
            varsExample = "$env:SAMPLE_VAR = 'hello'\n"
        }

        _ = os.WriteFile(filepath.Join(varsPath, "example"+ext), []byte(varsExample), 0644)

        // write example lib function
        libExample := "function say_hello() {\n  echo \"Hello from lib!\"\n}\n"
        if shell == "ps" {
            libExample = "function Say-Hello {\n  Write-Host 'Hello from lib!'\n}\n"
        }

        _ = os.WriteFile(filepath.Join(libsPath, "common"+ext), []byte(libExample), 0644)

        // write initial script
        mainScript := fmt.Sprintf("#!/usr/bin/env %s\n\necho \"SAMPLE_VAR: $SAMPLE_VAR\"\n", shell)
        if shell == "ps" {
            mainScript = "Write-Host \"SAMPLE_VAR: $env:SAMPLE_VAR\"\nSay-Hello\n"
        }

        _ = os.WriteFile(filepath.Join(fullPath, "000_main"+ext), []byte(mainScript), 0755)

        fmt.Printf("[OK] Initialized %s command structure at %s\n", shell, fullPath)
        return nil
    },
}
