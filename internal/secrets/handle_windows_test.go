// SPDX-License-Identifier: AGPL-3.0-or-later
//go:build windows

package secrets

import (
	"os"
	"testing"
)

func TestReadHandleDeleteOnClose(t *testing.T) {
	withTempDataDir(t)

	runDir, err := RunDir("win-run")
	if err != nil {
		t.Fatalf("run dir: %v", err)
	}
	path, err := CreateFile(runDir, "token", []byte("secret"))
	if err != nil {
		t.Fatalf("create file: %v", err)
	}
	handlePath, closeFn, err := ReadHandle(path)
	if err != nil {
		t.Fatalf("read handle: %v", err)
	}
	if closeFn == nil {
		t.Fatalf("expected close function")
	}
	if handlePath == "" {
		t.Fatalf("expected handle path")
	}
	if err := closeFn(); err != nil {
		t.Fatalf("close handle: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("expected path removed on close, stat err=%v", err)
	}
}
