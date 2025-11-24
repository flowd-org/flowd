// SPDX-License-Identifier: AGPL-3.0-or-later
package policy

// Bundle represents the policy bundle schema used by the flwd.
// Only a minimal subset is defined here to support Phase 3 tasks.
type Bundle struct {
	VerifySignatures  *string    `yaml:"verify_signatures,omitempty" json:"verify_signatures,omitempty"`
	AllowedRegistries []string   `yaml:"allowed_registries,omitempty" json:"allowed_registries,omitempty"`
	Ceilings          *Ceilings  `yaml:"ceilings,omitempty" json:"ceilings,omitempty"`
	Overrides         *Overrides `yaml:"overrides,omitempty" json:"overrides,omitempty"`
}

// Ceilings captures container resource ceilings (Phase 3 scope).
type Ceilings struct {
	CPU    string `yaml:"cpu,omitempty" json:"cpu,omitempty"`       // e.g., 1000m
	Memory string `yaml:"memory,omitempty" json:"memory,omitempty"` // e.g., 512Mi, 1Gi
}

// Overrides captures allowable overrides for isolation relaxations.
type Overrides struct {
	Network        []string `yaml:"network,omitempty" json:"network,omitempty"` // e.g., ["none","bridge"]
	Caps           []string `yaml:"caps,omitempty" json:"caps,omitempty"`       // e.g., ["NET_RAW"]
	RootfsWritable *bool    `yaml:"rootfs_writable,omitempty" json:"rootfs_writable,omitempty"`
	EnvInheritance *bool    `yaml:"env_inheritance,omitempty" json:"env_inheritance,omitempty"`
}

// NormalizeVerifySignatures ensures the value is one of required|permissive|disabled.
func NormalizeVerifySignatures(v string) (string, bool) {
	switch vLower := lower(v); vLower {
	case "required", "permissive", "disabled":
		return vLower, true
	default:
		return "", false
	}
}

func lower(s string) string {
	b := make([]byte, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'A' && c <= 'Z' {
			c = c + ('a' - 'A')
		}
		b[i] = c
	}
	return string(b)
}
