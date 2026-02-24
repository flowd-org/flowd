package main

import (
	"fmt"
	"os"
	"strings"

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

	// Run the conformance harness
	fmt.Println("conformance harness starting...")
	fmt.Printf("  flwd binary: %s\n", cfg.FlwdBinary)
	fmt.Printf("  bind: %s\n", cfg.Bind)
	fmt.Printf("  profiles: %v\n", cfg.ULCProfiles)

	// TODO: Implement actual conformance run logic (T-003+)

	// For now, return success
	return harness.ExitOK
}
