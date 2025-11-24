// SPDX-License-Identifier: AGPL-3.0-or-later

package metrics

import (
	servermetrics "github.com/flowd-org/flowd/internal/server/metrics"
)

const (
	// SSETransportDefault is the transport label for standard SSE streams.
	SSETransportDefault = "sse"
)

// SSEStreamStarted increments the active stream gauge and returns a function to decrement it.
func SSEStreamStarted() func() {
	servermetrics.Default.RecordSSEActiveDelta(SSETransportDefault, 1)
	return func() {
		servermetrics.Default.RecordSSEActiveDelta(SSETransportDefault, -1)
	}
}

// RecordSSEResumeAttempt increments the SSE resume counter.
func RecordSSEResumeAttempt() {
	servermetrics.Default.RecordSSEResumeAttempt()
}

// RecordSSECursorExpired increments the SSE cursor expired counter.
func RecordSSECursorExpired() {
	servermetrics.Default.RecordSSECursorExpired()
}
