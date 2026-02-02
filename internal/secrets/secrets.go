// SPDX-License-Identifier: AGPL-3.0-or-later
package secrets

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/flowd-org/flowd/internal/paths"
)

const (
	baseDirName = "secrets"
	janitorTTL  = 72 * time.Hour
)

// BaseDir returns the resolved base directory for secret handles.
// This location is intentionally under the flowd data directory.
func BaseDir() string {
	return paths.DataPath(baseDirName)
}

// EnsureBaseDir ensures the base secrets directory exists with restrictive permissions.
func EnsureBaseDir() (string, error) {
	base := BaseDir()
	if base == "" {
		return "", opError("resolve base dir")
	}
	if err := os.MkdirAll(base, 0o700); err != nil {
		return "", wrapErr("mkdir base dir", err)
	}
	return base, nil
}

// RunDir creates a per-run secrets directory with restrictive permissions.
func RunDir(runID string) (string, error) {
	if strings.TrimSpace(runID) == "" {
		return "", opError("create run dir")
	}
	base, err := EnsureBaseDir()
	if err != nil {
		return "", err
	}
	runDir := filepath.Join(base, runID)
	if err := os.MkdirAll(runDir, 0o700); err != nil {
		return "", wrapErr("mkdir run dir", err)
	}
	return runDir, nil
}

// CreateFile writes a secret value to a file under runDir with restrictive permissions.
// The returned path should be treated as sensitive and must not be logged or persisted.
func CreateFile(runDir string, name string, value []byte) (string, error) {
	if strings.TrimSpace(runDir) == "" {
		return "", opError("create secret file")
	}
	if strings.TrimSpace(name) == "" {
		return "", opError("create secret file")
	}
	if err := os.MkdirAll(runDir, 0o700); err != nil {
		return "", wrapErr("mkdir run dir", err)
	}
	path := filepath.Join(runDir, sanitizeName(name))
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return "", wrapErr("open secret file", err)
	}
	_, writeErr := file.Write(value)
	closeErr := file.Close()
	if writeErr != nil {
		return "", wrapErr("write secret file", writeErr)
	}
	if closeErr != nil {
		return "", wrapErr("close secret file", closeErr)
	}
	return path, nil
}

// Janitor removes secret run directories older than the janitor TTL.
// It only operates within the base secrets directory.
func Janitor(now time.Time) (removed int, err error) {
	base, err := EnsureBaseDir()
	if err != nil {
		return 0, err
	}
	entries, err := os.ReadDir(base)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return 0, nil
		}
		return 0, wrapErr("read base dir", err)
	}
	cutoff := now.Add(-janitorTTL)
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		info, infoErr := entry.Info()
		if infoErr != nil {
			continue
		}
		if info.ModTime().After(cutoff) {
			continue
		}
		target := filepath.Join(base, entry.Name())
		if removeErr := os.RemoveAll(target); removeErr != nil {
			return removed, wrapErr("remove stale run dir", removeErr)
		}
		removed++
	}
	return removed, nil
}

func sanitizeName(name string) string {
	if name == "" {
		return "secret"
	}
	var b strings.Builder
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '-' || r == '_':
			b.WriteRune(r)
		default:
			b.WriteRune('_')
		}
	}
	if b.Len() == 0 {
		return "secret"
	}
	return b.String()
}
