// SPDX-License-Identifier: AGPL-3.0-or-later
package janitor

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/flowd-org/flowd/internal/coredb"
)

const (
	defaultSupervisionMaxRestarts = 3
	defaultSupervisionRestartWait = 1 * time.Second
)

// SupervisorOptions controls janitor supervision behavior.
type SupervisorOptions struct {
	MaxRestarts int
	RestartWait time.Duration
}

// Supervisor monitors a janitor and restarts it on non-benign errors.
type Supervisor struct {
	opts SupervisorOptions
}

// NewSupervisor constructs a supervisor with default options.
func NewSupervisor(opts SupervisorOptions) *Supervisor {
	if opts.MaxRestarts == 0 {
		opts.MaxRestarts = defaultSupervisionMaxRestarts
	}
	if opts.RestartWait == 0 {
		opts.RestartWait = defaultSupervisionRestartWait
	}
	return &Supervisor{opts: opts}
}

// SuperviseJanitor starts the janitor, monitors for errors, and restarts on failure.
// It returns only when the context is cancelled or a terminal error occurs.
func (s *Supervisor) SuperviseJanitor(ctx context.Context, j *coredb.RuleYJanitor, logger *slog.Logger) error {
	restarts := 0

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		err := j.Run(ctx)
		if err == nil {
			return nil
		}

		if isBenignError(err) {
			logger.Debug("janitor stopped with benign error", "error", err)
			return nil
		}

		restarts++
		if restarts > s.opts.MaxRestarts {
			logger.Error("janitor exceeded max restarts", "restarts", restarts, "error", err)
			return fmt.Errorf("janitor failed after %d restarts: %w", restarts, err)
		}

		logger.Error("janitor failed, restarting", "restarts", restarts, "error", err)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(s.opts.RestartWait):
			// Continue to restart
		}
	}
}

// isBenignError returns true if the error is considered non-fatal.
func isBenignError(err error) bool {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	// Treat SQLite-specific errors as potentially benign during shutdown
	var sqliteErr *sqliteError
	if errors.As(err, &sqliteErr) {
		return true
	}
	return false
}

// sqliteError wraps sqlite3 errors for type checking.
type sqliteError struct {
	msg string
}

func (e *sqliteError) Error() string {
	return e.msg
}
