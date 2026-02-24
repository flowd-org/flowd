package main

import (
	"flag"
	"fmt"
	"os"
)

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	fs := flag.NewFlagSet("conformance", flag.ExitOnError)
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: conformance [options]\n\n")
		fmt.Fprintf(os.Stderr, "Conformance harness for flowd M1 E2E gate.\n\n")
		fmt.Fprintf(os.Stderr, "Options:\n")
		fs.PrintDefaults()
	}

	if err := fs.Parse(args); err != nil {
		return 1
	}

	// Placeholder for T-002: config parsing, token sourcing, exit codes
	fmt.Println("conformance harness (T-001 scaffold)")
	return 0
}
