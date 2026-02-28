package scenarios

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/flowd-org/flowd/conformance/internal/harness"
)

var authorizationBearerHeaderPattern = regexp.MustCompile(`(?i)authorization\s*:\s*bearer\s+[^\r\n]+`)

// APISurfaceScenario creates a scenario that validates health probes and
// API surface endpoints (/capabilities, /limits) via HTTP.
func APISurfaceScenario() Scenario {
	return Scenario{
		ID:             "api-surface",
		Name:           "API Surface - Health Probes & Endpoints",
		ConformanceIDs: []string{"M1-003", "M1-004"},
		Profiles:       []string{"api-surface"},
		Run:            runAPISurface,
	}
}

// runAPISurface implements the API surface scenario.
func runAPISurface(ctx context.Context, env Env) Result {
	start := time.Now()

	// Run all endpoint checks
	endpoints := []struct {
		name        string
		path        string
		expected    int
		allowStatus func(int) bool
		check       func(statusCode int, body []byte, token string) error
	}{
		{"healthz", "/healthz", 200, func(s int) bool { return s == 200 || s == 204 }, checkHealthz},
		{"startupz", "/startupz", 204, func(s int) bool { return s == 200 || s == 204 }, checkHealthz},
		{"readyz", "/readyz", 204, func(s int) bool { return s == 200 || s == 204 }, checkHealthz},
		{"capabilities", "/capabilities", 200, func(s int) bool { return s == 200 }, checkCapabilities},
		{"limits", "/limits", 200, func(s int) bool { return s == 200 }, checkLimits},
	}

	for _, ep := range endpoints {
		result := checkEndpoint(ctx, env, ep.name, ep.path, ep.expected, ep.allowStatus, ep.check)
		if !result.Passed {
			return result
		}
	}

	return Result{
		ScenarioID: "api-surface",
		Profile:    "api-surface",
		Passed:     true,
		Duration:   time.Since(start),
	}
}

// checkEndpoint makes a GET request to the endpoint and validates the response.
func checkEndpoint(ctx context.Context, env Env, name, path string, expected int, allowStatus func(int) bool, check func(int, []byte, string) error) Result {
	start := time.Now()

	req, err := http.NewRequestWithContext(ctx, "GET", env.BaseURL+path, nil)
	if err != nil {
		return Result{
			ScenarioID: "api-surface",
			Profile:    "api-surface",
			Passed:     false,
			Duration:   time.Since(start),
			Failure: &Failure{
				Message: fmt.Sprintf("failed to create request for %s: %v", name, err),
			},
		}
	}

	req.Header.Set("Authorization", "Bearer "+env.Token)

	resp, err := env.HTTPClient.HTTP.Do(req)
	if err != nil {
		return Result{
			ScenarioID: "api-surface",
			Profile:    "api-surface",
			Passed:     false,
			Duration:   time.Since(start),
			Failure: &Failure{
				Message: fmt.Sprintf("failed to execute request for %s: %v", name, err),
			},
		}
	}
	defer resp.Body.Close()

	// Read body once for diagnostics and validators
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 64*1024))

	// Check status code (allow expected or acceptable alternatives)
	if !allowStatus(resp.StatusCode) {
		return Result{
			ScenarioID: "api-surface",
			Profile:    "api-surface",
			Passed:     false,
			Duration:   time.Since(start),
			Failure: &Failure{
				Message: fmt.Sprintf("%s returned status %d, expected %d: %s", name, resp.StatusCode, expected, redactBody(string(body), env.Token)),
			},
		}
	}

	// Run additional checks with statusCode and body
	if err := check(resp.StatusCode, body, env.Token); err != nil {
		return Result{
			ScenarioID: "api-surface",
			Profile:    "api-surface",
			Passed:     false,
			Duration:   time.Since(start),
			Failure: &Failure{
				Message: fmt.Sprintf("%s validation failed: %v", name, err),
			},
		}
	}

	return Result{
		ScenarioID: "api-surface",
		Profile:    "api-surface",
		Passed:     true,
		Duration:   time.Since(start),
	}
}

// checkHealthz validates the healthz response.
// Accepts 200 or 204 status codes with no body.
func checkHealthz(statusCode int, body []byte, token string) error {
	// healthz should return 200 or 204 with no body
	if statusCode != 200 && statusCode != 204 {
		return fmt.Errorf("unexpected status code %d", statusCode)
	}
	return nil
}

// checkJSONResponse validates a generic JSON response.
func checkJSONResponse(statusCode int, body []byte, token string) error {
	bodyStr := string(body)
	if err := validateJSON(bodyStr); err != nil {
		return fmt.Errorf("invalid JSON: %v", err)
	}
	return nil
}

// validateJSON checks if the string is valid JSON.
func validateJSON(s string) error {
	if strings.TrimSpace(s) == "" {
		return fmt.Errorf("empty response body")
	}
	var v interface{}
	if err := json.Unmarshal([]byte(s), &v); err != nil {
		return err
	}
	return nil
}

// checkCapabilities validates the /capabilities response.
func checkCapabilities(statusCode int, body []byte, token string) error {
	var data map[string]interface{}
	if err := json.Unmarshal(body, &data); err != nil {
		return fmt.Errorf("invalid JSON: %v", err)
	}

	// Check for core.version and core.spec_version
	core, ok := data["core"]
	if !ok {
		return fmt.Errorf("missing 'core' field")
	}

	coreMap, ok := core.(map[string]interface{})
	if !ok {
		return fmt.Errorf("'core' is not an object")
	}

	if _, ok := coreMap["version"]; !ok {
		return fmt.Errorf("missing 'core.version' field")
	}

	if _, ok := coreMap["spec_version"]; !ok {
		return fmt.Errorf("missing 'core.spec_version' field")
	}

	return nil
}

// checkLimits validates the /limits response.
func checkLimits(statusCode int, body []byte, token string) error {
	var data map[string]interface{}
	if err := json.Unmarshal(body, &data); err != nil {
		return fmt.Errorf("invalid JSON: %v", err)
	}

	// Check for expected top-level keys
	expectedKeys := []string{"algorithm", "queue_max_depth", "backpressure_mode", "queue_stats"}
	for _, key := range expectedKeys {
		if _, ok := data[key]; !ok {
			return fmt.Errorf("missing '%s' field", key)
		}
	}

	return nil
}

// redactBody redacts secrets from the response body.
func redactBody(body, token string) string {
	result := harness.RedactSecrets(body, token)
	result = authorizationBearerHeaderPattern.ReplaceAllString(result, "Authorization: Bearer [REDACTED]")
	return result
}
