package scenarios

import (
	"context"
	"testing"
	"time"
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
