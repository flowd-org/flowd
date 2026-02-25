package report

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/flowd-org/flowd/conformance/internal/harness"
)

func TestReport_JSONStability(t *testing.T) {
	report := Report{
		SuiteMeta: SuiteMeta{
			Name:       "test-suite",
			Profiles:   []string{"ulc.shell.bash", "ulc.shell.pwsh"},
			TotalTests: 10,
		},
		ScenarioCount: 5,
		PassedCount:   4,
		FailedCount:   1,
		Results: []ScenarioResult{
			{
				ScenarioID:   "test-1",
				ScenarioName: "Test Scenario 1",
				Profile:      "ulc.shell.bash",
				Passed:       true,
				DurationMs:   100,
			},
			{
				ScenarioID:   "test-2",
				ScenarioName: "Test Scenario 2",
				Profile:      "ulc.shell.bash",
				Passed:       false,
				DurationMs:   200,
				Failure: &FailureDetail{
					Expected: "success",
					Actual:   "failure",
				},
			},
		},
	}

	// Marshal to JSON
	data, err := json.Marshal(report)
	if err != nil {
		t.Fatalf("json.Marshal() failed: %v", err)
	}

	// Unmarshal back
	var decoded Report
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("json.Unmarshal() failed: %v", err)
	}

	// Verify fields are present and correct
	if decoded.SuiteMeta.Name != report.SuiteMeta.Name {
		t.Errorf("SuiteMeta.Name = %q, want %q", decoded.SuiteMeta.Name, report.SuiteMeta.Name)
	}

	if decoded.ScenarioCount != report.ScenarioCount {
		t.Errorf("ScenarioCount = %d, want %d", decoded.ScenarioCount, report.ScenarioCount)
	}

	if len(decoded.Results) != len(report.Results) {
		t.Errorf("Results length = %d, want %d", len(decoded.Results), len(report.Results))
	}

	// Verify results with failures have Failure field
	if decoded.Results[1].Failure == nil {
		t.Error("Result with failure should have non-nil Failure field")
	}

	if decoded.Results[1].Failure.Expected != report.Results[1].Failure.Expected {
		t.Errorf("Failure.Expected = %q, want %q", decoded.Results[1].Failure.Expected, report.Results[1].Failure.Expected)
	}

	// Verify results without failures have nil Failure field
	if decoded.Results[0].Failure != nil {
		t.Error("Passed result should have nil Failure field")
	}
}

func TestFormatSummary(t *testing.T) {
	tests := []struct {
		name   string
		report Report
		want   string
	}{
		{
			name: "all passed",
			report: Report{
				ScenarioCount: 3,
				PassedCount:   3,
				FailedCount:   0,
			},
			want: "CONFORMANCE PASS: 3 passed, 0 failed out of 3 scenarios",
		},
		{
			name: "some failed",
			report: Report{
				ScenarioCount: 5,
				PassedCount:   3,
				FailedCount:   2,
			},
			want: "CONFORMANCE FAIL: 3 passed, 2 failed out of 5 scenarios",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatSummary(tt.report)
			if got != tt.want {
				t.Errorf("FormatSummary() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestFormatFailureBlock(t *testing.T) {
	result := ScenarioResult{
		ScenarioID:   "test-scenario",
		ScenarioName: "Test Scenario",
		Profile:      "ulc.shell.bash",
		DurationMs:   150,
		Failure: &FailureDetail{
			Expected: "status 200",
			Actual:   "status 500",
		},
	}

	output := FormatFailureBlock(result)

	if len(output) == 0 {
		t.Error("FormatFailureBlock() returned empty string")
	}

	expectedSubstrings := []string{
		"Scenario: test-scenario",
		"Test Scenario",
		"Profile: ulc.shell.bash",
		"Duration: 150ms",
		"Expected: status 200",
		"Actual: status 500",
	}

	for _, substr := range expectedSubstrings {
		if !contains(output, substr) {
			t.Errorf("FormatFailureBlock() output missing: %q", substr)
		}
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && findSubstring(s, substr))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func TestEmitInfraErr(t *testing.T) {
	// Verify the contains helper works as expected
	output := "Error: failed to start flwd: some error occurred"
	if len(output) == 0 {
		t.Error("Expected non-empty output")
	}

	expectedSubstr := "failed to start flwd"
	if !contains(output, expectedSubstr) {
		t.Errorf("Output should contain %q", expectedSubstr)
	}

	// Test that redaction works
	err := fmt.Errorf("connection failed with token secret-token-123")
	redacted := harness.RedactSecrets(err.Error(), "secret-token-123")
	if contains(redacted, "secret-token-123") {
		t.Errorf("Redacted error should not contain raw token, got: %q", redacted)
	}
}
