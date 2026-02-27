package harness

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func TestWaitForReadyWithProcess_ExitShortCircuits(t *testing.T) {
	// Create a test server that returns 503 (not ready)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	processExitCh := make(chan error, 1)
	getStderrTail := func() string { return "test stderr" }

	exitCode, err := WaitForReadyWithProcess(ctx, server.URL, "dummy-token", 500*time.Millisecond, processExitCh, getStderrTail)

	// Debug: print actual values
	t.Logf("exitCode = %d (ExitInfra=%d), err = %v", exitCode, ExitInfra, err)
	if err != nil {
		t.Logf("error message: %q", err.Error())
	}

	// Server returns 503, so we expect timeout/failure
	if exitCode != ExitInfra {
		t.Errorf("WaitForReadyWithProcess() exitCode = %d, want %d", exitCode, ExitInfra)
	}

	if err == nil {
		t.Error("WaitForReadyWithProcess() expected error (failure), got nil")
	} else if !strings.Contains(err.Error(), "failed to wait for") {
		t.Errorf("WaitForReadyWithProcess() error = %v, want to contain 'failed to wait for'", err)
	}

	// Signal process exit before retry
	processExitCh <- fmt.Errorf("process exited")

	// Retry with process already exited - should short-circuit immediately
	ctx2, cancel2 := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel2()

	// Create a new channel for the second call since we need to signal exit first
	exitCh2 := make(chan error, 1)
	exitCh2 <- fmt.Errorf("process exited")

	exitCode2, err2 := WaitForReadyWithProcess(ctx2, server.URL, "dummy-token", 500*time.Millisecond, exitCh2, getStderrTail)

	t.Logf("exitCode2 = %d (ExitInfra=%d), err2 = %v", exitCode2, ExitInfra, err2)
	if err2 != nil {
		t.Logf("error2 message: %q", err2.Error())
	}

	if exitCode2 != ExitInfra {
		t.Errorf("WaitForReadyWithProcess() after exit exitCode = %d, want %d", exitCode2, ExitInfra)
	}

	if err2 == nil {
		t.Error("WaitForReadyWithProcess() after exit expected error, got nil")
	} else if !strings.Contains(err2.Error(), "process exited") {
		t.Errorf("WaitForReadyWithProcess() after exit error = %v, want to contain 'process exited'", err2)
	}
}

func TestWaitForReadyWithProcess_RedactsStderrTail(t *testing.T) {
	// Create a test server that never responds
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Do nothing - request will hang
	}))
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	tokenInStderr := "secret-token-12345"
	processExitCh := make(chan error, 1)

	exitCode, err := WaitForReadyWithProcess(ctx, server.URL, tokenInStderr, 200*time.Millisecond, processExitCh, func() string {
		return "Some error with token: " + tokenInStderr + " and more text"
	})

	if exitCode != ExitInfra {
		t.Errorf("WaitForReadyWithProcess() exitCode = %d, want %d", exitCode, ExitInfra)
	}

	if err == nil {
		t.Error("WaitForReadyWithProcess() expected error, got nil")
	} else {
		errStr := err.Error()
		// Token should be redacted
		if strings.Contains(errStr, tokenInStderr) {
			t.Errorf("WaitForReadyWithProcess() error contains unredacted token: %s", errStr)
		}
		// Should contain [REDACTED]
		if !strings.Contains(errStr, "[REDACTED]") {
			t.Errorf("WaitForReadyWithProcess() error = %q, want to contain '[REDACTED]'", errStr)
		}
		// Wrapper text should remain
		if !strings.Contains(errStr, "failed to wait for startup") && !strings.Contains(errStr, "failed to wait for readiness") {
			t.Errorf("WaitForReadyWithProcess() error = %q, want to contain 'failed to wait for'", errStr)
		}
	}
}

func TestWaitForReadyWithProcess_ProcessExitsBeforeEndpoint(t *testing.T) {
	// Start a real process that exits immediately
	cmd := exec.Command("sleep", "0.1") // Exits after 0.1 seconds

	stderrBuf := &strings.Builder{}
	cmd.Stderr = io.MultiWriter(stderrBuf)

	if err := cmd.Start(); err != nil {
		t.Fatalf("Failed to start sleep process: %v", err)
	}

	// Create a channel that will receive when the process exits
	processExitCh := make(chan error, 1)
	go func() {
		_ = cmd.Wait()
		processExitCh <- fmt.Errorf("process exited")
	}()

	// Give process time to exit
	time.Sleep(200 * time.Millisecond)

	// Now try readiness check - should short-circuit on process exit
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	getStderrTail := func() string { return stderrBuf.String() }

	exitCode, err := WaitForReadyWithProcess(ctx, "http://127.0.0.1:9999", "dummy-token", 1*time.Second, processExitCh, getStderrTail)

	if exitCode != ExitInfra {
		t.Errorf("WaitForReadyWithProcess() exitCode = %d, want %d", exitCode, ExitInfra)
	}

	if err == nil {
		t.Error("WaitForReadyWithProcess() expected error after process exit, got nil")
	} else if !strings.Contains(err.Error(), "process exited") {
		t.Errorf("WaitForReadyWithProcess() error = %v, want to contain 'process exited'", err)
	}
}
