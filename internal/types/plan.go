// SPDX-License-Identifier: AGPL-3.0-or-later
package types

// Minimal Plan types (subset) for Phase 1 planning and preview

type Plan struct {
	JobID            string                 `json:"job_id"`
	EffectiveArgSpec ArgSpec                `json:"effective_argspec"`
	ExecutorPreview  map[string]interface{} `json:"executor_preview,omitempty"`
	Requirements     *PlanRequirements      `json:"requirements,omitempty"`
	ResolvedArgs     map[string]interface{} `json:"resolved_args,omitempty"`
	SecurityProfile  string                 `json:"security_profile,omitempty"`
	PolicyFindings   []Finding              `json:"policy_findings,omitempty"`
	ImageTrust       *ImageTrustPreview     `json:"image_trust,omitempty"`
	Steps            []PlanStepPreview      `json:"steps,omitempty"`
	Provenance       map[string]interface{} `json:"provenance,omitempty"`
}

type PlanRequirements struct {
	Tools  []ToolRequirement `json:"tools,omitempty"`
	Status string            `json:"status,omitempty"` // ok|failed
}

type ToolRequirement struct {
	Name            string `json:"name"`
	Version         string `json:"version,omitempty"`
	Status          string `json:"status,omitempty"` // unknown|present|missing
	Path            string `json:"path,omitempty"`
	DetectedVersion string `json:"detected_version,omitempty"`
}

// Finding captures policy evaluation messages surfaced to clients.
type Finding struct {
	Code    string `json:"code"`
	Level   string `json:"level,omitempty"`
	Message string `json:"message,omitempty"`
}

// ImageTrustPreview summarizes signature verification results for preview responses.
type ImageTrustPreview struct {
	Image    string `json:"image,omitempty"`
	Mode     string `json:"mode,omitempty"`
	Verified bool   `json:"verified"`
	Reason   string `json:"reason,omitempty"`
}

// PlanStepPreview summarizes executor details for DAG steps.
type PlanStepPreview struct {
	ID             string              `json:"id,omitempty"`
	Name           string              `json:"name,omitempty"`
	Executor       string              `json:"executor,omitempty"`
	ContainerImage string              `json:"container_image,omitempty"`
	Network        string              `json:"network,omitempty"`
	RootfsWritable bool                `json:"rootfs_writable,omitempty"`
	Capabilities   []string            `json:"capabilities,omitempty"`
	Resources      *ContainerResources `json:"resources,omitempty"`
	ImageTrust     *ImageTrustPreview  `json:"image_trust,omitempty"`
}
