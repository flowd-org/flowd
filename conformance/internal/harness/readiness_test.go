package harness

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
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
	// Create a test server that returns 503 (not ready)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Preload processExitCh with exit error (shell-independent trigger)
	processExitCh := make(chan error, 1)
	processExitCh <- fmt.Errorf("process exited")
	getStderrTail := func() string { return "test stderr" }

	exitCode, err := WaitForReadyWithProcess(ctx, server.URL, "dummy-token", 1*time.Second, processExitCh, getStderrTail)

	if exitCode != ExitInfra {
		t.Errorf("WaitForReadyWithProcess() exitCode = %d, want %d", exitCode, ExitInfra)
	}

	if err == nil {
		t.Error("WaitForReadyWithProcess() expected error after process exit, got nil")
	} else if !strings.Contains(err.Error(), "process exited") {
		t.Errorf("WaitForReadyWithProcess() error = %v, want to contain 'process exited'", err)
	}
}

func TestWaitForReadyWithProcess_UsesSharedExitChannel_NoDuplicateWaitRace(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	sharedExitCh := make(chan error)
	close(sharedExitCh)

	getStderrTail := func() string { return "shared exit channel" }

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	exitCode, err := WaitForReadyWithProcess(ctx, server.URL, "dummy-token", 500*time.Millisecond, sharedExitCh, getStderrTail)
	if exitCode != ExitInfra {
		t.Fatalf("WaitForReadyWithProcess() exitCode = %d, want %d", exitCode, ExitInfra)
	}
	if err == nil || !strings.Contains(err.Error(), "process exited") {
		t.Fatalf("WaitForReadyWithProcess() err = %v, want to contain 'process exited'", err)
	}

	ctx2, cancel2 := context.WithTimeout(context.Background(), time.Second)
	defer cancel2()

	exitCode2, err2 := WaitForReadyWithProcess(ctx2, server.URL, "dummy-token", 500*time.Millisecond, sharedExitCh, getStderrTail)
	if exitCode2 != ExitInfra {
		t.Fatalf("WaitForReadyWithProcess() second call exitCode = %d, want %d", exitCode2, ExitInfra)
	}
	if err2 == nil || !strings.Contains(err2.Error(), "process exited") {
		t.Fatalf("WaitForReadyWithProcess() second call err = %v, want to contain 'process exited'", err2)
	}
}
