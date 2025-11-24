package engine

import (
	"testing"

	"github.com/flowd-org/flowd/internal/events"
	"github.com/flowd-org/flowd/internal/types"
)

func TestBuildPlanRedactsSecrets(t *testing.T) {
	spec := &types.ArgSpec{Args: []types.Arg{
		{Name: "regular", Type: "string"},
		{Name: "secret", Type: "string", Secret: true},
	}}
	cfg := &types.Config{Interpreter: "/bin/bash"}
	bind := &Binding{
		Values:       map[string]interface{}{"regular": "ok", "secret": "value"},
		SecretNames:  map[string]struct{}{"secret": {}},
		SecretValues: []string{"value"},
	}

	plan := BuildPlan("demo.job", cfg, spec, bind)

	if plan.JobID != "demo.job" {
		t.Fatalf("unexpected job id %s", plan.JobID)
	}
	if plan.ExecutorPreview["interpreter"] != "/bin/bash" {
		t.Fatalf("expected interpreter preview")
	}
	if plan.ResolvedArgs["regular"] != "ok" {
		t.Fatalf("expected regular arg")
	}
	if plan.ResolvedArgs["secret"] != events.SecretToken() {
		t.Fatalf("secret arg not redacted: %v", plan.ResolvedArgs["secret"])
	}
}
