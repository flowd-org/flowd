// SPDX-License-Identifier: AGPL-3.0-or-later
package engine

import (
	"context"
	"time"

	"github.com/flowd-org/flowd/internal/events"
	"github.com/flowd-org/flowd/internal/types"
)

// NOTE: These interfaces are intentionally minimal in T-002.
// Follow-up tasks will provide concrete implementations and wire them from CLI/HTTP.

type RunStore interface {
	Create(ctx context.Context, runID, jobID string, startedAt time.Time) error
	SetFinished(ctx context.Context, runID string, status string, finishedAt time.Time) error
}

type JournalStore interface {
	Append(ctx context.Context, runID string, eventType string, payload []byte, ts time.Time) (int64, error)
}

type IdempotencyStore interface {
	Lookup(ctx context.Context, key string) (status int, payload []byte, bodyHash string, ok bool, err error)
	Store(ctx context.Context, key string, status int, payload []byte, bodyHash string, ttl time.Duration) error
}

type PolicyEvaluator interface {
	Evaluate(ctx context.Context, runID string, plan types.Plan) ([]types.Finding, error)
}

type SecretProvider interface {
	// Prepare handles for secret args and returns handle paths plus cleanup.
	Prepare(runID string, binding *Binding) (handles map[string]string, cleanup func() error, err error)
}

type Executor interface {
	Run(ctx context.Context, runID string, jobID string, scriptDir string, cfg *types.Config, bind *Binding, sink events.Sink) error
}

type JobResolver interface {
	Resolve(ctx context.Context, jobID string) (scriptDir string, effectiveJobID string, err error)
}

type ConfigLoader interface {
	LoadConfig(scriptDir string) (*types.Config, error)
}

type RunIDGenerator interface {
	NewRunID() string
}
