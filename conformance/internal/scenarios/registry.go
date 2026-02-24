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
	}
}

// RunSuite runs all scenarios under the specified profiles and returns a report.
func RunSuite(ctx context.Context, env Env, scenarios []Scenario, profiles []string) report.Report {
	// TODO: Implement suite runner (T-008)
	return report.Report{}
}
