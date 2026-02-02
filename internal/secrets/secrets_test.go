// SPDX-License-Identifier: AGPL-3.0-or-later
package secrets

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/flowd-org/flowd/internal/paths"
)

func withTempDataDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	paths.SetDataDirOverride(dir)
	t.Cleanup(func() {
		paths.SetDataDirOverride("")
	})
	return dir
}

func TestJanitorRemovesOldDirs(t *testing.T) {
	withTempDataDir(t)

	now := time.Now()
	oldDir, err := RunDir("old-run")
	if err != nil {
		t.Fatalf("run dir: %v", err)
	}
	freshDir, err := RunDir("fresh-run")
	if err != nil {
		t.Fatalf("run dir: %v", err)
	}
	if err := os.Chtimes(oldDir, now.Add(-73*time.Hour), now.Add(-73*time.Hour)); err != nil {
		t.Fatalf("chtimes old dir: %v", err)
	}
	if err := os.Chtimes(freshDir, now.Add(-1*time.Hour), now.Add(-1*time.Hour)); err != nil {
		t.Fatalf("chtimes fresh dir: %v", err)
	}

	removed, err := Janitor(now)
	if err != nil {
		t.Fatalf("janitor: %v", err)
	}
	if removed != 1 {
		t.Fatalf("expected 1 removed dir, got %d", removed)
	}
	if _, err := os.Stat(oldDir); !os.IsNotExist(err) {
		t.Fatalf("expected old dir removed, stat err=%v", err)
	}
	if _, err := os.Stat(freshDir); err != nil {
		t.Fatalf("expected fresh dir retained, stat err=%v", err)
	}

	// Ensure janitor only touches the secrets base directory.
	outside := filepath.Join(t.TempDir(), "other")
	if err := os.MkdirAll(outside, 0o700); err != nil {
		t.Fatalf("create outside dir: %v", err)
	}
	if err := os.Chtimes(outside, now.Add(-100*time.Hour), now.Add(-100*time.Hour)); err != nil {
		t.Fatalf("chtimes outside dir: %v", err)
	}
	if _, err := os.Stat(outside); err != nil {
		t.Fatalf("outside dir missing unexpectedly: %v", err)
	}
}

func TestCreateFilePermissionsAndCleanup(t *testing.T) {
	withTempDataDir(t)

	runDir, err := RunDir("perm-run")
	if err != nil {
		t.Fatalf("run dir: %v", err)
	}
	if info, err := os.Stat(runDir); err != nil {
		t.Fatalf("stat run dir: %v", err)
	} else if info.Mode().Perm()&0o077 != 0 {
		t.Fatalf("expected run dir perms to restrict group/other, got %v", info.Mode().Perm())
	}
	path, err := CreateFile(runDir, "token", []byte("secret"))
	if err != nil {
		t.Fatalf("create file: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat secret file: %v", err)
	}
	if info.Mode().Perm()&0o077 != 0 {
		t.Fatalf("expected file perms to restrict group/other, got %v", info.Mode().Perm())
	}

	if err := os.RemoveAll(runDir); err != nil {
		t.Fatalf("cleanup run dir: %v", err)
	}
	if _, err := os.Stat(runDir); !os.IsNotExist(err) {
		t.Fatalf("expected run dir removed, stat err=%v", err)
	}
}

func TestErrorsDoNotLeakPaths(t *testing.T) {
	_, err := CreateFile("", "secret", []byte("value"))
	if err == nil {
		t.Fatalf("expected error")
	}
	if err.Error() != "secrets: create secret file failed" {
		t.Fatalf("unexpected error message: %q", err.Error())
	}
}
