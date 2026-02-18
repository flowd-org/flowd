// SPDX-License-Identifier: AGPL-3.0-or-later
package janitor

import (
	"context"
	"testing"
	"time"
)

// TestSupervisorIsBenignError verifies benign error detection.
func TestSupervisorIsBenignError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "context.Canceled is benign",
			err:      context.Canceled,
			expected: true,
		},
		{
			name:     "context.DeadlineExceeded is benign",
			err:      context.DeadlineExceeded,
			expected: true,
		},
		{
			name:     "non-benign error",
			err:      error(nil),
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := isBenignError(tt.err)
			if result != tt.expected {
				t.Errorf("isBenignError() = %v, want %v", result, tt.expected)
			}
		})
	}
}

// TestSupervisorOptionsDefaults verifies default options.
func TestSupervisorOptionsDefaults(t *testing.T) {
	t.Parallel()

	s := NewSupervisor(SupervisorOptions{})
	if s.opts.MaxRestarts != defaultSupervisionMaxRestarts {
		t.Errorf("MaxRestarts = %d, want %d", s.opts.MaxRestarts, defaultSupervisionMaxRestarts)
	}
	if s.opts.RestartWait != defaultSupervisionRestartWait {
		t.Errorf("RestartWait = %v, want %v", s.opts.RestartWait, defaultSupervisionRestartWait)
	}
}

// TestSupervisorOptionsCustom verifies custom options.
func TestSupervisorOptionsCustom(t *testing.T) {
	t.Parallel()

	s := NewSupervisor(SupervisorOptions{
		MaxRestarts: 5,
		RestartWait: 2 * time.Second,
	})
	if s.opts.MaxRestarts != 5 {
		t.Errorf("MaxRestarts = %d, want 5", s.opts.MaxRestarts)
	}
	if s.opts.RestartWait != 2*time.Second {
		t.Errorf("RestartWait = %v, want 2s", s.opts.RestartWait)
	}
}
