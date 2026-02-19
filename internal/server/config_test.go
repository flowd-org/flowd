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
	if ns := norm.RuleY.Allowlist["core_triggers"].MaxBytes; ns != defaultRuleYMaxBytes {
		t.Fatalf("expected default max_bytes %d for core_triggers, got %d", defaultRuleYMaxBytes, ns)
	}
	if ns := norm.RuleY.Allowlist["core_invocation_state"].MaxBytes; ns != defaultRuleYMaxBytes {
		t.Fatalf("expected default max_bytes %d for core_invocation_state, got %d", defaultRuleYMaxBytes, ns)
	}
	if rows := norm.RuleY.Allowlist["core_triggers"].MaxRows; rows != defaultRuleYMaxRows {
		t.Fatalf("expected default max_rows %d for core_triggers, got %d", defaultRuleYMaxRows, rows)
	}
}

func TestConfigNormalizeRuleYCustomQuota(t *testing.T) {
	const customBytes = 8 << 20
	const customRows = 123
	cfg := Config{
		RuleY: types.RuleYConfig{
			Allowlist: map[string]types.RuleYNamespaceConfig{
				"core_triggers": {MaxBytes: customBytes, MaxRows: customRows},
			},
		},
	}
	norm := cfg.normalize()
	if norm.RuleY.Allowlist["core_triggers"].MaxBytes != customBytes {
		t.Fatalf("expected max_bytes %d, got %d", customBytes, norm.RuleY.Allowlist["core_triggers"].MaxBytes)
	}
	if norm.RuleY.Allowlist["core_triggers"].MaxRows != customRows {
		t.Fatalf("expected max_rows %d, got %d", customRows, norm.RuleY.Allowlist["core_triggers"].MaxRows)
	}
}

func TestConfigNormalizeRuleYLegacyLimitBytesCompatibility(t *testing.T) {
	cfg := Config{
		RuleY: types.RuleYConfig{
			Allowlist: map[string]types.RuleYNamespaceConfig{
				"core_triggers": {LimitBytes: 2048},
			},
		},
	}
	norm := cfg.normalize()
	if norm.RuleY.Allowlist["core_triggers"].MaxBytes != 2048 {
		t.Fatalf("expected max_bytes 2048, got %d", norm.RuleY.Allowlist["core_triggers"].MaxBytes)
	}
	if norm.RuleY.Allowlist["core_triggers"].MaxRows != defaultRuleYMaxRows {
		t.Fatalf("expected default max_rows %d, got %d", defaultRuleYMaxRows, norm.RuleY.Allowlist["core_triggers"].MaxRows)
	}
}
