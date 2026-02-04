// SPDX-License-Identifier: AGPL-3.0-or-later
//go:build !windows

package secrets

import (
	"fmt"
	"os"
	"runtime"
)

// Handle represents an open secret handle backed by a file descriptor.
// The original on-disk path is unlinked immediately after opening.
// Close must be called when the handle is no longer needed.
type Handle struct {
	Path string
	file *os.File
}

// Close closes the underlying file descriptor.
func (h *Handle) Close() error {
	if h == nil || h.file == nil {
		return nil
	}
	return h.file.Close()
}

// OpenHandle opens a secret file for read, unlinks it immediately, and returns
// an fd-backed path suitable for passing to a child process. The caller must
// keep the returned handle open for the duration of use.
func OpenHandle(path string) (*Handle, error) {
	if path == "" {
		return nil, opError("open secret handle")
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, wrapErr("open secret file", err)
	}
	if err := os.Remove(path); err != nil {
		_ = file.Close()
		return nil, wrapErr("unlink secret file", err)
	}
	fdPath, err := fdPathFor(file)
	if err != nil {
		_ = file.Close()
		return nil, err
	}
	return &Handle{Path: fdPath, file: file}, nil
}

func fdPathFor(file *os.File) (string, error) {
	if file == nil {
		return "", opError("resolve fd path")
	}
	fd := file.Fd()
	for _, candidate := range fdPathCandidates(fd) {
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
	}
	return "", opError("resolve fd path")
}

func fdPathCandidates(fd uintptr) []string {
	switch runtime.GOOS {
	case "linux":
		return []string{
			fmt.Sprintf("/proc/self/fd/%d", fd),
			fmt.Sprintf("/dev/fd/%d", fd),
		}
	case "darwin":
		return []string{
			fmt.Sprintf("/dev/fd/%d", fd),
			fmt.Sprintf("/proc/self/fd/%d", fd),
		}
	default:
		return []string{
			fmt.Sprintf("/dev/fd/%d", fd),
			fmt.Sprintf("/proc/self/fd/%d", fd),
		}
	}
}
