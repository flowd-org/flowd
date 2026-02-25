package harness

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFixtureJobIDs(t *testing.T) {
	jobIDs := FixtureJobIDs()
	if len(jobIDs) == 0 {
		t.Errorf("Expected non-empty job IDs")
	}
}

func TestStageFixtures(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "fixtures-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	runRoot := filepath.Join(tmpDir, "run")
	if err := os.MkdirAll(runRoot, 0755); err != nil {
		t.Fatalf("Failed to create run root: %v", err)
	}

	stagedRef, err := StageFixtures(runRoot)
	if err != nil {
		t.Fatalf("StageFixtures failed: %v", err)
	}

	expectedRef := "fixtures/tree-v1"
	if stagedRef != expectedRef {
		t.Errorf("Expected staged ref %q, got %q", expectedRef, stagedRef)
	}

	stagedPath := filepath.Join(runRoot, "scripts", "fixtures", "tree-v1")
	if _, err := os.Stat(stagedPath); os.IsNotExist(err) {
		t.Errorf("Staged fixtures directory does not exist at %s", stagedPath)
	}

	// Verify .sh files are executable
	shFiles := []string{
		filepath.Join(stagedPath, "conformance", "ulc-smoke-bash", "000_smoke.sh"),
		filepath.Join(stagedPath, "conformance", "ulc-smoke-pwsh", "000_smoke.ps1"),
	}
	for _, f := range shFiles {
		info, err := os.Stat(f)
		if err != nil {
			t.Errorf("Failed to stat %s: %v", f, err)
			continue
		}
		// bash script should be executable
		if filepath.Ext(f) == ".sh" {
			if info.Mode()&0111 == 0 {
				t.Errorf("Expected %s to be executable", f)
			}
		}
	}
}
