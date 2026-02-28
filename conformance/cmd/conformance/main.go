package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/flowd-org/flowd/conformance/internal/harness"
	"github.com/flowd-org/flowd/conformance/internal/report"
	"github.com/flowd-org/flowd/conformance/internal/scenarios"
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
		emitInfraErr(cfg, "failed to create run directory: %v", err)
		return harness.ExitInfra
	}
	defer os.RemoveAll(runRoot)

	// Start flwd process
	fp, exitCode, err := harness.StartFlwd(ctx, cfg, runRoot)
	if err != nil {
		emitInfraErr(cfg, "failed to start flwd: %v", err)
		return exitCode
	}
	defer fp.Cleanup(ctx)

	// Wait for server readiness with process-exit awareness
	fmt.Println("Waiting for flwd to be ready...")
	startupTimeout := 30 * time.Second

	// Set up process exit channel
	waitErrCh := make(chan error, 1)
	go func() {
		waitErrCh <- fp.Cmd.Wait()
		close(waitErrCh)
	}()

	exitCode, err = harness.WaitForReadyWithProcess(ctx, fp.BaseURL, cfg.Token, startupTimeout, waitErrCh, func() string { return fp.Stderr.String() })
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: readiness check failed: %s\n", harness.RedactSecrets(err.Error(), cfg.Token))
		return exitCode
	}
	fmt.Println("flwd is ready")

	// Run conformance tests
	exitCode, err = runConformanceTests(ctx, cfg, fp.BaseURL, runRoot)
	if err != nil || exitCode != harness.ExitOK {
		return exitCode
	}

	fmt.Println("Conformance tests passed")
	return harness.ExitOK
}

// writeInfraFailureReport writes a minimal JSON failure report for infrastructure errors.
func writeInfraFailureReport(path string, reason string) error {
	report := map[string]any{
		"status":   "failed",
		"reason":   reason,
		"exitCode": harness.ExitInfra,
	}
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal report: %w", err)
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("failed to write JSON file: %w", err)
	}
	return nil
}

// emitInfraErr prints a redaction-safe infrastructure error to stderr and writes a JSON report if requested.
func emitInfraErr(cfg harness.Config, format string, err error) {
	msg := fmt.Sprintf(format, harness.RedactSecrets(err.Error(), cfg.Token))
	fmt.Fprintf(os.Stderr, "Error: %s\n", msg)
	if cfg.ReportJSON != "" {
		redactedMsg := harness.RedactSecrets(msg, cfg.Token)
		if writeErr := writeInfraFailureReport(cfg.ReportJSON, redactedMsg); writeErr != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to write JSON report: %v\n", writeErr)
		}
	}
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
		emitInfraErr(cfg, "failed to stage fixtures: %v", err)
		return harness.ExitInfra, fmt.Errorf("failed to stage fixtures: %w", err)
	}
	fmt.Printf("Staged fixtures at %s\n", stagedRef)

	// Register local source
	fmt.Println("Registering local source...")
	if err := harness.RegisterLocalSource(ctx, client, scenarios.FixtureSourceName, stagedRef); err != nil {
		emitInfraErr(cfg, "failed to register source: %v", err)
		return harness.ExitInfra, fmt.Errorf("failed to register source: %w", err)
	}
	fmt.Println("Source registered")

	// Build scenarios environment
	env := scenarios.Env{
		BaseURL:         baseURL,
		Token:           cfg.Token,
		HTTPClient:      client,
		FlwdProcess:     nil,
		ScenarioTimeout: cfg.ScenarioTimeout,
		Verbose:         cfg.Verbose,
	}

	// Select scenario list and profiles
	scenarioList := scenarios.All()
	profiles := cfg.ULCProfiles
	if len(profiles) == 0 {
		profiles = scenarios.DefaultProfiles()
	}

	// Run the conformance test suite
	fmt.Println("Running conformance test suite...")
	r := scenarios.RunSuite(ctx, env, scenarioList, profiles)

	// Print stable one-line summary
	fmt.Println(report.FormatSummary(r))

	// Print failure details for failed scenarios (bounded output)
	if r.FailedCount > 0 {
		fmt.Println("\nFailed scenarios:")
		failedPrinted := 0
		for _, result := range r.Results {
			if !result.Passed {
				// Limit to first 10 failures to avoid overwhelming output
				if failedPrinted >= 10 {
					fmt.Printf("  ... and %d more\n", r.FailedCount-10)
					break
				}
				fmt.Println(report.FormatFailureBlock(result))
				failedPrinted++
			}
		}
	}

	// Write JSON report if requested
	if cfg.ReportJSON != "" {
		if err := report.WriteJSON(cfg.ReportJSON, r); err != nil {
			emitInfraErr(cfg, "failed to write JSON report: %v", err)
			return harness.ExitInfra, fmt.Errorf("failed to write JSON report: %w", err)
		}
		fmt.Printf("JSON report written to %s\n", cfg.ReportJSON)
	}

	// Determine exit code based on results
	if r.FailedCount > 0 {
		return harness.ExitScenarioFail, nil
	}

	return harness.ExitOK, nil
}
