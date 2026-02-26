package scenarios

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/flowd-org/flowd/conformance/internal/harness"
	"github.com/flowd-org/flowd/conformance/internal/report"
)

// FakeScenario creates a scenario with a custom Run function for testing.
func FakeScenario(id, name string, runFunc func(ctx context.Context, env Env) Result) Scenario {
	return Scenario{
		ID:             id,
		Name:           name,
		ConformanceIDs: []string{"TEST-001"},
		Profiles:       DefaultProfiles(),
		Run:            runFunc,
	}
}

func TestScenario_Validate(t *testing.T) {
	tests := []struct {
		name     string
		scenario Scenario
		wantErr  bool
	}{
		{
			name: "valid scenario with conformance IDs",
			scenario: Scenario{
				ID:             "test-scenario",
				Name:           "Test Scenario",
				ConformanceIDs: []string{"M1-001"},
				Profiles:       []string{"ulc.shell.bash"},
			},
			wantErr: false,
		},
		{
			name: "valid scenario with unmapped reason",
			scenario: Scenario{
				ID:             "test-scenario",
				Name:           "Test Scenario",
				UnmappedReason: "Not yet implemented",
				Profiles:       []string{"ulc.shell.bash"},
			},
			wantErr: false,
		},
		{
			name: "invalid - empty ID",
			scenario: Scenario{
				Name:           "Test Scenario",
				ConformanceIDs: []string{"M1-001"},
				Profiles:       []string{"ulc.shell.bash"},
			},
			wantErr: true,
		},
		{
			name: "invalid - empty name",
			scenario: Scenario{
				ID:             "test-scenario",
				ConformanceIDs: []string{"M1-001"},
				Profiles:       []string{"ulc.shell.bash"},
			},
			wantErr: true,
		},
		{
			name: "invalid - no conformance IDs and no unmapped reason",
			scenario: Scenario{
				ID:       "test-scenario",
				Name:     "Test Scenario",
				Profiles: []string{"ulc.shell.bash"},
			},
			wantErr: true,
		},
		{
			name: "invalid - empty profiles",
			scenario: Scenario{
				ID:             "test-scenario",
				Name:           "Test Scenario",
				ConformanceIDs: []string{"M1-001"},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.scenario.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Scenario.Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestDefaultProfiles(t *testing.T) {
	profiles := DefaultProfiles()

	if len(profiles) != 2 {
		t.Errorf("DefaultProfiles() expected 2 profiles, got %d", len(profiles))
	}

	if profiles[0] != "ulc.shell.bash" {
		t.Errorf("DefaultProfiles()[0] = %q, want %q", profiles[0], "ulc.shell.bash")
	}

	if profiles[1] != "ulc.shell.pwsh" {
		t.Errorf("DefaultProfiles()[1] = %q, want %q", profiles[1], "ulc.shell.pwsh")
	}
}

func TestAll_ScenarioIDsUnique(t *testing.T) {
	scenarios := All()
	seen := make(map[string]bool)

	for _, s := range scenarios {
		if seen[s.ID] {
			t.Errorf("Duplicate scenario ID: %s", s.ID)
		}
		seen[s.ID] = true
	}
}

func TestAll_ValidateAll(t *testing.T) {
	scenarios := All()

	for _, s := range scenarios {
		err := s.Validate()
		if err != nil {
			t.Errorf("Scenario %s failed validation: %v", s.ID, err)
		}
	}
}

func TestRunSuite_ProfileAttribution(t *testing.T) {
	// Create a fake scenario that returns a result with the profile from env
	fakeScenario := FakeScenario("fake", "Fake Scenario", func(ctx context.Context, env Env) Result {
		return Result{Passed: true, Profile: env.Profile}
	})

	profiles := []string{"ulc.shell.bash", "ulc.shell.pwsh"}
	env := Env{Profile: "", ScenarioTimeout: 30 * time.Second}

	// Run suite with 2 scenarios x 2 profiles = 4 expected results
	report := RunSuite(context.Background(), env, []Scenario{fakeScenario, fakeScenario}, profiles)

	if len(report.Results) != 4 {
		t.Errorf("expected 4 results (2 scenarios x 2 profiles), got %d", len(report.Results))
	}

	// Verify each result has exactly one profile and correct scenario ID
	for _, r := range report.Results {
		if r.ScenarioID != "fake" {
			t.Errorf("expected ScenarioID 'fake', got '%s'", r.ScenarioID)
		}
		// Each result should have exactly one profile (not comma-separated)
		if r.Profile != "ulc.shell.bash" && r.Profile != "ulc.shell.pwsh" {
			t.Errorf("expected exactly one profile, got '%s'", r.Profile)
		}
	}
}

// TestRunSuite_EmptyScenarios verifies that an empty conformance suite fails
// This test ensures the harness blocks empty suites with a clear error
func TestRunSuite_EmptyScenarios(t *testing.T) {
	ctx := context.Background()
	env := Env{
		BaseURL:         "http://localhost:8080",
		Token:           "test-token",
		HTTPClient:      &harness.Client{},
		ScenarioTimeout: 30 * time.Second,
		Verbose:         false,
		Profile:         "ulc.shell.bash",
	}

	// Run with empty scenarios - should now fail with 1 failed result
	report := RunSuite(ctx, env, []Scenario{}, []string{"ulc.shell.bash"})

	// Verify the report shows failure for empty suite
	if report.ScenarioCount != 0 {
		t.Errorf("Expected ScenarioCount=0, got %d", report.ScenarioCount)
	}
	if len(report.Results) != 1 {
		t.Errorf("Expected Results length=1 (empty-suite failure), got %d", len(report.Results))
	}
	if report.PassedCount != 0 {
		t.Errorf("Expected PassedCount=0, got %d", report.PassedCount)
	}
	if report.FailedCount != 1 {
		t.Errorf("Expected FailedCount=1 (empty suite fails), got %d", report.FailedCount)
	}
	// Verify the failure has expected details
	if len(report.Results) > 0 {
		r := report.Results[0]
		if r.ScenarioID != "empty-suite" {
			t.Errorf("Expected ScenarioID='empty-suite', got '%s'", r.ScenarioID)
		}
		if r.ScenarioName != "Empty conformance suite" {
			t.Errorf("Expected ScenarioName='Empty conformance suite', got '%s'", r.ScenarioName)
		}
		if r.Profile != "ulc.shell.bash" {
			t.Errorf("Expected Profile='ulc.shell.bash', got '%s'", r.Profile)
		}
		if r.Passed {
			t.Errorf("Expected Passed=false (empty suite fails), got true")
		}
		if r.Failure == nil {
			t.Errorf("Expected Failure to be non-nil")
		} else {
			if r.Failure.Expected != "at least one scenario to run" {
				t.Errorf("Expected Expected='at least one scenario to run', got '%s'", r.Failure.Expected)
			}
			if r.Failure.Actual != "no scenarios matched the selected profiles" {
				t.Errorf("Expected Actual='no scenarios matched the selected profiles', got '%s'", r.Failure.Actual)
			}
		}
	}
}

// TestFormatSummary_EmptyReport formats summary for an empty report
func TestFormatSummary_EmptyReport(t *testing.T) {
	rep := report.Report{
		SuiteMeta: report.SuiteMeta{
			Name:       "empty-suite",
			Profiles:   []string{"ulc.shell.bash"},
			TotalTests: 0,
		},
		ScenarioCount: 0,
		PassedCount:   0,
		FailedCount:   0,
		Results:       []report.ScenarioResult{},
	}

	summary := report.FormatSummary(rep)
	expected := "CONFORMANCE PASS: 0 passed, 0 failed out of 0 scenarios"
	if summary != expected {
		t.Errorf("FormatSummary() = %q, want %q", summary, expected)
	}
}

// TestRunSuite_EmptyScenarios_FailsGate validates that empty scenario execution
// does not produce PASS semantics - it should fail the gate with a clear error.
func TestRunSuite_EmptyScenarios_FailsGate(t *testing.T) {
	ctx := context.Background()
	env := Env{
		BaseURL:         "http://localhost:8080",
		Token:           "test-token",
		HTTPClient:      &harness.Client{},
		ScenarioTimeout: 30 * time.Second,
		Verbose:         false,
		Profile:         "ulc.shell.bash",
	}

	// Run with empty scenarios - should fail with 1 failed result
	rep := RunSuite(ctx, env, []Scenario{}, []string{"ulc.shell.bash"})

	// Verify the report shows failure for empty suite
	if rep.ScenarioCount != 0 {
		t.Errorf("Expected ScenarioCount=0, got %d", rep.ScenarioCount)
	}
	if len(rep.Results) != 1 {
		t.Errorf("Expected Results length=1 (empty-suite failure), got %d", len(rep.Results))
	}
	if rep.PassedCount != 0 {
		t.Errorf("Expected PassedCount=0, got %d", rep.PassedCount)
	}
	if rep.FailedCount != 1 {
		t.Errorf("Expected FailedCount=1 (empty suite fails), got %d", rep.FailedCount)
	}
	// Verify the failure has expected details
	if len(rep.Results) > 0 {
		r := rep.Results[0]
		if r.ScenarioID != "empty-suite" {
			t.Errorf("Expected ScenarioID='empty-suite', got '%s'", r.ScenarioID)
		}
		if r.ScenarioName != "Empty conformance suite" {
			t.Errorf("Expected ScenarioName='Empty conformance suite', got '%s'", r.ScenarioName)
		}
		if r.Profile != "ulc.shell.bash" {
			t.Errorf("Expected Profile='ulc.shell.bash', got '%s'", r.Profile)
		}
		if r.Passed {
			t.Errorf("Expected Passed=false (empty suite fails), got true")
		}
		if r.Failure == nil {
			t.Errorf("Expected Failure to be non-nil")
		} else {
			if r.Failure.Expected != "at least one scenario to run" {
				t.Errorf("Expected Expected='at least one scenario to run', got '%s'", r.Failure.Expected)
			}
			if r.Failure.Actual != "no scenarios matched the selected profiles" {
				t.Errorf("Expected Actual='no scenarios matched the selected profiles', got '%s'", r.Failure.Actual)
			}
		}
	}
	// Verify report summary indicates failure
	summary := report.FormatSummary(rep)
	if summary == "" || !containsString(summary, "0 passed") {
		t.Errorf("Expected summary to indicate 0 passed, got '%s'", summary)
	}
	if summary == "" || !containsString(summary, "1 failed") {
		t.Errorf("Expected summary to indicate 1 failed, got '%s'", summary)
	}
}

func containsString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// TestAPISurfaceScenario_Healthz204 tests that healthz returning 204 is accepted
func TestAPISurfaceScenario_Healthz204(t *testing.T) {
	// Create a test server that returns 204 for /healthz and valid JSON for other endpoints
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/healthz":
			w.WriteHeader(http.StatusNoContent)
		case "/startupz", "/readyz":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"status": "ok"}`))
		case "/capabilities":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"core": {"version": "1.0.0", "spec_version": "1.0.0"}}`))
		case "/limits":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"algorithm": "sha256", "queue_max_depth": 1000, "backpressure_mode": "block", "queue_stats": {}}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	client := &harness.Client{
		BaseURL: server.URL,
		Token:   "test-token",
		HTTP:    &http.Client{},
	}

	env := Env{
		BaseURL:    server.URL,
		Token:      "test-token",
		HTTPClient: client,
		Profile:    "api-surface",
	}

	scenario := APISurfaceScenario()
	result := scenario.Run(context.Background(), env)

	if !result.Passed {
		t.Errorf("Expected APISurfaceScenario to pass with 204 healthz, got failure: %v", result.Failure)
	}
}

// TestAPISurfaceScenario_JSONBodyReachesValidator tests that a valid JSON body reaches the validator
func TestAPISurfaceScenario_JSONBodyReachesValidator(t *testing.T) {
	// Create a test server that returns valid JSON for all endpoints
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/healthz":
			w.WriteHeader(http.StatusNoContent)
		case "/startupz", "/readyz":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"status": "ok"}`))
		case "/capabilities":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"core": {"version": "1.0.0", "spec_version": "1.0.0"}}`))
		case "/limits":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"algorithm": "sha256", "queue_max_depth": 1000, "backpressure_mode": "block", "queue_stats": {}}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	client := &harness.Client{
		BaseURL: server.URL,
		Token:   "test-token",
		HTTP:    &http.Client{},
	}

	env := Env{
		BaseURL:    server.URL,
		Token:      "test-token",
		HTTPClient: client,
		Profile:    "api-surface",
	}

	scenario := APISurfaceScenario()
	result := scenario.Run(context.Background(), env)

	if !result.Passed {
		t.Errorf("Expected APISurfaceScenario to pass with valid JSON response, got failure: %v", result.Failure)
	}
}

// TestRunSuite_RequiredScenariosAlwaysRun verifies that required scenarios
// (api-surface, tenant) run at least once even when selected profiles don't match their profile tags
func TestRunSuite_RequiredScenariosAlwaysRun(t *testing.T) {
	// Use only ULC profiles that don't match api-surface or tenant profiles
	profiles := []string{"ulc.shell.bash", "ulc.shell.pwsh"}
	env := Env{Profile: "", ScenarioTimeout: 30 * time.Second}

	// Create fake scenarios that mimic the required scenarios' profile tags
	apiSurfaceFake := Scenario{
		ID:             "api-surface",
		Name:           "API Surface - Health Probes & Endpoints",
		ConformanceIDs: []string{"M1-003", "M1-004"},
		Profiles:       []string{"api-surface"}, // Different from selected profiles
		Run:            func(ctx context.Context, env Env) Result { return Result{Passed: true, Profile: env.Profile} },
	}
	tenantFake := Scenario{
		ID:             "tenant",
		Name:           "Tenant Propagation",
		ConformanceIDs: []string{"M1-005"},
		Profiles:       []string{"tenant"}, // Different from selected profiles
		Run:            func(ctx context.Context, env Env) Result { return Result{Passed: true, Profile: env.Profile} },
	}

	// Run suite with only required scenarios - they should still run once
	report := RunSuite(context.Background(), env, []Scenario{apiSurfaceFake, tenantFake}, profiles)

	// Check that both required scenarios appear in results (each once, with first profile)
	foundAPISurface := false
	foundTenant := false
	for _, r := range report.Results {
		if r.ScenarioID == "api-surface" {
			foundAPISurface = true
			if r.Profile != "ulc.shell.bash" {
				t.Errorf("api-surface should run with first profile 'ulc.shell.bash', got '%s'", r.Profile)
			}
		}
		if r.ScenarioID == "tenant" {
			foundTenant = true
			if r.Profile != "ulc.shell.bash" {
				t.Errorf("tenant should run with first profile 'ulc.shell.bash', got '%s'", r.Profile)
			}
		}
	}

	if !foundAPISurface {
		t.Errorf("Required scenario 'api-surface' was filtered out; expected at least one result")
	}
	if !foundTenant {
		t.Errorf("Required scenario 'tenant' was filtered out; expected at least one result")
	}
}

// TestAPISurfaceScenario_EmptyBodyFails tests that an empty response body fails validation
func TestAPISurfaceScenario_EmptyBodyFails(t *testing.T) {
	// Create a test server that returns empty body for /capabilities but valid for others
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/healthz":
			w.WriteHeader(http.StatusNoContent)
		case "/startupz", "/readyz":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"status": "ok"}`))
		case "/capabilities":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			// Write truly empty response body
			w.Write(nil)
		case "/limits":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"algorithm": "sha256", "queue_max_depth": 1000, "backpressure_mode": "block", "queue_stats": {}}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	client := &harness.Client{
		BaseURL: server.URL,
		Token:   "test-token",
		HTTP:    &http.Client{},
	}

	env := Env{
		BaseURL:    server.URL,
		Token:      "test-token",
		HTTPClient: client,
		Profile:    "api-surface",
	}

	scenario := APISurfaceScenario()
	result := scenario.Run(context.Background(), env)

	if result.Passed {
		t.Errorf("Expected APISurfaceScenario to fail with empty response body, but it passed")
	}

	// Verify the failure contains either "empty response body" or "unexpected end of JSON input"
	// (both indicate the empty body was properly rejected)
	if result.Failure != nil && !strings.Contains(result.Failure.Message, "empty response body") &&
		!strings.Contains(result.Failure.Message, "unexpected end of JSON input") {
		t.Errorf("Expected failure message to contain 'empty response body' or 'unexpected end of JSON input', got: %v", result.Failure.Message)
	}
}

// TestRunSuite_WrapperScenarioFailureDetailsPreserved verifies that wrapper
// scenario failures preserve diagnostic details (Failure.Actual text).
// Note: This test documents the current behavior where Expected is empty
// and Actual contains the error message. The fix should populate Expected
// with meaningful diagnostic info.
func TestRunSuite_WrapperScenarioFailureDetailsPreserved(t *testing.T) {
	// Create a test server that returns a failing response for /runs
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/healthz":
			w.WriteHeader(http.StatusNoContent)
		case "/startupz", "/readyz":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"status": "ok"}`))
		case "/capabilities":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"core": {"version": "1.0.0", "spec_version": "1.0.0"}}`))
		case "/limits":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"algorithm": "sha256", "queue_max_depth": 1000, "backpressure_mode": "block", "queue_stats": {}}`))
		case "/runs":
			// Return a 500 error to simulate a wrapper scenario failure
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(`{"error": "internal server error"}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	client := &harness.Client{
		BaseURL: server.URL,
		Token:   "test-token",
		HTTP:    &http.Client{},
	}

	env := Env{
		BaseURL:         server.URL,
		Token:           "test-token",
		HTTPClient:      client,
		Profile:         "ulc.shell.bash",
		ScenarioTimeout: 10 * time.Second,
	}

	// Run the canonical job IDs scenario which will fail against the test server
	scenario := ScenarioCanonicalJobIDs()
	report := RunSuite(context.Background(), env, []Scenario{scenario}, []string{"ulc.shell.bash"})

	// Verify we have exactly one result
	if len(report.Results) != 1 {
		t.Fatalf("Expected 1 result, got %d", len(report.Results))
	}

	result := report.Results[0]

	// Verify the scenario failed (expected due to 500 error)
	if result.Passed {
		t.Error("Expected scenario to fail with 500 error, but it passed")
	}

	// Verify failure details are preserved (regression check for RF-001, RF-002)
	if result.Failure == nil {
		t.Error("Expected Failure to be non-nil for failed result")
	} else {
		// Verify Actual is non-empty (critical for actionable failure text)
		if result.Failure.Actual == "" {
			t.Error("Expected Failure.Actual to be non-empty for actionable failure text")
		}

		// Verify Actual contains error context from the server response
		if !strings.Contains(result.Failure.Actual, "500") &&
			!strings.Contains(result.Failure.Actual, "internal server error") &&
			!strings.Contains(result.Failure.Actual, "context deadline exceeded") {
			t.Errorf("Expected Failure.Actual to contain error context, got: %q", result.Failure.Actual)
		}
	}
}
