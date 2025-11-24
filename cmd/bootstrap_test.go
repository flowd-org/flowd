// SPDX-License-Identifier: AGPL-3.0-or-later
package cmd

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
)

func TestRegisterScriptCommandsRegistersAliases(t *testing.T) {
	tmp := t.TempDir()
	scriptsDir := filepath.Join(tmp, "scripts")
	jobDir := filepath.Join(scriptsDir, "demo", "build")
	if err := os.MkdirAll(filepath.Join(jobDir, "config.d"), 0o755); err != nil {
		t.Fatal(err)
	}
	config := `version: v1
job:
  id: demo.build
  name: Demo Build
`
	if err := os.WriteFile(filepath.Join(jobDir, "config.d", "config.yaml"), []byte(config), 0o644); err != nil {
		t.Fatal(err)
	}
	flwd := `aliases:
  - from: "demo/build"
    to: "build-alias"
`
	if err := os.WriteFile(filepath.Join(scriptsDir, "flwd.yaml"), []byte(flwd), 0o644); err != nil {
		t.Fatal(err)
	}

	rootCmd := &cobra.Command{Use: "flwd"}
	if err := RegisterScriptCommands(rootCmd, scriptsDir); err != nil {
		t.Fatalf("RegisterScriptCommands error: %v", err)
	}

	aliasCmd, _, err := rootCmd.Find([]string{"build-alias"})
	if err != nil {
		t.Fatalf("alias lookup failed: %v", err)
	}
	if aliasCmd == nil {
		t.Fatalf("alias command not registered")
	}
	if aliasCmd.Annotations["isAlias"] != "true" {
		t.Fatalf("expected alias annotation, got %v", aliasCmd.Annotations)
	}
	if aliasCmd.Annotations["aliasTarget"] != "demo/build" {
		t.Fatalf("unexpected alias target annotation %q", aliasCmd.Annotations["aliasTarget"])
	}
}
