package scenarios

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/flowd-org/flowd/conformance/internal/harness"
)

// TenantScenario creates a scenario that validates tenant propagation through
// run creation and read operations.
func TenantScenario() Scenario {
	return Scenario{
		ID:             "tenant",
		Name:           "Tenant Propagation",
		ConformanceIDs: []string{"M1-005"},
		Profiles:       []string{"tenant"},
		Run:            runTenant,
	}
}

// runTenant implements the tenant propagation scenario.
func runTenant(ctx context.Context, env Env) Result {
	start := time.Now()

	// Create run with explicit tenant
	payload := map[string]interface{}{
		"job_id": getJobIDForProfile("ulc.shell.bash"),
		"tenant": "acme",
		"args":   map[string]interface{}{},
		"source": map[string]interface{}{
			"name": FixtureSourceName,
		},
	}

	bodyBytes, err := harness.CanonicalJSON(payload)
	if err != nil {
		return Result{
			ScenarioID: "tenant",
			Profile:    "tenant",
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
			ScenarioID: "tenant",
			Profile:    "tenant",
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
			ScenarioID: "tenant",
			Profile:    "tenant",
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
			ScenarioID: "tenant",
			Profile:    "tenant",
			Passed:     false,
			Duration:   time.Since(start),
			Failure: &Failure{
				Message: fmt.Sprintf("POST /runs returned status %d: %s", resp.StatusCode, redactBody(string(body), env.Token)),
			},
		}
	}

	// Parse run ID and tenant from response
	var runResp struct {
		ID     string `json:"id"`
		Tenant string `json:"tenant"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&runResp); err != nil {
		return Result{
			ScenarioID: "tenant",
			Profile:    "tenant",
			Passed:     false,
			Duration:   time.Since(start),
			Failure: &Failure{
				Message: fmt.Sprintf("failed to decode run response: %v", err),
			},
		}
	}

	runID := runResp.ID
	expectedTenant := "acme"

	// Assert tenant in create response
	if runResp.Tenant != expectedTenant {
		return Result{
			ScenarioID: "tenant",
			Profile:    "tenant",
			Passed:     false,
			Duration:   time.Since(start),
			Failure: &Failure{
				Message: fmt.Sprintf("create response tenant mismatch: expected %q, got %q", expectedTenant, runResp.Tenant),
			},
		}
	}

	// Fetch run via GET /runs/{id}
	reqGet, err := http.NewRequestWithContext(ctx, "GET", fmt.Sprintf("%s/runs/%s", env.BaseURL, runID), nil)
	if err != nil {
		return Result{
			ScenarioID: "tenant",
			Profile:    "tenant",
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
			ScenarioID: "tenant",
			Profile:    "tenant",
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
			ScenarioID: "tenant",
			Profile:    "tenant",
			Passed:     false,
			Duration:   time.Since(start),
			Failure: &Failure{
				Message: fmt.Sprintf("GET /runs/%s returned status %d: %s", runID, respGet.StatusCode, redactBody(string(body), env.Token)),
			},
		}
	}

	// Parse tenant from GET response
	var getResp struct {
		ID     string `json:"id"`
		Tenant string `json:"tenant"`
	}
	if err := json.NewDecoder(respGet.Body).Decode(&getResp); err != nil {
		return Result{
			ScenarioID: "tenant",
			Profile:    "tenant",
			Passed:     false,
			Duration:   time.Since(start),
			Failure: &Failure{
				Message: fmt.Sprintf("failed to decode GET response: %v", err),
			},
		}
	}

	// Assert tenant in GET response
	if getResp.Tenant != expectedTenant {
		return Result{
			ScenarioID: "tenant",
			Profile:    "tenant",
			Passed:     false,
			Duration:   time.Since(start),
			Failure: &Failure{
				Message: fmt.Sprintf("GET response tenant mismatch: expected %q, got %q", expectedTenant, getResp.Tenant),
			},
		}
	}

	return Result{
		ScenarioID: "tenant",
		Profile:    "tenant",
		Passed:     true,
		Duration:   time.Since(start),
	}
}
