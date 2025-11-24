//go:build unix

package executor

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestUmaskProducesSecureFiles(t *testing.T) {
	if _, err := os.Stat("/bin/bash"); err != nil {
		t.Skip("/bin/bash not available")
	}

	root := t.TempDir()
	dir := filepath.Join(root, "scripts", "demo")
	configDir := filepath.Join(dir, "config.d")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatal(err)
	}
	config := "interpreter: /bin/bash\n"
	if err := os.WriteFile(filepath.Join(configDir, "config.yaml"), []byte(config), 0o644); err != nil {
		t.Fatal(err)
	}

	scriptPath := filepath.Join(dir, "100_write.sh")
	script := "#!/usr/bin/env bash\nset -Eeuo pipefail\nIFS=$'\n\t'\nDIR=$(dirname \"$0\")\nFILE=$DIR/secure.out\necho secret > \"$FILE\"\n"
	if err := os.WriteFile(scriptPath, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(cwd)
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}

	ecfg := ExecutorConfig{}
	if _, err := RunScripts(context.Background(), filepath.Join("scripts", "demo"), ecfg); err != nil {
		t.Fatalf("RunScripts error: %v", err)
	}

	stat, err := os.Stat(filepath.Join(dir, "secure.out"))
	if err != nil {
		t.Fatalf("secure.out missing: %v", err)
	}
	if stat.Mode().Perm() != 0o600 {
		t.Fatalf("expected 0600 perms, got %v", stat.Mode().Perm())
	}
}
