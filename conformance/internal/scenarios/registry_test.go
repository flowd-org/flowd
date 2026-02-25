package scenarios

import (
	"testing"
)

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
