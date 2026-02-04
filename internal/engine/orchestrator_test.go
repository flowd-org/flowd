// SPDX-License-Identifier: AGPL-3.0-or-later
package engine

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/flowd-org/flowd/internal/events"
	"github.com/flowd-org/flowd/internal/types"
)

func TestOrchestratorStartRun_NonServerCallable(t *testing.T) {
	tmp := t.TempDir()
	cfgDir := filepath.Join(tmp, "config.d")
	if err := os.MkdirAll(cfgDir, 0o700); err != nil {
		t.Fatalf("mkdir config.d: %v", err)
	}
	configPath := filepath.Join(cfgDir, "config.yaml")
	if err := os.WriteFile(configPath, []byte(`interpreter: bash
argspec:
  args:
    - name: token
      type: string
      format: secret
`), 0o600); err != nil {
		t.Fatalf("write config.yaml: %v", err)
	}

	o := NewOrchestrator(OrchestratorDeps{RunIDGen: staticRunIDGen("run-test")})
	res, err := o.StartRun(context.Background(), types.StartRunRequest{
		JobID:     "jobs.demo",
		ScriptDir: tmp,
		Args:      map[string]interface{}{"token": "supersecret"},
	})
	if err != nil {
		t.Fatalf("StartRun error: %v", err)
	}
	if res.RunID != "run-test" {
		t.Fatalf("expected run id, got %q", res.RunID)
	}
	if res.Plan.JobID != "jobs.demo" {
		t.Fatalf("expected job id in plan, got %q", res.Plan.JobID)
	}
	if got := res.Plan.ResolvedArgs["token"]; got != events.SecretToken() {
		t.Fatalf("expected secret redacted, got %v", got)
	}
	if res.Binding == nil {
		t.Fatalf("expected binding")
	}
	if res.Binding.SecretBuffers == nil || len(res.Binding.SecretBuffers) == 0 {
		t.Fatalf("expected secret buffers populated")
	}
	if buf := res.Binding.SecretBuffers["token"]; buf == nil || buf.Len() == 0 {
		t.Fatalf("expected token buffer populated")
	}
	var decoded map[string]string
	if err := json.Unmarshal([]byte(res.Binding.ArgsJSON), &decoded); err != nil {
		t.Fatalf("decode ArgsJSON: %v", err)
	}
	if decoded["token"] != "$$REDACTED$$" {
		t.Fatalf("unexpected ArgsJSON values: %+v", decoded)
	}
	if len(res.SecretHandles) == 0 {
		t.Fatalf("expected secret handles")
	}
	if res.SecretCleanup == nil {
		t.Fatalf("expected secret cleanup")
	}
	if err := res.SecretCleanup(); err != nil {
		t.Fatalf("cleanup secret handles: %v", err)
	}
}

type staticRunIDGen string

func (s staticRunIDGen) NewRunID() string { return string(s) }
