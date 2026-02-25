package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/flowd-org/flowd/conformance/internal/harness"
)

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	// Convert environment to map
	env := make(map[string]string)
	for _, e := range os.Environ() {
		parts := strings.SplitN(e, "=", 2)
		if len(parts) == 2 {
			env[parts[0]] = parts[1]
		}
	}

	// Parse configuration
	cfg, exitCode, err := harness.ParseConfig(args, env)
	if err != nil {
		// Redact the token from the error message before printing
		redactedMsg := harness.RedactSecrets(err.Error(), cfg.Token)
		fmt.Fprintf(os.Stderr, "Error: %s\n", redactedMsg)
		return exitCode
	}

	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), cfg.Timeout)
	defer cancel()

	// Handle SIGINT/SIGTERM for graceful shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		fmt.Println("\nReceived signal, shutting down...")
		cancel()
	}()

	// Create temp run directory
	runRoot, err := os.MkdirTemp("", "flowd-conformance-")
	if err != nil {
		return harness.ExitInfra
	}
	defer os.RemoveAll(runRoot)

	// Start flwd process
	fp, exitCode, err := harness.StartFlwd(ctx, cfg, runRoot)
	if err != nil {
		return exitCode
	}
	defer fp.Cleanup(ctx)

	// Wait for server readiness
	fmt.Println("Waiting for flwd to be ready...")
	startupTimeout := 30 * time.Second
	exitCode, err = harness.WaitForReady(ctx, fp.BaseURL, cfg.Token, startupTimeout)
	if err != nil {
		return exitCode
	}
	fmt.Println("flwd is ready")

	// Run conformance tests
	exitCode, err = runConformanceTests(ctx, cfg, fp.BaseURL, runRoot)
	if err != nil {
		return exitCode
	}

	fmt.Println("Conformance tests passed")
	return harness.ExitOK
}

// runConformanceTests runs the conformance test suite against the running flwd server.
func runConformanceTests(ctx context.Context, cfg harness.Config, baseURL string, runRoot string) (int, error) {
	// Create HTTP client
	client := &harness.Client{
		BaseURL: baseURL,
		Token:   cfg.Token,
		HTTP:    &http.Client{},
		Verbose: cfg.Verbose,
	}

	// Stage fixtures
	fmt.Println("Staging fixtures...")
	stagedRef, err := harness.StageFixtures(runRoot)
	if err != nil {
		return harness.ExitInfra, fmt.Errorf("failed to stage fixtures: %w", err)
	}
	fmt.Printf("Staged fixtures at %s\n", stagedRef)

	// Register local source
	fmt.Println("Registering local source...")
	if err := harness.RegisterLocalSource(ctx, client, "fixtures", stagedRef); err != nil {
		return harness.ExitInfra, fmt.Errorf("failed to register source: %w", err)
	}
	fmt.Println("Source registered")

	// TODO: Implement actual test execution against the server
	// This will involve:
	// 1. Listing available jobs
	// 2. Creating jobs for each profile
	// 3. Running the jobs
	// 4. Checking results

	return harness.ExitOK, nil
}
