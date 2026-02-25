package harness

import (
	"testing"
)

func TestParseConfig_TokenPrecedence(t *testing.T) {
	tests := []struct {
		name         string
		args         []string
		env          map[string]string
		wantToken    string
		wantExitCode int
		wantErr      bool
	}{
		{
			name:         "flag overrides env",
			args:         []string{"--token", "from-flag", "--flwd-binary", "/path/to/flwd"},
			env:          map[string]string{"FLWD_TOKEN": "from-env"},
			wantToken:    "from-flag",
			wantExitCode: ExitOK,
			wantErr:      false,
		},
		{
			name:         "env used when flag not set",
			args:         []string{"--flwd-binary", "/path/to/flwd"},
			env:          map[string]string{"FLWD_TOKEN": "from-env"},
			wantToken:    "from-env",
			wantExitCode: ExitOK,
			wantErr:      false,
		},
		{
			name:         "empty env with no flag fails",
			args:         []string{"--flwd-binary", "/path/to/flwd"},
			env:          map[string]string{},
			wantToken:    "",
			wantExitCode: ExitUsage,
			wantErr:      true,
		},
		{
			name:         "empty flag with no env fails",
			args:         []string{"--token", "", "--flwd-binary", "/path/to/flwd"},
			env:          map[string]string{},
			wantToken:    "",
			wantExitCode: ExitUsage,
			wantErr:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, exitCode, err := ParseConfig(tt.args, tt.env)

			if (err != nil) != tt.wantErr {
				t.Errorf("ParseConfig() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if exitCode != tt.wantExitCode {
				t.Errorf("ParseConfig() exitCode = %d, want %d", exitCode, tt.wantExitCode)
			}

			if cfg.Token != tt.wantToken {
				t.Errorf("ParseConfig() token = %q, want %q", cfg.Token, tt.wantToken)
			}
		})
	}
}

func TestParseConfig_MissingToken(t *testing.T) {
	// Test that missing token returns exit code 2
	_, exitCode, err := ParseConfig([]string{"--flwd-binary", "/path/to/flwd"}, map[string]string{})

	if err == nil {
		t.Errorf("ParseConfig() expected error for missing token")
	}

	if exitCode != ExitUsage {
		t.Errorf("ParseConfig() exitCode = %d, want %d", exitCode, ExitUsage)
	}
}

func TestParseConfig_RequiredFlags(t *testing.T) {
	// Test that --flwd-binary is required
	_, exitCode, err := ParseConfig([]string{}, map[string]string{"FLWD_TOKEN": "dummy"})

	if err == nil {
		t.Errorf("ParseConfig() expected error for missing --flwd-binary")
	}

	if exitCode != ExitUsage {
		t.Errorf("ParseConfig() exitCode = %d, want %d", exitCode, ExitUsage)
	}
}
