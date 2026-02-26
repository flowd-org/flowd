package harness

import (
	"fmt"
	"strings"
	"testing"
)

func TestDebugRedact(t *testing.T) {
	token := "secret-token-xyz"
	input := "Authorization:    Bearer " + token

	fmt.Printf("Input: %q\n", input)
	fmt.Printf("Token: %q\n", token)

	// First, let's see what RedactSecrets does with just the raw token
	result1 := strings.ReplaceAll(input, token, "[REDACTED]")
	fmt.Printf("After raw token replacement: %q\n", result1)

	// Then let's see what redactAuthorizationHeader does
	result2 := redactAuthorizationHeader(result1)
	fmt.Printf("After redactAuthorizationHeader: %q\n", result2)
}

func TestDebugRedact2(t *testing.T) {
	// Test the redactAuthorizationHeader function directly
	input := "Authorization:    Bearer [REDACTED]"
	fmt.Printf("\nDirect test:\n")
	fmt.Printf("Input: %q\n", input)

	result := redactAuthorizationHeader(input)
	fmt.Printf("Output: %q\n", result)

	// Let's also trace through manually
	remaining := input[len("Authorization"):]
	fmt.Printf("After 'authorization': %q\n", remaining)

	colonIdx := strings.Index(remaining, ":")
	fmt.Printf("Colon index: %d\n", colonIdx)

	afterColon := remaining[colonIdx+1:]
	fmt.Printf("After colon: %q\n", afterColon)

	lowerAfterColon := strings.ToLower(afterColon)
	bearerIdx := strings.Index(lowerAfterColon, "bearer")
	fmt.Printf("Bearer index (case-insensitive): %d\n", bearerIdx)

	if bearerIdx >= 0 {
		fmt.Printf("After 'bearer': %q\n", afterColon[bearerIdx+len("bearer"):])
	}
}
