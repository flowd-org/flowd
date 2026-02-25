package scenarios

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/flowd-org/flowd/conformance/internal/harness"
)

// ScenarioCanonicalJobIDs validates that job IDs are stable, canonical, and
// predictable from the source path.
func ScenarioCanonicalJobIDs() Scenario {
	return Scenario{
		ID:             "canonical-job-ids",
		Name:           "Canonical Job IDs",
		ConformanceIDs: []string{"M1-006"},
		Profiles:       DefaultProfiles(),
		Run:            runCanonicalJobIDs,
	}
}

// runCanonicalJobIDs implements the canonical job IDs scenario.
func runCanonicalJobIDs(ctx context.Context, env Env) Result {
	start := time.Now()

	// For each profile, create a run and verify the job ID is stable and canonical
	profiles := []string{"ulc.shell.bash", "ulc.shell.pwsh"}
	for _, profile := range profiles {
		result := validateJobID(ctx, env, profile)
		if !result.Passed {
			return result
		}
	}

	return Result{
		ScenarioID: "canonical-job-ids",
		Profile:    strings.Join(profiles, ","),
		Passed:     true,
		Duration:   time.Since(start),
	}
}

// validateJobID creates a run and verifies the job ID is stable and canonical.
func validateJobID(ctx context.Context, env Env, profile string) Result {
	start := time.Now()

	// Determine expected job ID based on profile
	expectedJobID := getJobIDForProfile(profile)

	// Create run payload
	payload := map[string]interface{}{
		"job_id": expectedJobID,
		"args":   map[string]interface{}{},
		"source": map[string]interface{}{
			"name": "conformance-fixtures",
		},
	}

	// Marshal to JSON with stable ordering
	bodyBytes, err := harness.CanonicalJSON(payload)
	if err != nil {
		return Result{
			ScenarioID: "canonical-job-ids",
			Profile:    profile,
			Passed:     false,
			Duration:   time.Since(start),
			Failure: &Failure{
				Message: fmt.Sprintf("failed to marshal payload: %v", err),
			},
		}
	}

	// Create idempotency headers
	idempotencyKey := fmt.Sprintf("conformance-canonical-%d", time.Now().UnixNano())
	idempotencySHA256 := harness.ComputeSHA256(bodyBytes)

	// Build request
	req, err := http.NewRequestWithContext(ctx, "POST", env.BaseURL+"/runs", bytes.NewReader(bodyBytes))
	if err != nil {
		return Result{
			ScenarioID: "canonical-job-ids",
			Profile:    profile,
			Passed:     false,
			Duration:   time.Since(start),
			Failure: &Failure{
				Message: fmt.Sprintf("failed to create request: %v", err),
			},
		}
	}

	// Add headers
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+env.Token)
	req.Header.Set("Idempotency-Key", idempotencyKey)
	req.Header.Set("Idempotency-SHA256", idempotencySHA256)

	// Execute request
	resp, err := env.HTTPClient.HTTP.Do(req)
	if err != nil {
		return Result{
			ScenarioID: "canonical-job-ids",
			Profile:    profile,
			Passed:     false,
			Duration:   time.Since(start),
			Failure: &Failure{
				Message: fmt.Sprintf("failed to execute request: %v", err),
			},
		}
	}
	defer resp.Body.Close()

	// Check response status
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return Result{
			ScenarioID: "canonical-job-ids",
			Profile:    profile,
			Passed:     false,
			Duration:   time.Since(start),
			Failure: &Failure{
				Message: fmt.Sprintf("POST /runs returned status %d: %s", resp.StatusCode, redactBody(string(body), env.Token)),
			},
		}
	}

	// Parse run ID and job ID from response
	var runResp struct {
		ID     string `json:"id"`
		JobID  string `json:"job_id"`
		Tenant string `json:"tenant"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&runResp); err != nil {
		return Result{
			ScenarioID: "canonical-job-ids",
			Profile:    profile,
			Passed:     false,
			Duration:   time.Since(start),
			Failure: &Failure{
				Message: fmt.Sprintf("failed to decode run response: %v", err),
			},
		}
	}

	// Assert job ID matches expected
	if runResp.JobID != expectedJobID {
		return Result{
			ScenarioID: "canonical-job-ids",
			Profile:    profile,
			Passed:     false,
			Duration:   time.Since(start),
			Failure: &Failure{
				Message: fmt.Sprintf("job ID mismatch: expected %q, got %q", expectedJobID, runResp.JobID),
			},
		}
	}

	// Fetch run via GET /runs/{id} to verify stability
	reqGet, err := http.NewRequestWithContext(ctx, "GET", fmt.Sprintf("%s/runs/%s", env.BaseURL, runResp.ID), nil)
	if err != nil {
		return Result{
			ScenarioID: "canonical-job-ids",
			Profile:    profile,
			Passed:     false,
			Duration:   time.Since(start),
			Failure: &Failure{
				Message: fmt.Sprintf("failed to create GET request: %v", err),
			},
		}
	}
	reqGet.Header.Set("Authorization", "Bearer "+env.Token)

	respGet, err := env.HTTPClient.HTTP.Do(reqGet)
	if err != nil {
		return Result{
			ScenarioID: "canonical-job-ids",
			Profile:    profile,
			Passed:     false,
			Duration:   time.Since(start),
			Failure: &Failure{
				Message: fmt.Sprintf("failed to execute GET request: %v", err),
			},
		}
	}
	defer respGet.Body.Close()

	// Check GET response status
	if respGet.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(respGet.Body, 4096))
		return Result{
			ScenarioID: "canonical-job-ids",
			Profile:    profile,
			Passed:     false,
			Duration:   time.Since(start),
			Failure: &Failure{
				Message: fmt.Sprintf("GET /runs/%s returned status %d: %s", runResp.ID, respGet.StatusCode, redactBody(string(body), env.Token)),
			},
		}
	}

	// Parse job ID from GET response
	var getResp struct {
		ID     string `json:"id"`
		JobID  string `json:"job_id"`
		Tenant string `json:"tenant"`
	}
	if err := json.NewDecoder(respGet.Body).Decode(&getResp); err != nil {
		return Result{
			ScenarioID: "canonical-job-ids",
			Profile:    profile,
			Passed:     false,
			Duration:   time.Since(start),
			Failure: &Failure{
				Message: fmt.Sprintf("failed to decode GET response: %v", err),
			},
		}
	}

	// Assert job ID is stable across requests
	if getResp.JobID != expectedJobID {
		return Result{
			ScenarioID: "canonical-job-ids",
			Profile:    profile,
			Passed:     false,
			Duration:   time.Since(start),
			Failure: &Failure{
				Message: fmt.Sprintf("GET job ID mismatch: expected %q, got %q", expectedJobID, getResp.JobID),
			},
		}
	}

	return Result{
		ScenarioID: "canonical-job-ids",
		Profile:    profile,
		Passed:     true,
		Duration:   time.Since(start),
	}
}

// ScenarioCollisionBehavior validates that duplicate job IDs are handled consistently.
func ScenarioCollisionBehavior() Scenario {
	return Scenario{
		ID:             "collision-behavior",
		Name:           "Collision Behavior",
		ConformanceIDs: []string{"M1-007"},
		Profiles:       DefaultProfiles(),
		Run:            runCollisionBehavior,
	}
}

// runCollisionBehavior implements the collision behavior scenario.
func runCollisionBehavior(ctx context.Context, env Env) Result {
	start := time.Now()

	// For each profile, create two runs with the same job ID and verify collision handling
	profiles := []string{"ulc.shell.bash", "ulc.shell.pwsh"}
	for _, profile := range profiles {
		result := validateCollision(ctx, env, profile)
		if !result.Passed {
			return result
		}
	}

	return Result{
		ScenarioID: "collision-behavior",
		Profile:    strings.Join(profiles, ","),
		Passed:     true,
		Duration:   time.Since(start),
	}
}

// validateCollision creates two runs with the same job ID and verifies collision handling.
func validateCollision(ctx context.Context, env Env, profile string) Result {
	start := time.Now()

	// Determine expected job ID based on profile
	expectedJobID := getJobIDForProfile(profile)

	// First run - should succeed
	firstRunID, err := createRun(ctx, env, expectedJobID, profile)
	if err != nil {
		return Result{
			ScenarioID: "collision-behavior",
			Profile:    profile,
			Passed:     false,
			Duration:   time.Since(start),
			Failure: &Failure{
				Message: fmt.Sprintf("first run failed: %v", err),
			},
		}
	}

	// Second run with same job ID - should either succeed (dedupe) or fail (reject)
	secondRunID, secondErr := createRun(ctx, env, expectedJobID, profile)

	// If second run succeeded, verify it's the same run (dedupe)
	if secondErr == nil {
		if secondRunID != firstRunID {
			return Result{
				ScenarioID: "collision-behavior",
				Profile:    profile,
				Passed:     false,
				Duration:   time.Since(start),
				Failure: &Failure{
					Message: fmt.Sprintf("collision handling inconsistent: expected same run ID, got %q vs %q", firstRunID, secondRunID),
				},
			}
		}
	} else {
		// Second run failed - verify it's a 409 Conflict (expected behavior)
		if !strings.Contains(secondErr.Error(), "409") && !strings.Contains(secondErr.Error(), "Conflict") {
			return Result{
				ScenarioID: "collision-behavior",
				Profile:    profile,
				Passed:     false,
				Duration:   time.Since(start),
				Failure: &Failure{
					Message: fmt.Sprintf("collision handling failed unexpectedly: %v", secondErr),
				},
			}
		}
	}

	return Result{
		ScenarioID: "collision-behavior",
		Profile:    profile,
		Passed:     true,
		Duration:   time.Since(start),
	}
}

// createRun creates a run with the given job ID and returns the run ID or an error.
func createRun(ctx context.Context, env Env, jobID, profile string) (string, error) {
	payload := map[string]interface{}{
		"job_id": jobID,
		"args":   map[string]interface{}{},
		"source": map[string]interface{}{
			"name": "conformance-fixtures",
		},
	}

	bodyBytes, err := harness.CanonicalJSON(payload)
	if err != nil {
		return "", fmt.Errorf("failed to marshal payload: %w", err)
	}

	idempotencyKey := fmt.Sprintf("conformance-collision-%d", time.Now().UnixNano())
	idempotencySHA256 := harness.ComputeSHA256(bodyBytes)

	req, err := http.NewRequestWithContext(ctx, "POST", env.BaseURL+"/runs", bytes.NewReader(bodyBytes))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+env.Token)
	req.Header.Set("Idempotency-Key", idempotencyKey)
	req.Header.Set("Idempotency-SHA256", idempotencySHA256)

	resp, err := env.HTTPClient.HTTP.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return "", fmt.Errorf("POST /runs returned status %d: %s", resp.StatusCode, redactBody(string(body), env.Token))
	}

	var runResp struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&runResp); err != nil {
		return "", fmt.Errorf("failed to decode run response: %w", err)
	}

	return runResp.ID, nil
}
