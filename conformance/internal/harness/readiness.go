package harness

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// WaitForReady polls /startupz then /readyz endpoints until the server is ready
// or the context times out. Returns ExitOK on success, ExitInfra on failure.
func WaitForReady(ctx context.Context, baseURL string, token string, startupTimeout time.Duration) (int, error) {
	client := &http.Client{
		Timeout: 5 * time.Second,
	}

	// Create a context with the overall timeout
	ctx, cancel := context.WithTimeout(ctx, startupTimeout)
	defer cancel()

	// Poll /startupz until it returns 200
	if err := pollEndpoint(ctx, client, baseURL, "/startupz", token, nil, nil); err != nil {
		return ExitInfra, fmt.Errorf("failed to wait for startup: %w", err)
	}

	// Poll /readyz until it returns 200
	if err := pollEndpoint(ctx, client, baseURL, "/readyz", token, nil, nil); err != nil {
		return ExitInfra, fmt.Errorf("failed to wait for readiness: %w", err)
	}

	return ExitOK, nil
}

// WaitForReadyWithProcess polls /startupz then /readyz endpoints until the server is ready
// or the context times out. Returns ExitOK on success, ExitInfra on failure.
// If processExitCh is non-nil, readiness polling short-circuits when the process exits,
// using getStderrTail for contextual error messages (called at failure time).
func WaitForReadyWithProcess(ctx context.Context, baseURL string, token string, startupTimeout time.Duration, processExitCh <-chan error, getStderrTail func() string) (int, error) {
	client := &http.Client{
		Timeout: 5 * time.Second,
	}

	// Create a context with the overall timeout
	ctx, cancel := context.WithTimeout(ctx, startupTimeout)
	defer cancel()

	// Poll /startupz until it returns 200
	if err := pollEndpoint(ctx, client, baseURL, "/startupz", token, processExitCh, getStderrTail); err != nil {
		return ExitInfra, fmt.Errorf("failed to wait for startup: %w", err)
	}

	// Poll /readyz until it returns 200
	if err := pollEndpoint(ctx, client, baseURL, "/readyz", token, processExitCh, getStderrTail); err != nil {
		return ExitInfra, fmt.Errorf("failed to wait for readiness: %w", err)
	}

	return ExitOK, nil
}

// pollEndpoint polls a given endpoint until it returns 200 or the context times out.
// If processExitCh is non-nil, polling short-circuits when the process exits.
func pollEndpoint(ctx context.Context, client *http.Client, baseURL, path, token string, processExitCh <-chan error, getStderrTail func() string) error {
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			// Context timeout - include stderr tail if available (redacted)
			var errStr string
			if getStderrTail != nil {
				stderrTail := getStderrTail()
				if stderrTail != "" {
					errStr = fmt.Sprintf("timeout waiting for %s: %v; stderr: %s", path, ctx.Err(), RedactStderrTail(stderrTail, token))
				} else {
					errStr = fmt.Sprintf("timeout waiting for %s: %v", path, ctx.Err())
				}
			} else {
				errStr = fmt.Sprintf("timeout waiting for %s: %v", path, ctx.Err())
			}
			return fmt.Errorf("%s", errStr)
		case _, ok := <-processExitCh:
			if ok {
				// Process exited
				if getStderrTail != nil {
					stderrTail := getStderrTail()
					if stderrTail != "" {
						return fmt.Errorf("process exited before %s: %w", path, fmt.Errorf("stderr: %s", RedactStderrTail(stderrTail, token)))
					}
				}
				return fmt.Errorf("process exited before %s", path)
			}
			// Channel closed (ok=false) - process already exited
			if getStderrTail != nil {
				stderrTail := getStderrTail()
				if stderrTail != "" {
					return fmt.Errorf("process exited before %s: %w", path, fmt.Errorf("stderr: %s", RedactStderrTail(stderrTail, token)))
				}
			}
			return fmt.Errorf("process exited before %s", path)
		case <-ticker.C:
			if err := checkEndpoint(ctx, client, baseURL, path, token); err == nil {
				return nil
			}
			// Continue polling on error
		}
	}
}

// checkEndpoint makes a single request to the endpoint and returns nil if it succeeds.
func checkEndpoint(ctx context.Context, client *http.Client, baseURL, path, token string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+path, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("connection failed: %w", err)
	}
	defer resp.Body.Close()

	// Read body for potential error context (redacted)
	_, _ = io.Copy(io.Discard, resp.Body)

	// Accept 200 as success
	if resp.StatusCode == http.StatusOK {
		return nil
	}

	return fmt.Errorf("unexpected status: %d", resp.StatusCode)
}

// RedactStderrTail extracts and redacts a small tail of stderr for error messages.
func RedactStderrTail(stderr, token string) string {
	lines := strings.Split(stderr, "\n")
	if len(lines) <= 5 {
		return RedactSecrets(stderr, token)
	}
	tail := lines[len(lines)-5:]
	return RedactSecrets(strings.Join(tail, "\n"), token)
}
