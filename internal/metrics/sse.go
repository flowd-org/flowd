// SPDX-License-Identifier: AGPL-3.0-or-later

package metrics

const (
	// SSETransportDefault is the transport label for standard SSE streams.
	SSETransportDefault = "sse"
)

// SSEStreamStarted increments the active stream gauge and returns a function to decrement it.
func SSEStreamStarted() func() {
	getSink().RecordSSEActiveDelta(SSETransportDefault, 1)
	return func() {
		getSink().RecordSSEActiveDelta(SSETransportDefault, -1)
	}
}

// RecordSSEResumeAttempt increments the SSE resume counter.
func RecordSSEResumeAttempt() {
	getSink().RecordSSEResumeAttempt()
}

// RecordSSECursorExpired increments the SSE cursor expired counter.
func RecordSSECursorExpired() {
	getSink().RecordSSECursorExpired()
}
