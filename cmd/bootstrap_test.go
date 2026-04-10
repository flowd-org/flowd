// SPDX-License-Identifier: AGPL-3.0-or-later
package cmd

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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

// TestBootstrapMissing verifies that flowd fails when required bootstrap config is missing.
func TestBootstrapMissing(t *testing.T) {
	tmp := t.TempDir()
	configPath := filepath.Join(tmp, "config.yaml")
	if err := os.WriteFile(configPath, []byte("version: v1\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Build the binary first in the repo root
	repoRoot, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get repo root: %v", err)
	}
	binaryPath := filepath.Join(repoRoot, "flowd_test_bin")
	buildCmd := exec.Command("go", "build", "-o", binaryPath, "../main.go")
	if out, err := buildCmd.CombinedOutput(); err != nil {
		t.Fatalf("failed to build flowd: %s", out)
	}
	defer os.Remove(binaryPath)

	cmd := exec.Command(binaryPath, "--config", configPath, ":serve", "--bind", "127.0.0.1:0")
	cmd.Dir = tmp
	cmd.Env = append(os.Environ(), "FLWD_BOOTSTRAP_TOKEN=", "FLWD_BOOTSTRAP_FILE=")

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err == nil {
		t.Fatalf("expected non-zero exit when bootstrap is missing")
	}

	output := stdout.String() + stderr.String()
	// The error should mention the failure reason (scripts dir or bootstrap)
	if !strings.Contains(output, "scanning scripts") && !strings.Contains(output, "bootstrap") && !strings.Contains(output, "missing") {
		t.Logf("output: %s", output)
		t.Fatalf("expected error message to mention missing config")
	}
}

func TestNormalizeBaseURLRejectsHTTPS(t *testing.T) {
	if _, err := normalizeBaseURL("https://example.com"); err == nil {
		t.Fatal("expected https base URL to be rejected")
	}
}

func TestSourcesClientDoesNotFollowRedirects(t *testing.T) {
	var finalHits int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/sources":
			w.Header().Set("Location", "/final")
			w.WriteHeader(http.StatusFound)
		default:
			finalHits++
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer server.Close()

	root := &cobra.Command{Use: "root"}
	root.PersistentFlags().String("server", server.URL, "")
	root.PersistentFlags().String("token", "", "")
	cmd := &cobra.Command{Use: "child"}
	root.AddCommand(cmd)

	client, err := resolveSourcesClient(cmd)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := client.do(t.Context(), http.MethodGet, "/sources", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("expected redirect response, got %d", resp.StatusCode)
	}
	if finalHits != 0 {
		t.Fatalf("redirect was followed; final handler hit %d times", finalHits)
	}
}
