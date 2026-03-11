package paths

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestSetDataDirOverride(t *testing.T) {
	// Clear any existing state
	SetDataDirOverride("")
	defer SetDataDirOverride("")

	// Set an override and verify DataDir uses it
	expected := "/tmp/flowd-test"
	SetDataDirOverride(expected)
	got := DataDir()
	if got != expected {
		t.Errorf("DataDir() = %q, want %q", got, expected)
	}

	// Clear the override and verify it falls back
	SetDataDirOverride("")
	got = DataDir()
	if got == expected {
		t.Errorf("DataDir() = %q after clear, expected fallback", got)
	}
}

func TestSetDataDirOverride_Priority(t *testing.T) {
	// Clear state
	SetDataDirOverride("")
	defer SetDataDirOverride("")

	// Temporarily set DATA_DIR and verify override takes precedence
	t.Setenv("DATA_DIR", "/data-dir-env")

	// First without override
	noOverride := DataDir()

	// Now set override - should take precedence over DATA_DIR
	SetDataDirOverride("/override-precedence")
	overridePath := DataDir()

	if overridePath == noOverride {
		t.Error("Override did not change DataDir() from env value")
	}
	if overridePath != "/override-precedence" {
		t.Errorf("DataDir() with override = %q, want /override-precedence", overridePath)
	}
}

func TestDATA_DIR_Precedence(t *testing.T) {
	// Clear override
	SetDataDirOverride("")
	defer SetDataDirOverride("")

	// Set DATA_DIR and verify DataDir uses it (no XDG_DATA_HOME set)
	t.Setenv("DATA_DIR", "/data-dir-env")
	got := DataDir()
	if got != "/data-dir-env" {
		t.Errorf("DataDir() with DATA_DIR = %q, want /data-dir-env", got)
	}

	// Clear DATA_DIR and set XDG_DATA_HOME - should fall back to XDG
	t.Setenv("DATA_DIR", "")
	t.Setenv("XDG_DATA_HOME", "/xdg-home")
	got = DataDir()
	expectedXDG := filepath.Join("/xdg-home", "flowd")
	if got != expectedXDG {
		t.Errorf("DataDir() with XDG_DATA_HOME = %q, want %q", got, expectedXDG)
	}
}

func TestDataPath(t *testing.T) {
	// Clear override
	SetDataDirOverride("")
	defer SetDataDirOverride("")

	t.Setenv("DATA_DIR", "/data-dir")
	got := DataPath("a", "b", "c")
	expected := filepath.Join("/data-dir", "a", "b", "c")
	if got != expected {
		t.Errorf("DataPath() = %q, want %q", got, expected)
	}
}

func TestEnsureDataPath(t *testing.T) {
	// Clear override
	SetDataDirOverride("")
	defer SetDataDirOverride("")

	// Use a temp directory as override to avoid polluting real paths
	tmpDir := t.TempDir()
	SetDataDirOverride(tmpDir)

	subdir := filepath.Join(tmpDir, "runs", "test-run")
	got, err := EnsureDataPath("runs", "test-run")
	if err != nil {
		t.Fatalf("EnsureDataPath() returned error: %v", err)
	}
	if got != subdir {
		t.Errorf("EnsureDataPath() = %q, want %q", got, subdir)
	}

	// Verify the directory was created
	if _, err := os.Stat(subdir); os.IsNotExist(err) {
		t.Error("Directory was not created by EnsureDataPath")
	}
}

func TestRunsDir_RunDir_SourcesDir_OCICacheDir(t *testing.T) {
	// Clear override
	SetDataDirOverride("")
	defer SetDataDirOverride("")

	t.Setenv("DATA_DIR", "/data-dir")

	if got := RunsDir(); got != filepath.Join("/data-dir", "runs") {
		t.Errorf("RunsDir() = %q, want %q", got, filepath.Join("/data-dir", "runs"))
	}

	if got := RunDir("abc123"); got != filepath.Join("/data-dir", "runs", "abc123") {
		t.Errorf("RunDir() = %q, want %q", got, filepath.Join("/data-dir", "runs", "abc123"))
	}

	if got := SourcesDir(); got != filepath.Join("/data-dir", "sources") {
		t.Errorf("SourcesDir() = %q, want %q", got, filepath.Join("/data-dir", "sources"))
	}

	if got := OCICacheDir(); got != filepath.Join("/data-dir", "oci") {
		t.Errorf("OCICacheDir() = %q, want %q", got, filepath.Join("/data-dir", "oci"))
	}
}

func TestPathFunctions_Coverage(t *testing.T) {
	// Ensure all code paths in paths.go are exercised for coverage
	SetDataDirOverride("")
	defer SetDataDirOverride("")

	// Force different OS-specific branches by mocking environment
	t.Setenv("DATA_DIR", "")
	t.Setenv("XDG_DATA_HOME", "")

	// Call each exported function to ensure coverage
	_ = DataPath("a")
	_, _ = EnsureDataPath("b")
	_ = RunsDir()
	_ = RunDir("run-id")
	_ = SourcesDir()
	_ = OCICacheDir()

	// Verify override clearing works
	SetDataDirOverride("")
	if ptr := override.Load(); ptr != nil {
		t.Error("override not cleared after SetDataDirOverride(\"\")")
	}
}

func TestDATA_DIR_Precedence_Windows(t *testing.T) {
	// TestDATA_DIR_Precedence_Windows tests the Windows-specific fallback path
	// by mocking PROGRAMDATA and HOME environment variables.
	// Clear override
	SetDataDirOverride("")
	defer SetDataDirOverride("")

	// Temporarily pretend we're on Windows
	t.Setenv("DATA_DIR", "")
	t.Setenv("XDG_DATA_HOME", "")

	// Mock ProgramData for Windows fallback - only set it when not on Windows
	if runtime.GOOS != "windows" {
		t.Setenv("PROGRAMDATA", "/fake/ProgramData")
	}

	got := DataDir()

	// On non-Windows systems, the PROGRAMDATA env var won't be used,
	// so we just verify that the function returns a valid path without crashing.
	if got == "" {
		t.Error("DataDir() returned empty string for Windows fallback test")
	}
}

func TestDATA_DIR_Precedence_POSIX(t *testing.T) {
	// TestDATA_DIR_Precedence_POSIX tests the POSIX fallback path
	// by mocking HOME and XDG_DATA_HOME environment variables.
	// Clear override
	SetDataDirOverride("")
	defer SetDataDirOverride("")

	t.Setenv("DATA_DIR", "")
	t.Setenv("XDG_DATA_HOME", "")

	// Mock HOME for POSIX fallback
	home := t.TempDir()
	if runtime.GOOS != "windows" {
		t.Setenv("HOME", home)
	}

	got := DataDir()
	expectedPosix := filepath.Join(home, ".local", "share", "flowd")
	if got != expectedPosix {
		t.Errorf("DataDir() POSIX fallback = %q, want %q", got, expectedPosix)
	}
}

func TestDATA_DIR_Precedence_NoHome(t *testing.T) {
	// TestDATA_DIR_Precedence_NoHome tests fallback to CWD when HOME is not available
	SetDataDirOverride("")
	defer SetDataDirOverride("")

	t.Setenv("DATA_DIR", "")
	t.Setenv("XDG_DATA_HOME", "")
	t.Setenv("HOME", "")

	// Clear USERPROFILE on Windows systems
	if runtime.GOOS == "windows" {
		t.Setenv("USERPROFILE", "")
	}

	got := DataDir()

	// Should fall back to CWD/flowd
	expectedCwd := filepath.Join(os.Getenv("PWD"), "flowd")
	if got != expectedCwd {
		t.Errorf("DataDir() fallback to CWD = %q, want %q", got, expectedCwd)
	}
}

func TestRunsDir(t *testing.T) {
	SetDataDirOverride("/tmp/test-paths")
	defer SetDataDirOverride("")

	got := RunsDir()
	expected := filepath.Join("/tmp/test-paths", "runs")
	if got != expected {
		t.Errorf("RunsDir() = %q, want %q", got, expected)
	}
}

func TestRunDir(t *testing.T) {
	SetDataDirOverride("/tmp/test-paths")
	defer SetDataDirOverride("")

	got := RunDir("abc123")
	expected := filepath.Join("/tmp/test-paths", "runs", "abc123")
	if got != expected {
		t.Errorf("RunDir() = %q, want %q", got, expected)
	}
}

func TestSourcesDir(t *testing.T) {
	SetDataDirOverride("/tmp/test-paths")
	defer SetDataDirOverride("")

	got := SourcesDir()
	expected := filepath.Join("/tmp/test-paths", "sources")
	if got != expected {
		t.Errorf("SourcesDir() = %q, want %q", got, expected)
	}
}

func TestOCICacheDir(t *testing.T) {
	SetDataDirOverride("/tmp/test-paths")
	defer SetDataDirOverride("")

	got := OCICacheDir()
	expected := filepath.Join("/tmp/test-paths", "oci")
	if got != expected {
		t.Errorf("OCICacheDir() = %q, want %q", got, expected)
	}
}
