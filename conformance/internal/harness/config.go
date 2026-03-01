package harness

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
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
	fs.StringVar(&bind, "bind", "", "Bind address for flwd (empty = auto)")
	fs.StringVar(&flwdProfile, "flwd-profile", "", "flwd profile to use")
	fs.StringVar(&ulcProfilesStr, "ulc-profiles", "ulc.shell.bash,ulc.shell.pwsh", "Comma-separated list of ULC profiles to test (aliases: bash, pwsh)")
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

	// Validate scenario-timeout is positive
	if scenarioTimeout <= 0 {
		return Config{}, ExitUsage, fmt.Errorf("--scenario-timeout must be > 0")
	}

	// Validate required flags
	if flwdBinary == "" {
		return Config{}, ExitUsage, fmt.Errorf("--flwd-binary is required")
	}

	// Canonicalize flwdBinary to absolute path to ensure startup is independent
	// of the caller's current working directory.
	resolved, err := filepath.Abs(flwdBinary)
	if err != nil {
		return Config{}, ExitUsage, fmt.Errorf("failed to resolve --flwd-binary: %w", err)
	}
	flwdBinary = filepath.Clean(resolved)

	// Token sourcing: flag wins over environment
	if token == "" {
		token = env["FLWD_TOKEN"]
	}

	// If still no token, fail fast with exit code 2
	if token == "" {
		return Config{}, ExitUsage, fmt.Errorf("missing token (set --token or FLWD_TOKEN)")
	}

	// Parse ULC profiles with alias normalization
	var ulcProfiles []string
	if ulcProfilesStr != "" {
		rawProfiles := splitCSV(ulcProfilesStr)
		for _, p := range rawProfiles {
			normalized := normalizeProfile(p)
			if normalized != "" {
				ulcProfiles = append(ulcProfiles, normalized)
			}
		}
	}

	cfg := Config{
		FlwdBinary:      flwdBinary,
		Token:           token,
		Bind:            strings.TrimSpace(bind),
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

// normalizeProfile converts ULC profile aliases to canonical identifiers.
// Supported aliases: "bash" -> "ulc.shell.bash", "pwsh" -> "ulc.shell.pwsh".
// Canonical identifiers are passed through unchanged.
func normalizeProfile(p string) string {
	switch p {
	case "bash":
		return "ulc.shell.bash"
	case "pwsh":
		return "ulc.shell.pwsh"
	default:
		// Pass through canonical identifiers and unknown profiles unchanged
		return p
	}
}
