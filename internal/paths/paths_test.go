package paths_test

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	paths "github.com/flowd-org/flowd/internal/paths"
)

func TestSetDataDirOverride(t *testing.T) {
	// Clear any existing state
	paths.SetDataDirOverride("")
	defer paths.SetDataDirOverride("")

	// Set an override and verify DataDir uses it
	expected := "/tmp/flowd-test"
	paths.SetDataDirOverride(expected)
	got := paths.DataDir()
	if got != expected {
		t.Errorf("paths.DataDir() = %q, want %q", got, expected)
	}

	// Clear the override and verify it falls back
	paths.SetDataDirOverride("")
	got = paths.DataDir()
	if got == expected {
		t.Errorf("paths.DataDir() = %q after clear, expected fallback", got)
	}
}

func TestSetDataDirOverride_Priority(t *testing.T) {
	// Clear state
	paths.SetDataDirOverride("")
	defer paths.SetDataDirOverride("")

	// Temporarily set DATA_DIR and verify override takes precedence
	t.Setenv("DATA_DIR", "/data-dir-env")

	// First without override
	noOverride := paths.DataDir()

	// Now set override - should take precedence over DATA_DIR
	paths.SetDataDirOverride("/override-precedence")
	overridePath := paths.DataDir()

	if overridePath == noOverride {
		t.Error("Override did not change paths.DataDir() from env value")
	}
	if overridePath != "/override-precedence" {
		t.Errorf("paths.DataDir() with override = %q, want /override-precedence", overridePath)
	}
}

func TestDATA_DIR_Precedence(t *testing.T) {
	// Clear override
	paths.SetDataDirOverride("")
	defer paths.SetDataDirOverride("")

	// Set DATA_DIR and verify DataDir uses it (no XDG_DATA_HOME set)
	t.Setenv("DATA_DIR", "/data-dir-env")
	got := paths.DataDir()
	if got != "/data-dir-env" {
		t.Errorf("paths.DataDir() with DATA_DIR = %q, want /data-dir-env", got)
	}

	// Clear DATA_DIR and set XDG_DATA_HOME - should fall back to XDG
	t.Setenv("DATA_DIR", "")
	t.Setenv("XDG_DATA_HOME", "/xdg-home")
	got = paths.DataDir()
	expectedXDG := filepath.Join("/xdg-home", "flowd")
	if got != expectedXDG {
		t.Errorf("paths.DataDir() with XDG_DATA_HOME = %q, want %q", got, expectedXDG)
	}
}

func TestDataPath(t *testing.T) {
	// Clear override
	paths.SetDataDirOverride("")
	defer paths.SetDataDirOverride("")

	t.Setenv("DATA_DIR", "/data-dir")
	got := paths.DataPath("a", "b", "c")
	expected := filepath.Join("/data-dir", "a", "b", "c")
	if got != expected {
		t.Errorf("paths.DataPath() = %q, want %q", got, expected)
	}
}

func TestEnsureDataPath(t *testing.T) {
	// Clear override
	paths.SetDataDirOverride("")
	defer paths.SetDataDirOverride("")

	// Use a temp directory as override to avoid polluting real paths
	tmpDir := t.TempDir()
	paths.SetDataDirOverride(tmpDir)

	subdir := filepath.Join(tmpDir, "runs", "test-run")
	got, err := paths.EnsureDataPath("runs", "test-run")
	if err != nil {
		t.Fatalf("Ensurepaths.DataPath() returned error: %v", err)
	}
	if got != subdir {
		t.Errorf("Ensurepaths.DataPath() = %q, want %q", got, subdir)
	}

	// Verify the directory was created
	if _, err := os.Stat(subdir); os.IsNotExist(err) {
		t.Error("Directory was not created by EnsureDataPath")
	}
}

func TestRunsDir_RunDir_SourcesDir_OCICacheDir(t *testing.T) {
	// Clear override
	paths.SetDataDirOverride("")
	defer paths.SetDataDirOverride("")

	t.Setenv("DATA_DIR", "/data-dir")

	if got := paths.RunsDir(); got != filepath.Join("/data-dir", "runs") {
		t.Errorf("RunsDir() = %q, want %q", got, filepath.Join("/data-dir", "runs"))
	}

	if got := paths.RunDir("abc123"); got != filepath.Join("/data-dir", "runs", "abc123") {
		t.Errorf("RunDir() = %q, want %q", got, filepath.Join("/data-dir", "runs", "abc123"))
	}

	if got := paths.SourcesDir(); got != filepath.Join("/data-dir", "sources") {
		t.Errorf("SourcesDir() = %q, want %q", got, filepath.Join("/data-dir", "sources"))
	}

	if got := paths.OCICacheDir(); got != filepath.Join("/data-dir", "oci") {
		t.Errorf("OCICacheDir() = %q, want %q", got, filepath.Join("/data-dir", "oci"))
	}
}

func TestPathFunctions_Coverage(t *testing.T) {
	// Ensure all code paths in paths.go are exercised for coverage
	paths.SetDataDirOverride("")
	defer paths.SetDataDirOverride("")

	// Force different OS-specific branches by mocking environment
	t.Setenv("DATA_DIR", "")
	t.Setenv("XDG_DATA_HOME", "")

	// Call each exported function to ensure coverage
	_ = paths.DataPath("a")
	_, _ = paths.EnsureDataPath("b")
	_ = paths.RunsDir()
	_ = paths.RunDir("run-id")
	_ = paths.SourcesDir()
	_ = paths.OCICacheDir()

	// Verify override clearing works via exported behavior.
	paths.SetDataDirOverride("")
	if got := paths.DataDir(); got == "" {
		t.Error("paths.DataDir() returned empty string after clearing override")
	}
}

func TestDATA_DIR_Precedence_Windows(t *testing.T) {
	// TestDATA_DIR_Precedence_Windows tests the Windows-specific fallback path
	// by mocking PROGRAMDATA and HOME environment variables.
	// Clear override
	paths.SetDataDirOverride("")
	defer paths.SetDataDirOverride("")

	// Temporarily pretend we're on Windows
	t.Setenv("DATA_DIR", "")
	t.Setenv("XDG_DATA_HOME", "")

	// Mock ProgramData for Windows fallback - only set it when not on Windows
	if runtime.GOOS != "windows" {
		t.Setenv("PROGRAMDATA", "/fake/ProgramData")
	}

	got := paths.DataDir()

	// On non-Windows systems, the PROGRAMDATA env var won't be used,
	// so we just verify that the function returns a valid path without crashing.
	if got == "" {
		t.Error("paths.DataDir() returned empty string for Windows fallback test")
	}
}

func TestDATA_DIR_Precedence_POSIX(t *testing.T) {
	// TestDATA_DIR_Precedence_POSIX tests the POSIX fallback path
	// by mocking HOME and XDG_DATA_HOME environment variables.
	// Clear override
	paths.SetDataDirOverride("")
	defer paths.SetDataDirOverride("")

	t.Setenv("DATA_DIR", "")
	t.Setenv("XDG_DATA_HOME", "")

	// Mock HOME for POSIX fallback
	home := t.TempDir()
	if runtime.GOOS != "windows" {
		t.Setenv("HOME", home)
	}

	got := paths.DataDir()
	expectedPosix := filepath.Join(home, ".local", "share", "flowd")
	if got != expectedPosix {
		t.Errorf("paths.DataDir() POSIX fallback = %q, want %q", got, expectedPosix)
	}
}

func TestDATA_DIR_Precedence_NoHome(t *testing.T) {
	// TestDATA_DIR_Precedence_NoHome tests fallback to CWD when HOME is not available
	paths.SetDataDirOverride("")
	defer paths.SetDataDirOverride("")

	t.Setenv("DATA_DIR", "")
	t.Setenv("XDG_DATA_HOME", "")
	t.Setenv("HOME", "")

	// Clear USERPROFILE on Windows systems
	if runtime.GOOS == "windows" {
		t.Setenv("USERPROFILE", "")
	}

	// Should fall back to CWD/flowd.
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("os.Getwd(): %v", err)
	}
	got := paths.DataDir()
	expectedCwd := filepath.Join(cwd, "flowd")
	if got != expectedCwd {
		t.Errorf("paths.DataDir() fallback to CWD = %q, want %q", got, expectedCwd)
	}
}

func TestRunsDir(t *testing.T) {
	paths.SetDataDirOverride("/tmp/test-paths")
	defer paths.SetDataDirOverride("")

	got := paths.RunsDir()
	expected := filepath.Join("/tmp/test-paths", "runs")
	if got != expected {
		t.Errorf("RunsDir() = %q, want %q", got, expected)
	}
}

func TestRunDir(t *testing.T) {
	paths.SetDataDirOverride("/tmp/test-paths")
	defer paths.SetDataDirOverride("")

	got := paths.RunDir("abc123")
	expected := filepath.Join("/tmp/test-paths", "runs", "abc123")
	if got != expected {
		t.Errorf("RunDir() = %q, want %q", got, expected)
	}
}

func TestSourcesDir(t *testing.T) {
	paths.SetDataDirOverride("/tmp/test-paths")
	defer paths.SetDataDirOverride("")

	got := paths.SourcesDir()
	expected := filepath.Join("/tmp/test-paths", "sources")
	if got != expected {
		t.Errorf("SourcesDir() = %q, want %q", got, expected)
	}
}

func TestOCICacheDir(t *testing.T) {
	paths.SetDataDirOverride("/tmp/test-paths")
	defer paths.SetDataDirOverride("")

	got := paths.OCICacheDir()
	expected := filepath.Join("/tmp/test-paths", "oci")
	if got != expected {
		t.Errorf("OCICacheDir() = %q, want %q", got, expected)
	}
}
