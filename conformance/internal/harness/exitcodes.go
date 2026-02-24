package harness

// Exit codes for the conformance harness.
// 0: success
// 1: scenario failure
// 2: usage/config error
// 3: infrastructure error (failed to start flwd, timeout, etc.)
const (
	ExitOK           = 0
	ExitScenarioFail = 1
	ExitUsage        = 2
	ExitInfra        = 3
)
