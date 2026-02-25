package scenarios

import (
	"context"
	"fmt"
	"time"

	"github.com/flowd-org/flowd/conformance/internal/harness"
	"github.com/flowd-org/flowd/conformance/internal/report"
)

// FixtureSourceName is the canonical source name used for conformance fixtures.
const FixtureSourceName = "conformance-fixtures"

// Result represents the outcome of running a single scenario under a single profile.
type Result struct {
	ScenarioID string
	Profile    string
	Passed     bool
	Duration   time.Duration
	Failure    *Failure
}

// Failure captures detailed failure information.
type Failure struct {
	Message string
	Stack   string
}

// Env holds the execution environment for running scenarios.
type Env struct {
	BaseURL         string
	Token           string
	HTTPClient      *harness.Client
	FlwdProcess     *harness.FlwdProcess
	ScenarioTimeout time.Duration
	Verbose         bool
	Profile         string
}

// Scenario represents a single conformance scenario.
type Scenario struct {
	ID             string
	Name           string
	ConformanceIDs []string
	UnmappedReason string
	Profiles       []string
	Run            func(ctx context.Context, env Env) Result
}

// Validate checks that the scenario meets required invariants.
func (s Scenario) Validate() error {
	if s.ID == "" {
		return fmt.Errorf("scenario ID cannot be empty")
	}
	if s.Name == "" {
		return fmt.Errorf("scenario name cannot be empty")
	}
	if len(s.ConformanceIDs) == 0 && s.UnmappedReason == "" {
		return fmt.Errorf("scenario must have at least one ConformanceID or an UnmappedReason")
	}
	if len(s.Profiles) == 0 {
		return fmt.Errorf("scenario must have at least one profile")
	}
	return nil
}

// DefaultProfiles returns the default profiles (bash and pwsh).
func DefaultProfiles() []string {
	return []string{"ulc.shell.bash", "ulc.shell.pwsh"}
}

// All returns the complete list of registered scenarios.
func All() []Scenario {
	return []Scenario{
		ULCSmokeScenario(),
		APISurfaceScenario(),
		TenantScenario(),
		ScenarioCanonicalJobIDs(),
		ScenarioCollisionBehavior(),
	}
}

// RunSuite runs all scenarios under the specified profiles and returns a report.
func RunSuite(ctx context.Context, env Env, scenarios []Scenario, profiles []string) report.Report {
	var results []report.ScenarioResult

	// Required scenarios that must always run at least once regardless of profile selection
	requiredScenarioIDs := map[string]bool{
		"api-surface": true,
		"tenant":      true,
	}

	for _, scenario := range scenarios {
		// Validate scenario before running
		if err := scenario.Validate(); err != nil {
			results = append(results, report.ScenarioResult{
				ScenarioID:   scenario.ID,
				ScenarioName: scenario.Name,
				Profile:      profiles[0], // Use first profile for validation failures
				Passed:       false,
				DurationMs:   0,
				Failure: &report.FailureDetail{
					Expected: "scenario to be valid",
					Actual:   err.Error(),
				},
			})
			continue
		}

		// Determine profiles to run for this scenario
		runProfiles := profiles
		if len(scenario.Profiles) > 0 {
			// Intersect scenario profiles with selected profiles
			runProfiles = intersectProfiles(scenario.Profiles, profiles)
			// For required scenarios, if no matching profiles, still run once with the first selected profile
			if len(runProfiles) == 0 && requiredScenarioIDs[scenario.ID] {
				runProfiles = []string{profiles[0]}
			} else if len(runProfiles) == 0 {
				continue // No matching profiles and not a required scenario
			}
		}

		for _, profile := range runProfiles {
			// Wrap scenario execution in timeout
			timeoutCtx, cancel := context.WithTimeout(ctx, env.ScenarioTimeout)

			// Bind the current profile to a copy of env for the scenario
			profileEnv := env
			profileEnv.Profile = profile

			result := scenario.Run(timeoutCtx, profileEnv)
			result.ScenarioID = scenario.ID
			result.Profile = profile

			cancel()

			// Map scenario.Result to report.ScenarioResult
			scenarioResult := report.ScenarioResult{
				ScenarioID:   scenario.ID,
				ScenarioName: scenario.Name,
				Profile:      profile,
				Passed:       result.Passed,
				DurationMs:   int(result.Duration.Milliseconds()),
			}

			if !result.Passed && result.Failure != nil {
				scenarioResult.Failure = &report.FailureDetail{
					Expected: "",
					Actual:   result.Failure.Message,
				}
			}

			results = append(results, scenarioResult)
		}
	}

	// Count passed/failed
	passed := 0
	failed := 0
	for _, r := range results {
		if r.Passed {
			passed++
		} else {
			failed++
		}
	}

	// Block empty conformance suite passes
	if len(results) == 0 {
		return report.Report{
			SuiteMeta: report.SuiteMeta{
				Name:       "conformance",
				Profiles:   profiles,
				TotalTests: 0,
			},
			ScenarioCount: 0,
			PassedCount:   0,
			FailedCount:   1,
			Results: []report.ScenarioResult{
				{
					ScenarioID:   "empty-suite",
					ScenarioName: "Empty conformance suite",
					Profile:      profiles[0],
					Passed:       false,
					DurationMs:   0,
					Failure: &report.FailureDetail{
						Expected: "at least one scenario to run",
						Actual:   "no scenarios matched the selected profiles",
					},
				},
			},
		}
	}

	return report.Report{
		SuiteMeta: report.SuiteMeta{
			Name:       "conformance",
			Profiles:   profiles,
			TotalTests: len(results),
		},
		ScenarioCount: len(results),
		PassedCount:   passed,
		FailedCount:   failed,
		Results:       results,
	}
}

// intersectProfiles returns the intersection of scenarioProfiles and selectedProfiles.
func intersectProfiles(scenarioProfiles, selectedProfiles []string) []string {
	selected := make(map[string]bool)
	for _, p := range selectedProfiles {
		selected[p] = true
	}

	var result []string
	for _, p := range scenarioProfiles {
		if selected[p] {
			result = append(result, p)
		}
	}
	return result
}
