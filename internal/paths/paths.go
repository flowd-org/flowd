// SPDX-License-Identifier: AGPL-3.0-or-later

// Package paths centralises flowd data-directory resolution.
package paths

import (
	"os"
	"path/filepath"
	"runtime"
	"sync/atomic"
)

const (
	appDirName      = "flowd"
	envDataDir      = "DATA_DIR"
	envXDGDataHome  = "XDG_DATA_HOME"
	envProgramData  = "PROGRAMDATA"
	windowsVendor   = "Flowd"
	windowsDataLeaf = "data"
)

var override atomic.Pointer[string]

// SetDataDirOverride allows callers (e.g. config loader) to pin the data
// directory to an explicit location. Passing an empty string clears the override.
func SetDataDirOverride(dir string) {
	if dir == "" {
		override.Store(nil)
		return
	}
	clean := filepath.Clean(dir)
	override.Store(&clean)
}

// DataDir returns the absolute directory flowd should use for persistence.
// Order of precedence:
//  1. Explicit override provided via SetDataDirOverride.
//  2. DATA_DIR environment variable (flowd appended automatically).
//  3. Platform defaults:
//     * POSIX: $XDG_DATA_HOME/flowd, or ~/.local/share/flowd
//     * Windows: %ProgramData%\Flowd\data
//  4. Fallback: current working directory ./flowd (mainly for constrained envs)
func DataDir() string {
	if ptr := override.Load(); ptr != nil && *ptr != "" {
		return *ptr
	}

	if dir := os.Getenv(envDataDir); dir != "" {
		return filepath.Clean(dir)
	}

	if runtime.GOOS == "windows" {
		if base := os.Getenv(envProgramData); base != "" {
			return filepath.Join(base, windowsVendor, windowsDataLeaf)
		}
		if home, err := os.UserHomeDir(); err == nil && home != "" {
			return filepath.Join(home, "AppData", "Local", windowsVendor, windowsDataLeaf)
		}
	}

	if xdg := os.Getenv(envXDGDataHome); xdg != "" {
		return filepath.Join(xdg, appDirName)
	}

	if home, err := os.UserHomeDir(); err == nil && home != "" {
		return filepath.Join(home, ".local", "share", appDirName)
	}

	if cwd, err := os.Getwd(); err == nil && cwd != "" {
		return filepath.Join(cwd, appDirName)
	}

	// As an absolute last resort fall back to the OS temp dir.
	return filepath.Join(os.TempDir(), appDirName)
}

// DataPath joins the flowd data directory with the supplied path elements.
func DataPath(elem ...string) string {
	parts := append([]string{DataDir()}, elem...)
	return filepath.Join(parts...)
}

// EnsureDataPath ensures that the directory composed of data dir + elem exists.
// It returns the created/resolved path.
func EnsureDataPath(elem ...string) (string, error) {
	path := DataPath(elem...)
	if err := os.MkdirAll(path, 0o700); err != nil {
		return "", err
	}
	return path, nil
}

// RunsDir returns the root directory for run artifacts.
func RunsDir() string {
	return DataPath("runs")
}

// RunDir returns the artifact directory for a specific run.
func RunDir(runID string) string {
	return DataPath("runs", runID)
}

// SourcesDir returns the checkout cache for sources.
func SourcesDir() string {
	return DataPath("sources")
}

// OCICacheDir returns the directory for cached OCI artifacts.
func OCICacheDir() string {
	return DataPath("oci")
}
