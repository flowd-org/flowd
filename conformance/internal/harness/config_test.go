package harness

import (
	"path/filepath"
	"testing"
)

func TestParseConfig_TokenPrecedence(t *testing.T) {
	tests := []struct {
		name            string
		args            []string
		env             map[string]string
		wantToken       string
		wantExitCode    int
		wantErr         bool
		wantULCProfiles []string
	}{
		{
			name:            "flag overrides env",
			args:            []string{"--token", "from-flag", "--flwd-binary", "/path/to/flwd"},
			env:             map[string]string{"FLWD_TOKEN": "from-env"},
			wantToken:       "from-flag",
			wantExitCode:    ExitOK,
			wantErr:         false,
			wantULCProfiles: []string{"ulc.shell.bash", "ulc.shell.pwsh"},
		},
		{
			name:            "env used when flag not set",
			args:            []string{"--flwd-binary", "/path/to/flwd"},
			env:             map[string]string{"FLWD_TOKEN": "from-env"},
			wantToken:       "from-env",
			wantExitCode:    ExitOK,
			wantErr:         false,
			wantULCProfiles: []string{"ulc.shell.bash", "ulc.shell.pwsh"},
		},
		{
			name:            "empty env with no flag fails",
			args:            []string{"--flwd-binary", "/path/to/flwd"},
			env:             map[string]string{},
			wantToken:       "",
			wantExitCode:    ExitUsage,
			wantErr:         true,
			wantULCProfiles: nil,
		},
		{
			name:            "default ulc-profiles uses canonical bash,pwsh",
			args:            []string{"--flwd-binary", "/path/to/flwd", "--token", "dummy"},
			env:             map[string]string{},
			wantToken:       "dummy",
			wantExitCode:    ExitOK,
			wantErr:         false,
			wantULCProfiles: []string{"ulc.shell.bash", "ulc.shell.pwsh"},
		},
		{
			name:            "mixed profile input with alias bash,pwsh,ulc.shell.bash",
			args:            []string{"--flwd-binary", "/path/to/flwd", "--ulc-profiles", "bash,pwsh,ulc.shell.bash", "--token", "dummy"},
			env:             map[string]string{},
			wantToken:       "dummy",
			wantExitCode:    ExitOK,
			wantErr:         false,
			wantULCProfiles: []string{"ulc.shell.bash", "ulc.shell.pwsh", "ulc.shell.bash"},
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

			// Verify ULC profiles match expected
			if len(cfg.ULCProfiles) != len(tt.wantULCProfiles) {
				t.Errorf("ParseConfig() ULCProfiles length = %d, want %d", len(cfg.ULCProfiles), len(tt.wantULCProfiles))
			} else {
				for i := range tt.wantULCProfiles {
					if cfg.ULCProfiles[i] != tt.wantULCProfiles[i] {
						t.Errorf("ParseConfig() ULCProfiles[%d] = %q, want %q", i, cfg.ULCProfiles[i], tt.wantULCProfiles[i])
					}
				}
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

func TestParseConfig_BindFlag(t *testing.T) {
	tests := []struct {
		name         string
		args         []string
		wantBind     string
		wantExitCode int
		wantErr      bool
	}{
		{
			name:         "empty bind uses auto",
			args:         []string{"--flwd-binary", "/path/to/flwd", "--token", "dummy"},
			wantBind:     "",
			wantExitCode: ExitOK,
			wantErr:      false,
		},
		{
			name:         "explicit bind is preserved",
			args:         []string{"--flwd-binary", "/path/to/flwd", "--token", "dummy", "--bind", "127.0.0.1:19000"},
			wantBind:     "127.0.0.1:19000",
			wantExitCode: ExitOK,
			wantErr:      false,
		},
		{
			name:         "whitespace-only bind trimmed to empty (auto)",
			args:         []string{"--flwd-binary", "/path/to/flwd", "--token", "dummy", "--bind", "   "},
			wantBind:     "",
			wantExitCode: ExitOK,
			wantErr:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, exitCode, err := ParseConfig(tt.args, map[string]string{})

			if (err != nil) != tt.wantErr {
				t.Errorf("ParseConfig() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if exitCode != tt.wantExitCode {
				t.Errorf("ParseConfig() exitCode = %d, want %d", exitCode, tt.wantExitCode)
			}

			if cfg.Bind != tt.wantBind {
				t.Errorf("ParseConfig() Bind = %q, want %q", cfg.Bind, tt.wantBind)
			}
		})
	}
}

func TestParseConfig_FlwdBinaryCanonicalizedToAbsolute(t *testing.T) {
	tests := []struct {
		name           string
		args           []string
		env            map[string]string
		wantFlwdBinary string
		wantExitCode   int
		wantErr        bool
	}{
		{
			name:           "relative path is canonicalized to absolute",
			args:           []string{"--flwd-binary", "./bin/flwd", "--token", "dummy"},
			env:            map[string]string{},
			wantFlwdBinary: filepath.Join("/home/didacog/didacog.eu/flowd/conformance/internal/harness", "bin/flwd"),
			wantExitCode:   ExitOK,
			wantErr:        false,
		},
		{
			name:           "absolute path is preserved and cleaned",
			args:           []string{"--flwd-binary", "/usr/local/bin/flwd", "--token", "dummy"},
			env:            map[string]string{},
			wantFlwdBinary: "/usr/local/bin/flwd",
			wantExitCode:   ExitOK,
			wantErr:        false,
		},
		{
			name:           "path with dot segments is normalized",
			args:           []string{"--flwd-binary", "/usr/local/../local/bin/flwd", "--token", "dummy"},
			env:            map[string]string{},
			wantFlwdBinary: "/usr/local/bin/flwd",
			wantExitCode:   ExitOK,
			wantErr:        false,
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

			if cfg.FlwdBinary != tt.wantFlwdBinary {
				t.Errorf("ParseConfig() FlwdBinary = %q, want %q", cfg.FlwdBinary, tt.wantFlwdBinary)
			}

			// Assert that FlwdBinary is absolute
			if !filepath.IsAbs(cfg.FlwdBinary) {
				t.Errorf("ParseConfig() FlwdBinary is not absolute: %q", cfg.FlwdBinary)
			}
		})
	}
}

func TestParseConfig_FlwdBinaryRelativePathWithDotSegments(t *testing.T) {
	// Test that parsing with a relative path containing dot segments succeeds
	// and produces a normalized absolute path.
	args := []string{"--flwd-binary", "./../bin/flwd", "--token", "dummy"}
	cfg, exitCode, err := ParseConfig(args, map[string]string{})

	if err != nil {
		t.Errorf("ParseConfig() unexpected error: %v", err)
	}

	if exitCode != ExitOK {
		t.Errorf("ParseConfig() exitCode = %d, want %d", exitCode, ExitOK)
	}

	if !filepath.IsAbs(cfg.FlwdBinary) {
		t.Errorf("ParseConfig() FlwdBinary is not absolute: %q", cfg.FlwdBinary)
	}

	// The path should be cleaned (dot segments resolved)
	expectedPrefix := "/home/didacog/didacog.eu/flowd"
	if !filepath.HasPrefix(cfg.FlwdBinary, expectedPrefix) {
		t.Errorf("ParseConfig() FlwdBinary = %q, expected prefix %q", cfg.FlwdBinary, expectedPrefix)
	}
}
