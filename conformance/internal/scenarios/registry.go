package scenarios

import (
	"context"
	"fmt"
	"time"

	"github.com/flowd-org/flowd/conformance/internal/harness"
	"github.com/flowd-org/flowd/conformance/internal/report"
)

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
			if len(runProfiles) == 0 {
				continue // No matching profiles
			}
		}

		for _, profile := range runProfiles {
			// Wrap scenario execution in timeout
			timeoutCtx, cancel := context.WithTimeout(ctx, env.ScenarioTimeout)

			result := scenario.Run(timeoutCtx, env)
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
