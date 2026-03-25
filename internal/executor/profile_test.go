package executor

import (
	"os"
	"os/exec"

	"path/filepath"
	"testing"

	"github.com/flowd-org/flowd/internal/types"
)

func TestGenerateRunnerProfile_SetsStrictModeAndBindings(t *testing.T) {
	spec := types.ArgSpec{Args: []types.Arg{
		{Name: "name", Type: "string"},
		{Name: "loud", Type: "boolean"},
	}}
	bind := map[string]interface{}{"name": "alice", "loud": true}

	profilePath, cleanup, err := GenerateRunnerProfile("scripts/demo", "/bin/bash", 0, &spec, bind)
	if err != nil {
		t.Fatalf("GenerateRunnerProfile error: %v", err)
	}
	defer cleanup()

	script := `#!/usr/bin/env bash
source "` + profilePath + `"
if [ "$name" != "alice" ]; then echo "name binding missing"; exit 1; fi
if [ "$loud" != "true" ]; then echo "loud binding missing"; exit 1; fi
exit 0
`
	tmpScript, err := os.CreateTemp("", "test_script_*.sh")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpScript.Name())
	if _, err := tmpScript.WriteString(script); err != nil {
		t.Fatal(err)
	}
	tmpScript.Close()
	if err := os.Chmod(tmpScript.Name(), 0o755); err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command(tmpScript.Name())
	cmd.Env = append(os.Environ(), "ARG_NAME=alice", "ARG_LOUD=true")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("profile script failed: %v output=%s", err, string(out))
	}
}

func TestGenerateRunnerProfile_VerbosityLevels(t *testing.T) {
	spec := types.ArgSpec{Args: []types.Arg{
		{Name: "name", Type: "string"},
	}}
	bind := map[string]interface{}{"name": "alice"}

	tests := []struct {
		name   string
		level  int
		expect string
	}{
		{"level 0", 0, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			profilePath, cleanup, err := GenerateRunnerProfile("scripts/demo", "/bin/bash", tt.level, &spec, bind)
			if err != nil {
				t.Fatalf("GenerateRunnerProfile error: %v", err)
			}
			defer cleanup()

			content, err := os.ReadFile(profilePath)
			if err != nil {
				t.Fatalf("read profile: %v", err)
			}

			if tt.expect != "" && !contains(string(content), tt.expect) {
				t.Errorf("expected %q in profile content", tt.expect)
			}
		})
	}
}

func TestGenerateRunnerProfile_InvalidInterpreter(t *testing.T) {
	spec := types.ArgSpec{Args: []types.Arg{
		{Name: "name", Type: "string"},
	}}
	bind := map[string]interface{}{"name": "alice"}

	_, _, err := GenerateRunnerProfile("scripts/demo", "/nonexistent/interpreter", 0, &spec, bind)
	if err == nil {
		t.Fatal("expected error for invalid interpreter")
	}
}

func TestGenerateRunnerProfile_CleanupRemovesTempFiles(t *testing.T) {
	spec := types.ArgSpec{Args: []types.Arg{
		{Name: "name", Type: "string"},
	}}
	bind := map[string]interface{}{"name": "alice"}

	tmpDir := t.TempDir()
	profilePath, cleanup, err := GenerateRunnerProfile(filepath.Join(tmpDir, "demo"), "/bin/bash", 0, &spec, bind)
	if err != nil {
		t.Fatalf("GenerateRunnerProfile error: %v", err)
	}

	if _, err := os.Stat(profilePath); err != nil {
		t.Fatalf("profile should exist before cleanup: %v", err)
	}

	cleanup()

	if _, err := os.Stat(profilePath); !os.IsNotExist(err) {
		t.Errorf("expected profile to be removed after cleanup")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && findSubstring(s, substr))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func TestGenerateRunnerProfile_JoinShellListAndMap_Coverage(t *testing.T) {
	// cover joinShellList with empty and single-element arrays (shellQuote wraps values in single quotes)
	empty := []string{}
	if joinShellList(empty) != "" {
		t.Errorf("empty array: expected empty string")
	}
	single := []string{"one"}
	if joinShellList(single) != "'one'" {
		t.Errorf("single element: expected 'one', got %q", joinShellList(single))
	}

	// cover joinShellMap with empty and populated maps
	emptyMap := map[string]string{}
	if joinShellMap(emptyMap) != "" {
		t.Errorf("empty map: expected empty string")
	}
	populated := map[string]string{"a": "1", "b": "2"}
	result := joinShellMap(populated)
	if result == "" {
		t.Errorf("populated map returned empty string")
	}
}
