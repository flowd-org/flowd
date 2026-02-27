package harness

import (
	"os"
	"path/filepath"
	"testing"
)

func TestStartFlwd_BootstrapRootPreflight(t *testing.T) {
	tests := []struct {
		name            string
		setupRunRoot    func(t *testing.T) string
		wantExitCode    int
		wantErrContains string
	}{
		{
			name: "missing flwd binary returns ExitInfra",
			setupRunRoot: func(t *testing.T) string {
				tmpDir, err := os.MkdirTemp("", "flwd-process-test-*")
				if err != nil {
					t.Fatalf("Failed to create temp dir: %v", err)
				}
				t.Cleanup(func() { os.RemoveAll(tmpDir) })
				return tmpDir
			},
			wantExitCode:    ExitInfra,
			wantErrContains: "flwd binary not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runRoot := tt.setupRunRoot(t)

			cfg := Config{
				FlwdBinary: "/nonexistent/flwd",
			}

			_, exitCode, err := StartFlwd(t.Context(), cfg, runRoot)

			if err == nil {
				t.Errorf("StartFlwd() expected error, got nil")
			}

			if exitCode != tt.wantExitCode {
				t.Errorf("StartFlwd() exitCode = %d, want %d", exitCode, tt.wantExitCode)
			}

			if tt.wantErrContains != "" && err != nil {
				errStr := err.Error()
				if !contains(errStr, tt.wantErrContains) {
					t.Errorf("StartFlwd() error = %q, want to contain %q", errStr, tt.wantErrContains)
				}
			}
		})
	}
}

func TestStartFlwd_BootstrapRootNotReadable(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "flwd-process-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create a bootstrap root that exists but is unreadable
	bootstrapRoot := filepath.Join(tmpDir, "scripts", "fixtures", "tree-v1")
	if err := os.MkdirAll(bootstrapRoot, 0o000); err != nil {
		t.Skipf("Cannot create unreadable directory on this system: %v", err)
	}

	cfg := Config{
		FlwdBinary: "/nonexistent/flwd",
	}

	_, exitCode, err := StartFlwd(t.Context(), cfg, tmpDir)

	if err == nil {
		t.Errorf("StartFlwd() expected error for unreadable bootstrap root, got nil")
	}

	if exitCode != ExitInfra {
		t.Errorf("StartFlwd() exitCode = %d, want %d", exitCode, ExitInfra)
	}

	errStr := err.Error()
	if !contains(errStr, "bootstrap root not readable") && !contains(errStr, "cannot stat bootstrap root") {
		t.Errorf("StartFlwd() error = %q, want to contain 'bootstrap root not readable' or 'cannot stat bootstrap root'", errStr)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsAt(s, substr))
}

func containsAt(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
