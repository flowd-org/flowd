// SPDX-License-Identifier: AGPL-3.0-or-later

package metrics

import (
	"strings"
	"time"

	servermetrics "github.com/flowd-org/flowd/internal/server/metrics"
)

const (
	// Persistence operations recorded in metrics.
	PersistenceOperationIdempotencyLookup = "idempotency_lookup"
	PersistenceOperationIdempotencyStore  = "idempotency_store"
	PersistenceOperationJournalAppend     = "journal_append"
	PersistenceOperationJournalRead       = "journal_read"

	// Persistence kinds for eviction counters.
	PersistenceKindJournal     = "journal"
	PersistenceKindIdempotency = "idempotency"
	persistenceKindUnknown     = "unknown"

	// Persistence outcomes used to categorize latency observations.
	PersistenceOutcomeOK            = "ok"
	PersistenceOutcomeError         = "error"
	PersistenceOutcomeHit           = "hit"
	PersistenceOutcomeMiss          = "miss"
	PersistenceOutcomeExpired       = "expired"
	PersistenceOutcomeQuotaExceeded = "quota_exceeded"
)

var latencyDefaults = map[string][]string{
	PersistenceOperationIdempotencyLookup: {
		PersistenceOutcomeHit,
		PersistenceOutcomeMiss,
		PersistenceOutcomeExpired,
		PersistenceOutcomeError,
	},
	PersistenceOperationIdempotencyStore: {
		PersistenceOutcomeOK,
		PersistenceOutcomeError,
	},
	PersistenceOperationJournalAppend: {
		PersistenceOutcomeOK,
		PersistenceOutcomeQuotaExceeded,
		PersistenceOutcomeError,
	},
	PersistenceOperationJournalRead: {
		PersistenceOutcomeOK,
		PersistenceOutcomeError,
	},
}

func init() {
	for operation, outcomes := range latencyDefaults {
		for _, outcome := range outcomes {
			servermetrics.Default.EnsurePersistenceLatency(operation, outcome)
		}
	}
}

// PersistenceTimer records elapsed time for a persistence operation and
// writes the result when Observe is invoked.
type PersistenceTimer struct {
	operation string
	start     time.Time
	recorded  bool
}

// StartPersistenceTimer returns a timer for the supplied operation.
func StartPersistenceTimer(operation string) *PersistenceTimer {
	op := sanitize(operation)
	if op == "" {
		return nil
	}
	return &PersistenceTimer{
		operation: op,
		start:     time.Now(),
	}
}

// Observe records the latency for the timer using the provided outcome.
func (t *PersistenceTimer) Observe(outcome string) {
	if t == nil || t.recorded {
		return
	}
	t.recorded = true
	o := sanitize(outcome)
	if o == "" {
		o = PersistenceOutcomeOK
	}
	servermetrics.Default.RecordPersistenceLatency(t.operation, o, time.Since(t.start))
}

// RecordPersistenceEviction increments counters for persistence evictions.
func RecordPersistenceEviction(kind string, bytes int64) {
	k := sanitize(kind)
	if k == "" {
		k = persistenceKindUnknown
	}
	servermetrics.Default.RecordPersistenceEviction(k, bytes)
}

func sanitize(v string) string {
	return strings.TrimSpace(strings.ToLower(v))
}
