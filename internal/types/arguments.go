// SPDX-License-Identifier: AGPL-3.0-or-later
package types

// Legacy/compat definition for simple maps in config.yaml
type ArgumentDefinition struct {
	Type        string      `yaml:"type"`
	Default     interface{} `yaml:"default,omitempty"`
	Required    bool        `yaml:"required,omitempty"`
	Description string      `yaml:"description,omitempty"`
	Choices     []string    `yaml:"choices,omitempty"`
}

// Arg is the SOT-aligned argument schema (subset for Phase 1)
// Types: string | integer | boolean | array | object (value_type)
// Formats: path | file | directory | secret
type Arg struct {
	Name        string      `yaml:"name" json:"name"`
	Type        string      `yaml:"type" json:"type"`
	Format      string      `yaml:"format,omitempty" json:"format,omitempty"`
	Secret      bool        `yaml:"secret,omitempty" json:"secret,omitempty"`
	Required    bool        `yaml:"required,omitempty" json:"required,omitempty"`
	Default     interface{} `yaml:"default,omitempty" json:"default,omitempty"`
	Description string      `yaml:"description,omitempty" json:"description,omitempty"`
	Enum        []string    `yaml:"enum,omitempty" json:"enum,omitempty"`
	ItemsType   string      `yaml:"items_type,omitempty" json:"items_type,omitempty"`
	ItemsEnum   []string    `yaml:"items_enum,omitempty" json:"items_enum,omitempty"`
	ValueType   string      `yaml:"value_type,omitempty" json:"value_type,omitempty"`
}

type ArgSpec struct {
	Args []Arg `yaml:"args" json:"args"`
}
