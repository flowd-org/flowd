// SPDX-License-Identifier: AGPL-3.0-or-later
package types

type ErrorHandling struct {
	Policy       string `yaml:"policy,omitempty"`
	Retries      int    `yaml:"retries,omitempty"`
	RetryBackoff int    `yaml:"retry_backoff,omitempty"`
}

type Config struct {
	Interpreter    string            `yaml:"interpreter,omitempty"`
	Env            map[string]string `yaml:"env,omitempty"`
	Timeout        int               `yaml:"timeout,omitempty"`
	ErrorHandling  ErrorHandling     `yaml:"error_handling,omitempty"`
	Executor       string            `yaml:"executor,omitempty"`
	Container      *ContainerConfig  `yaml:"container,omitempty"`
	EnvInheritance bool              `yaml:"env_inheritance,omitempty"`
	Composition    string            `yaml:"composition,omitempty"`
	Steps          []StepConfig      `yaml:"steps,omitempty"`
	//old ---------------
	Arguments map[string]ArgumentDefinition `yaml:"arguments,omitempty"`
	// New (Phase 1): SOT-aligned ArgSpec (preferred when provided)
	ArgSpec *ArgSpec       `yaml:"argspec,omitempty"`
	Aliases []CommandAlias `yaml:"aliases,omitempty"`
}

// CommandAlias defines a friendly alias for a fully qualified job path.
type CommandAlias struct {
	From        string `yaml:"from"`
	To          string `yaml:"to"`
	Description string `yaml:"description,omitempty"`
}

// StepConfig captures configuration for DAG steps.
type StepConfig struct {
	ID        string           `yaml:"id,omitempty"`
	Name      string           `yaml:"name,omitempty"`
	Script    string           `yaml:"script,omitempty"`
	Needs     []string         `yaml:"needs,omitempty"`
	Executor  string           `yaml:"executor,omitempty"`
	Container *ContainerConfig `yaml:"container,omitempty"`
}

// ContainerConfig captures container-specific execution settings.
type ContainerConfig struct {
	Image          string              `yaml:"image,omitempty"`
	Resources      *ContainerResources `yaml:"resources,omitempty"`
	Network        string              `yaml:"network,omitempty"`
	RootfsWritable bool                `yaml:"rootfs_writable,omitempty"`
	Capabilities   []string            `yaml:"capabilities,omitempty"`
	ExtraArgs      []string            `yaml:"extra_args,omitempty"`
	Entrypoint     []string            `yaml:"entrypoint,omitempty"`
}

// ContainerResources holds resource requests for container executors.
type ContainerResources struct {
	CPU    string `yaml:"cpu,omitempty"`
	Memory string `yaml:"memory,omitempty"`
}

// RuleYNamespaceConfig captures the per-namespace restrictions for the Rule-Y KV store.
type RuleYNamespaceConfig struct {
	LimitBytes int64 `yaml:"limit_bytes,omitempty" json:"limit_bytes,omitempty"`
}

// RuleYConfig defines the namespace allowlist and quotas for the Rule-Y KV API.
type RuleYConfig struct {
	Allowlist map[string]RuleYNamespaceConfig `yaml:"allowlist,omitempty" json:"allowlist,omitempty"`
}

func (c *Config) EnvSlice() []string {
	if c == nil {
		return nil
	}
	out := make([]string, 0, len(c.Env))
	for k, v := range c.Env {
		out = append(out, k+"="+v)
	}
	return out
}
