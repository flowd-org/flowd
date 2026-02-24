package harness

import (
	"flag"
	"fmt"
	"os"
	"strings"
	"time"
)

// Config holds the parsed configuration for the conformance harness.
type Config struct {
	FlwdBinary      string
	Token           string
	Bind            string
	FlwdProfile     string
	ULCProfiles     []string
	Timeout         time.Duration
	ScenarioTimeout time.Duration
	ReportJSON      string
	Verbose         bool
}

// ParseConfig parses command-line arguments and environment variables to produce a Config.
// Returns (config, exitCode, error). If exitCode != 0, the caller should exit with that code.
func ParseConfig(args []string, env map[string]string) (Config, int, error) {
	fs := flag.NewFlagSet("conformance", flag.ContinueOnError)

	var (
		flwdBinary      string
		token           string
		bind            string
		flwdProfile     string
		ulcProfilesStr  string
		timeout         time.Duration
		scenarioTimeout time.Duration
		reportJSON      string
		verbose         bool
	)

	fs.StringVar(&flwdBinary, "flwd-binary", "", "Path to the flwd binary to test (required)")
	fs.StringVar(&token, "token", "", "flowd API token (overrides FLWD_TOKEN env var)")
	fs.StringVar(&bind, "bind", "127.0.0.1:8080", "Bind address for the flwd server")
	fs.StringVar(&flwdProfile, "flwd-profile", "", "flwd profile to use")
	fs.StringVar(&ulcProfilesStr, "ulc-profiles", "bash,pwsh", "Comma-separated list of ULC profiles to test")
	fs.DurationVar(&timeout, "timeout", 5*time.Minute, "Overall timeout for the conformance run")
	fs.DurationVar(&scenarioTimeout, "scenario-timeout", 2*time.Minute, "Timeout per scenario")
	fs.StringVar(&reportJSON, "report-json", "", "Path to write JSON report")
	fs.BoolVar(&verbose, "verbose", false, "Enable verbose logging")

	// Set our own error handling to avoid flag.ExitOnError
	fs.SetOutput(os.Stderr)

	// Parse arguments
	if err := fs.Parse(args); err != nil {
		return Config{}, ExitUsage, fmt.Errorf("failed to parse arguments: %w", err)
	}

	// Validate required flags
	if flwdBinary == "" {
		return Config{}, ExitUsage, fmt.Errorf("--flwd-binary is required")
	}

	// Token sourcing: flag wins over environment
	if token == "" {
		token = env["FLWD_TOKEN"]
	}

	// If still no token, fail fast with exit code 2
	if token == "" {
		return Config{}, ExitUsage, fmt.Errorf("missing token (set --token or FLWD_TOKEN)")
	}

	// Parse ULC profiles
	var ulcProfiles []string
	if ulcProfilesStr != "" {
		ulcProfiles = splitCSV(ulcProfilesStr)
	}

	cfg := Config{
		FlwdBinary:      flwdBinary,
		Token:           token,
		Bind:            bind,
		FlwdProfile:     flwdProfile,
		ULCProfiles:     ulcProfiles,
		Timeout:         timeout,
		ScenarioTimeout: scenarioTimeout,
		ReportJSON:      reportJSON,
		Verbose:         verbose,
	}

	return cfg, ExitOK, nil
}

// splitCSV splits a comma-separated string into a slice, trimming whitespace.
func splitCSV(s string) []string {
	if s == "" {
		return nil
	}

	var parts []string
	for _, p := range strings.Split(s, ",") {
		p = strings.TrimSpace(p)
		if p != "" {
			parts = append(parts, p)
		}
	}
	return parts
}
