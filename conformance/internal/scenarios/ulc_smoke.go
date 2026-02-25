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

// ULCSmokeScenario creates a scenario that validates local-source registration
// and run creation under both bash and pwsh profiles.
func ULCSmokeScenario() Scenario {
	return Scenario{
		ID:             "ulc-smoke",
		Name:           "ULC Smoke Run",
		ConformanceIDs: []string{"M1-001", "M1-002"},
		Profiles:       DefaultProfiles(),
		Run:            runULCSmoke,
	}
}

// runULCSmoke implements the ULC smoke scenario.
func runULCSmoke(ctx context.Context, env Env) Result {
	start := time.Now()

	// Run only the profile specified in env.Profile
	result := runProfile(ctx, env, env.Profile)

	return Result{
		ScenarioID: "ulc-smoke",
		Profile:    env.Profile,
		Passed:     result.Passed,
		Duration:   time.Since(start),
	}
}

// runProfile runs a single profile's smoke test.
func runProfile(ctx context.Context, env Env, profile string) Result {
	start := time.Now()

	// Determine job ID based on profile
	jobID := getJobIDForProfile(profile)

	// Create run payload
	payload := map[string]interface{}{
		"job_id": jobID,
		"args":   map[string]interface{}{},
		"source": map[string]interface{}{
			"name": FixtureSourceName,
		},
	}

	// Marshal to JSON with stable ordering
	bodyBytes, err := harness.CanonicalJSON(payload)
	if err != nil {
		return Result{
			ScenarioID: "ulc-smoke",
			Profile:    profile,
			Passed:     false,
			Duration:   time.Since(start),
			Failure: &Failure{
				Message: fmt.Sprintf("failed to marshal payload: %v", err),
			},
		}
	}

	// Create idempotency headers
	idempotencyKey := fmt.Sprintf("conformance-tenant-%d", time.Now().UnixNano())
	idempotencySHA256 := harness.ComputeSHA256(bodyBytes)

	// Build request
	req, err := http.NewRequestWithContext(ctx, "POST", env.BaseURL+"/runs", bytes.NewReader(bodyBytes))
	if err != nil {
		return Result{
			ScenarioID: "ulc-smoke",
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
			ScenarioID: "ulc-smoke",
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
			ScenarioID: "ulc-smoke",
			Profile:    profile,
			Passed:     false,
			Duration:   time.Since(start),
			Failure: &Failure{
				Message: fmt.Sprintf("POST /runs returned status %d: %s", resp.StatusCode, redactBody(string(body), env.Token)),
			},
		}
	}

	// Parse run ID from response
	var runResp struct {
		ID string `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&runResp); err != nil {
		return Result{
			ScenarioID: "ulc-smoke",
			Profile:    profile,
			Passed:     false,
			Duration:   time.Since(start),
			Failure: &Failure{
				Message: fmt.Sprintf("failed to decode run response: %v", err),
			},
		}
	}

	runID := runResp.ID

	// Poll for run completion
	if err := pollRunCompletion(ctx, env, runID, profile, start); err != nil {
		return Result{
			ScenarioID: "ulc-smoke",
			Profile:    profile,
			Passed:     false,
			Duration:   time.Since(start),
			Failure: &Failure{
				Message: err.Error(),
			},
		}
	}

	return Result{
		ScenarioID: "ulc-smoke",
		Profile:    profile,
		Passed:     true,
		Duration:   time.Since(start),
	}
}

// getJobIDForProfile returns the job ID for a given profile.
func getJobIDForProfile(profile string) string {
	switch profile {
	case "ulc.shell.bash":
		return "conformance/ulc-smoke-bash"
	case "ulc.shell.pwsh":
		return "conformance/ulc-smoke-pwsh"
	default:
		return "conformance/ulc-smoke-bash"
	}
}

// pollRunCompletion polls the run endpoint until completion or timeout.
func pollRunCompletion(ctx context.Context, env Env, runID, profile string, start time.Time) error {
	pollURL := fmt.Sprintf("%s/runs/%s", env.BaseURL, runID)
	timeout := env.ScenarioTimeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}

	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("polling cancelled: %v", ctx.Err())
		case <-ticker.C:
			elapsed := time.Since(start)
			if elapsed > timeout {
				// Fetch events for failure report
				events := fetchEvents(ctx, env, runID, profile)
				return fmt.Errorf("run %s timed out after %v. Events: %s", runID, elapsed, events)
			}

			req, err := http.NewRequestWithContext(ctx, "GET", pollURL, nil)
			if err != nil {
				continue
			}
			req.Header.Set("Authorization", "Bearer "+env.Token)

			resp, err := env.HTTPClient.HTTP.Do(req)
			if err != nil {
				continue
			}

			if resp.StatusCode != http.StatusOK {
				resp.Body.Close()
				continue
			}

			var run struct {
				Status string `json:"status"`
			}
			if err := json.NewDecoder(resp.Body).Decode(&run); err != nil {
				resp.Body.Close()
				continue
			}
			resp.Body.Close()

			// Check if status is terminal
			if isTerminalStatus(run.Status) {
				if run.Status != "completed" {
					// Fetch events for failure report
					events := fetchEvents(ctx, env, runID, profile)
					return fmt.Errorf("run %s completed with status %s. Events: %s", runID, run.Status, events)
				}
				return nil
			}
		}
	}
}

// isTerminalStatus checks if the status is a terminal state.
func isTerminalStatus(status string) bool {
	switch status {
	case "completed", "failed", "cancelled", "timeout":
		return true
	default:
		return false
	}
}

// fetchEvents retrieves the last N lines of events for a run.
func fetchEvents(ctx context.Context, env Env, runID, profile string) string {
	eventsURL := fmt.Sprintf("%s/runs/%s/events.ndjson", env.BaseURL, runID)

	req, err := http.NewRequestWithContext(ctx, "GET", eventsURL, nil)
	if err != nil {
		return "failed to create request"
	}
	req.Header.Set("Authorization", "Bearer "+env.Token)

	resp, err := env.HTTPClient.HTTP.Do(req)
	if err != nil {
		return fmt.Sprintf("failed to fetch events: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Sprintf("failed to read events: %v", err)
	}

	// Redact secrets
	redacted := redactBody(string(body), env.Token)

	// Keep only last N lines (bounded)
	lines := strings.Split(redacted, "\n")
	bounded := lines
	if len(lines) > 50 {
		bounded = lines[len(lines)-50:]
	}

	return strings.Join(bounded, "\n")
}
