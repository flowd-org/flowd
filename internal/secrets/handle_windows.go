// SPDX-License-Identifier: AGPL-3.0-or-later
//go:build windows

package secrets

import (
	"os"

	"golang.org/x/sys/windows"
)

// Handle represents an open secret handle backed by a file descriptor.
// The file is marked delete-on-close to enforce lifecycle semantics.
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

// OpenHandle opens a secret file for read and marks it delete-on-close,
// returning a handle path suitable for passing to a child process. The caller
// must keep the returned handle open for the duration of use.
func OpenHandle(path string) (*Handle, error) {
	if path == "" {
		return nil, opError("open secret handle")
	}
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return nil, opError("open secret handle")
	}
	handle, err := windows.CreateFile(
		p,
		windows.GENERIC_READ,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		nil,
		windows.OPEN_EXISTING,
		windows.FILE_ATTRIBUTE_NORMAL|windows.FILE_FLAG_DELETE_ON_CLOSE,
		0,
	)
	if err != nil {
		return nil, wrapErr("open secret file", err)
	}
	file := os.NewFile(uintptr(handle), path)
	if file == nil {
		_ = windows.CloseHandle(handle)
		return nil, opError("open secret handle")
	}
	return &Handle{Path: path, file: file}, nil
}
