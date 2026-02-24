package report

import (
	"fmt"
	"strings"
)

// Report is the top-level conformance report structure.
type Report struct {
	SuiteMeta     SuiteMeta        `json:"suite_meta"`
	ScenarioCount int              `json:"scenario_count"`
	PassedCount   int              `json:"passed_count"`
	FailedCount   int              `json:"failed_count"`
	Results       []ScenarioResult `json:"results"`
}

// SuiteMeta contains metadata about the conformance suite.
type SuiteMeta struct {
	Name       string   `json:"name"`
	Profiles   []string `json:"profiles"`
	TotalTests int      `json:"total_tests"`
}

// ScenarioResult represents the outcome of a single scenario.
type ScenarioResult struct {
	ScenarioID   string         `json:"scenario_id"`
	ScenarioName string         `json:"scenario_name"`
	Profile      string         `json:"profile"`
	Passed       bool           `json:"passed"`
	DurationMs   int            `json:"duration_ms"`
	Failure      *FailureDetail `json:"failure,omitempty"`
}

// FailureDetail contains details about a failed scenario.
type FailureDetail struct {
	Expected string `json:"expected"`
	Actual   string `json:"actual"`
}

// FormatSummary returns a concise one-line summary of the report.
func FormatSummary(r Report) string {
	status := "PASS"
	if r.FailedCount > 0 {
		status = "FAIL"
	}
	return fmt.Sprintf("CONFORMANCE %s: %d passed, %d failed out of %d scenarios",
		status, r.PassedCount, r.FailedCount, r.ScenarioCount)
}

// FormatFailureBlock formats a single failure for stdout display.
func FormatFailureBlock(result ScenarioResult) string {
	var buf strings.Builder

	buf.WriteString(fmt.Sprintf("Scenario: %s (%s)\n", result.ScenarioID, result.ScenarioName))
	buf.WriteString(fmt.Sprintf("  Profile: %s\n", result.Profile))
	buf.WriteString(fmt.Sprintf("  Duration: %dms\n", result.DurationMs))

	if result.Failure != nil {
		buf.WriteString(fmt.Sprintf("  Expected: %s\n", result.Failure.Expected))
		buf.WriteString(fmt.Sprintf("  Actual: %s\n", result.Failure.Actual))
	}

	return buf.String()
}
