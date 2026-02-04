// SPDX-License-Identifier: AGPL-3.0-or-later
package types

// StartRunRequest is a shared, server-agnostic request shape for starting a run.
// It is intentionally minimal; follow-up tasks will extend it as orchestration is rewired.
type StartRunRequest struct {
	JobID     string                 `json:"job_id"`
	ScriptDir string                 `json:"script_dir,omitempty"`
	Args      map[string]interface{} `json:"args,omitempty"`
	RequestID string                 `json:"request_id,omitempty"`
}
