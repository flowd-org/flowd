// SPDX-License-Identifier: AGPL-3.0-or-later

package metrics

import (
	"sync"
	"time"
)

// Sink receives core metrics events without depending on server packages.
type Sink interface {
	EnsurePersistenceLatency(operation, outcome string)
	RecordPersistenceLatency(operation, outcome string, duration time.Duration)
	RecordPersistenceEviction(kind string, bytes int64)
	RecordSSEActiveDelta(transport string, delta int64)
	RecordSSEResumeAttempt()
	RecordSSECursorExpired()
}

type noopSink struct{}

func (noopSink) EnsurePersistenceLatency(string, string)                {}
func (noopSink) RecordPersistenceLatency(string, string, time.Duration) {}
func (noopSink) RecordPersistenceEviction(string, int64)                {}
func (noopSink) RecordSSEActiveDelta(string, int64)                     {}
func (noopSink) RecordSSEResumeAttempt()                                {}
func (noopSink) RecordSSECursorExpired()                                {}

var (
	sinkMu sync.RWMutex
	sink   Sink = noopSink{}
)

// SetSink installs the metrics sink used by core packages.
// Passing nil resets the sink to a no-op implementation.
func SetSink(s Sink) {
	if s == nil {
		s = noopSink{}
	}
	sinkMu.Lock()
	sink = s
	sinkMu.Unlock()
	seedPersistenceDefaults(s)
}

func getSink() Sink {
	sinkMu.RLock()
	defer sinkMu.RUnlock()
	return sink
}
