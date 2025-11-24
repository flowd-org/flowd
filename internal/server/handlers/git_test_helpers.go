package handlers

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func createGitJobRepo(t *testing.T, jobID, scriptContent string) (string, string) {
	t.Helper()
	repoDir := t.TempDir()

	runGitTest(t, repoDir, "init")
	runGitTest(t, repoDir, "config", "user.name", "Runner Tests")
	runGitTest(t, repoDir, "config", "user.email", "flwd-tests@example.com")
	runGitTest(t, repoDir, "symbolic-ref", "HEAD", "refs/heads/main")

	jobDir := filepath.Join(repoDir, "scripts", jobID)
	if err := os.MkdirAll(filepath.Join(jobDir, "config.d"), 0o755); err != nil {
		t.Fatalf("mkdir config.d: %v", err)
	}

	config := fmt.Sprintf("version: v1\njob:\n  id: %s\n  name: %s Job\nargspec:\n  args:\n    - name: name\n      type: string\n      required: true\n", jobID, jobID)
	if err := os.WriteFile(filepath.Join(jobDir, "config.d", "config.yaml"), []byte(config), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	scriptPath := filepath.Join(jobDir, "100_main.sh")
	content := scriptContent
	if content == "" {
		content = "#!/usr/bin/env bash\nset -euo pipefail\necho git-job run\n"
	}
	if err := os.WriteFile(scriptPath, []byte(content), 0o755); err != nil {
		t.Fatalf("write script: %v", err)
	}

	runGitTest(t, repoDir, "add", ".")
	runGitTest(t, repoDir, "commit", "-m", "add job")
	commit := strings.TrimSpace(runGitTest(t, repoDir, "rev-parse", "HEAD"))

	return repoDir, commit
}

func runGitTest(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	if err := cmd.Run(); err != nil {
		t.Fatalf("git %s failed: %v (%s)", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return stdout.String()
}
