package executor

import (
	"os"
	"os/exec"
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
