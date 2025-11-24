package server

import (
	"testing"

	"github.com/flowd-org/flowd/internal/types"
)

func TestConfigNormalizeRuleYDefaults(t *testing.T) {
	cfg := Config{}
	norm := cfg.normalize()
	if len(norm.RuleY.Allowlist) != 2 {
		t.Fatalf("expected default allowlist entries, got %d", len(norm.RuleY.Allowlist))
	}
	if ns := norm.RuleY.Allowlist["core_triggers"].LimitBytes; ns != defaultRuleYLimitBytes {
		t.Fatalf("expected default limit %d for core_triggers, got %d", defaultRuleYLimitBytes, ns)
	}
	if ns := norm.RuleY.Allowlist["core_invocation_state"].LimitBytes; ns != defaultRuleYLimitBytes {
		t.Fatalf("expected default limit %d for core_invocation_state, got %d", defaultRuleYLimitBytes, ns)
	}
}

func TestConfigNormalizeRuleYCustomLimit(t *testing.T) {
	const custom = 8 << 20
	cfg := Config{
		RuleY: types.RuleYConfig{
			Allowlist: map[string]types.RuleYNamespaceConfig{
				"core_triggers": {LimitBytes: custom},
			},
		},
	}
	norm := cfg.normalize()
	if norm.RuleY.Allowlist["core_triggers"].LimitBytes != custom {
		t.Fatalf("expected limit %d, got %d", custom, norm.RuleY.Allowlist["core_triggers"].LimitBytes)
	}
}
