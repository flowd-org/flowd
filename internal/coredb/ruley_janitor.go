package coredb

import (
	"context"
	"database/sql"
	"errors"
	"time"

	sqlite3 "modernc.org/sqlite/lib"
)

const (
	defaultRuleYJanitorInterval = 30 * time.Second
	defaultRuleYJanitorBatch    = 256
)

// RuleYJanitorOptions controls janitor runtime behavior.
type RuleYJanitorOptions struct {
	Now           func() time.Time
	Tick          <-chan time.Time
	Interval      time.Duration
	Batch         int
	MaxIterations int // Max sweeps per tick (default: 16)
}

// RuleYJanitor removes expired KV rows in bounded batches.
type RuleYJanitor struct {
	db       *DB
	now      func() time.Time
	tick     <-chan time.Time
	interval time.Duration
	batch    int
	maxIter  int // max sweeps per tick (bounded)
}

// NewRuleYJanitor constructs a janitor with defaults suitable for server mode.
func NewRuleYJanitor(db *DB, opts RuleYJanitorOptions) *RuleYJanitor {
	nowFn := opts.Now
	if nowFn == nil {
		nowFn = func() time.Time { return time.Now().UTC() }
	}
	interval := opts.Interval
	if interval <= 0 {
		interval = defaultRuleYJanitorInterval
	}
	batch := opts.Batch
	if batch <= 0 {
		batch = defaultRuleYJanitorBatch
	}
	maxIterations := opts.MaxIterations
	if maxIterations <= 0 {
		maxIterations = 16
	}
	return &RuleYJanitor{
		db:       db,
		now:      nowFn,
		tick:     opts.Tick,
		interval: interval,
		batch:    batch,
		maxIter:  maxIterations,
	}
}

// Sweep deletes up to the configured batch of expired rows.
func (j *RuleYJanitor) Sweep(ctx context.Context) (int64, error) {
	if j == nil || j.db == nil || j.db.sql == nil {
		return 0, ErrRuleYUnavailable
	}
	nowMillis := j.now().UnixMilli()
	res, err := j.db.sql.ExecContext(ctx, `DELETE FROM kv
		WHERE rowid IN (
			SELECT rowid
			FROM kv
			WHERE expires_at IS NOT NULL AND expires_at <= ?
			LIMIT ?
		)`, nowMillis, j.batch)
	if err != nil {
		return 0, err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return 0, err
	}
	return affected, nil
}

// SweepUntilDrained repeatedly calls Sweep until no more expired rows remain
// or the maximum number of iterations is reached (bounded).
func (j *RuleYJanitor) SweepUntilDrained(ctx context.Context) (int64, error) {
	if j == nil || j.db == nil || j.db.sql == nil {
		return 0, ErrRuleYUnavailable
	}
	var totalDeleted int64

	for i := 0; i < j.maxIter; i++ {
		deleted, err := j.Sweep(ctx)
		if err != nil {
			return totalDeleted, err
		}
		if deleted == 0 {
			break
		}
		totalDeleted += deleted
	}
	return totalDeleted, nil
}

// Run executes janitor sweeps on each tick until ctx is canceled.
func (j *RuleYJanitor) Run(ctx context.Context) error {
	if j == nil || j.db == nil || j.db.sql == nil {
		return ErrRuleYUnavailable
	}

	ticks := j.tick
	var ticker *time.Ticker
	if ticks == nil {
		ticker = time.NewTicker(j.interval)
		defer ticker.Stop()
		ticks = ticker.C
	}

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticks:
			if _, err := j.SweepUntilDrained(ctx); err != nil &&
				!errors.Is(err, sql.ErrTxDone) &&
				!errors.Is(err, context.Canceled) {
				var coder codeError
				if !errors.As(err, &coder) || coder.Code() != int(sqlite3.SQLITE_INTERRUPT) {
					return err
				}
			}
		}
	}
}
