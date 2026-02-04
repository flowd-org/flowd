// SPDX-License-Identifier: AGPL-3.0-or-later

package metrics

const (
	// SSETransportDefault is the transport label for standard SSE streams.
	SSETransportDefault = "sse"
)

// SSEStreamStarted increments the active stream gauge and returns a function to decrement it.
func SSEStreamStarted() func() {
	getSink().RecordSSEActiveDelta(SSETransportDefault, 1)
	getSink().RecordSSEStreamStart(SSETransportDefault)
	return func() {
		getSink().RecordSSEActiveDelta(SSETransportDefault, -1)
		getSink().RecordSSEStreamEnd(SSETransportDefault)
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

// RecordSSEStreamStart increments the SSE stream start counter for the transport.
func RecordSSEStreamStart(transport string) {
	getSink().RecordSSEStreamStart(transport)
}

// RecordSSEStreamEnd increments the SSE stream end counter for the transport.
func RecordSSEStreamEnd(transport string) {
	getSink().RecordSSEStreamEnd(transport)
}
