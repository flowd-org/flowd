package harness

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestStartFlwd_BootstrapRootPreflight(t *testing.T) {
	tests := []struct {
		name            string
		setupRunRoot    func(t *testing.T) string
		wantExitCode    int
		wantErrContains string
	}{
		{
			name: "missing flwd binary returns ExitInfra",
			setupRunRoot: func(t *testing.T) string {
				tmpDir, err := os.MkdirTemp("", "flwd-process-test-*")
				if err != nil {
					t.Fatalf("Failed to create temp dir: %v", err)
				}
				t.Cleanup(func() { os.RemoveAll(tmpDir) })
				return tmpDir
			},
			wantExitCode:    ExitInfra,
			wantErrContains: "flwd binary not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runRoot := tt.setupRunRoot(t)

			cfg := Config{
				FlwdBinary: "/nonexistent/flwd",
			}

			_, exitCode, err := StartFlwd(t.Context(), cfg, runRoot)

			if err == nil {
				t.Errorf("StartFlwd() expected error, got nil")
			}

			if exitCode != tt.wantExitCode {
				t.Errorf("StartFlwd() exitCode = %d, want %d", exitCode, tt.wantExitCode)
			}

			if tt.wantErrContains != "" && err != nil {
				errStr := err.Error()
				if !contains(errStr, tt.wantErrContains) {
					t.Errorf("StartFlwd() error = %q, want to contain %q", errStr, tt.wantErrContains)
				}
			}
		})
	}
}

func TestStartFlwd_BootstrapRootNotReadable(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "flwd-process-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create a bootstrap root that exists but is unreadable
	bootstrapRoot := filepath.Join(tmpDir, "scripts", "fixtures", "tree-v1")
	if err := os.MkdirAll(bootstrapRoot, 0o755); err != nil {
		t.Fatalf("Failed to create bootstrap directory: %v", err)
	}
	// Make the directory unreadable
	if err := os.Chmod(bootstrapRoot, 0o000); err != nil {
		t.Fatalf("Cannot make directory unreadable on this system: %v", err)
	}

	// Create an executable stub for flwdBinary so StartFlwd passes binary checks
	flwdStub := filepath.Join(tmpDir, "flwd-stub")
	if err := os.WriteFile(flwdStub, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatalf("Failed to create flwd stub: %v", err)
	}

	cfg := Config{
		FlwdBinary: flwdStub,
	}

	_, exitCode, err := StartFlwd(t.Context(), cfg, tmpDir)

	if err == nil {
		t.Errorf("StartFlwd() expected error for unreadable bootstrap root, got nil")
	}

	if exitCode != ExitInfra {
		t.Errorf("StartFlwd() exitCode = %d, want %d", exitCode, ExitInfra)
	}

	errStr := err.Error()
	if !contains(errStr, "bootstrap root not readable") && !contains(errStr, "cannot stat bootstrap root") {
		t.Errorf("StartFlwd() error = %q, want to contain 'bootstrap root not readable' or 'cannot stat bootstrap root'", errStr)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsAt(s, substr))
}

func containsAt(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func TestFlwdProcess_StopUsesSingleWaitOwner(t *testing.T) {
	cmd := exec.Command("sh", "-c", "trap 'exit 0' INT; while :; do sleep 0.05; done")
	cmd.Stdout = &bytes.Buffer{}
	cmd.Stderr = &bytes.Buffer{}

	if err := cmd.Start(); err != nil {
		t.Fatalf("cmd.Start() failed: %v", err)
	}

	processExitCh := make(chan error, 1)
	waitDone := make(chan struct{})

	fp := &FlwdProcess{
		Cmd:           cmd,
		processExitCh: processExitCh,
		waitDone:      waitDone,
	}

	go func() {
		err := cmd.Wait()
		fp.waitErrMu.Lock()
		fp.waitErr = err
		fp.waitErrMu.Unlock()
		processExitCh <- err
		close(processExitCh)
		close(waitDone)
	}()

	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()

	err := fp.Stop(ctx)
	if err != nil {
		if strings.Contains(err.Error(), "Wait was already called") || strings.Contains(err.Error(), "wait was already called") {
			t.Fatalf("Stop() duplicate wait error: %v", err)
		}
		if !strings.Contains(err.Error(), "interrupt") {
			t.Fatalf("Stop() error = %v, want nil or interrupt", err)
		}
	}

	ctx2, cancel2 := context.WithTimeout(t.Context(), time.Second)
	defer cancel2()

	secondDone := make(chan error, 1)
	go func() {
		secondDone <- fp.Cleanup(ctx2)
	}()

	select {
	case err2 := <-secondDone:
		if err2 != nil {
			if strings.Contains(err2.Error(), "Wait was already called") || strings.Contains(err2.Error(), "wait was already called") {
				t.Fatalf("Cleanup() duplicate wait error: %v", err2)
			}
			if !strings.Contains(err2.Error(), "interrupt") {
				t.Fatalf("Cleanup() error = %v, want nil or interrupt", err2)
			}
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("Cleanup() blocked unexpectedly")
	}
}
