//go:build !windows

// SPDX-License-Identifier: AGPL-3.0-or-later
// SC-REF: SC703 (Phase 7 — Completion performance thresholds)
package e2e

import (
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"

	flwdcmd "github.com/flowd-org/flowd/cmd"
	"github.com/spf13/cobra"
)

func TestCompletionLatencyThresholds(t *testing.T) {
	const (
		jobCount = 1000
		warmup   = 50
		calls    = 1200
	)

	workspace := setupBulkCompletionWorkspace(t, jobCount)
	scriptsDir := filepath.Join(workspace, "scripts")
	internal := prepareCompletionCommand(t, scriptsDir)

	for i := 0; i < warmup; i++ {
		runCompletionCall(t, internal, "2", "cmd")
	}

	durations := make([]time.Duration, 0, calls)
	for i := 0; i < calls; i++ {
		durations = append(durations, runCompletionCall(t, internal, "2", "cmd"))
	}

	sort.Slice(durations, func(i, j int) bool { return durations[i] < durations[j] })
	p50 := percentileDuration(durations, 0.50)
	p95 := percentileDuration(durations, 0.95)
	p99 := percentileDuration(durations, 0.99)

	if p50 > 25*time.Millisecond {
		t.Fatalf("p50 latency %s exceeds 25ms threshold", p50)
	}
	if p95 > 60*time.Millisecond {
		t.Fatalf("p95 latency %s exceeds 60ms threshold", p95)
	}
	if p99 > 120*time.Millisecond {
		t.Fatalf("p99 latency %s exceeds 120ms threshold", p99)
	}
}

func BenchmarkCompletionLatency(b *testing.B) {
	const jobCount = 1000
	workspace := setupBulkCompletionWorkspace(b, jobCount)
	scriptsDir := filepath.Join(workspace, "scripts")
	internal := prepareCompletionCommand(b, scriptsDir)
	for i := 0; i < 50; i++ {
		runCompletionCall(b, internal, "2", "cmd")
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		runCompletionCall(b, internal, "2", "cmd")
	}
}

func setupBulkCompletionWorkspace(tb testing.TB, count int) string {
	tb.Helper()
	dir, err := os.MkdirTemp("", "flwd-completion")
	if err != nil {
		tb.Fatalf("create temp workspace: %v", err)
	}
	tb.Cleanup(func() { _ = os.RemoveAll(dir) })

	scriptsDir := filepath.Join(dir, "scripts")
	if err := os.MkdirAll(scriptsDir, 0o755); err != nil {
		tb.Fatalf("mkdir scripts dir: %v", err)
	}

	for i := 0; i < count; i++ {
		name := fmt.Sprintf("cmd%04d", i)
		configDir := filepath.Join(scriptsDir, name, "config.d")
		if err := os.MkdirAll(configDir, 0o755); err != nil {
			tb.Fatalf("mkdir %s: %v", configDir, err)
		}
		config := fmt.Sprintf(`version: v1
job:
  id: %s
  name: Command %s
argspec:
  args:
    - name: mode
      type: string
      enum:
        - prod
        - staging
steps:
  - id: exec
    script: scripts/%s/run.sh
`, name, name, name)
		if err := os.WriteFile(filepath.Join(configDir, "config.yaml"), []byte(config), 0o644); err != nil {
			tb.Fatalf("write config for %s: %v", name, err)
		}
		scriptPath := filepath.Join(scriptsDir, name, "run.sh")
		if err := os.WriteFile(scriptPath, []byte("#!/usr/bin/env bash\nexit 0\n"), 0o755); err != nil {
			tb.Fatalf("write script for %s: %v", name, err)
		}
	}
	return dir
}

func prepareCompletionCommand(tb testing.TB, scriptsDir string) *cobra.Command {
	tb.Helper()
	root := &cobra.Command{Use: "flwd"}
	if err := flwdcmd.RegisterScriptCommands(root, scriptsDir); err != nil {
		tb.Fatalf("register script commands: %v", err)
	}
	internal := flwdcmd.NewInternalCompleteCmd(root)
	internal.SetOut(io.Discard)
	internal.SetErr(io.Discard)
	root.AddCommand(internal)
	return internal
}

func runCompletionCall(tb testing.TB, command *cobra.Command, args ...string) time.Duration {
	tb.Helper()
	start := time.Now()
	if err := command.RunE(command, args); err != nil {
		tb.Fatalf("completion resolve failed: %v", err)
	}
	return time.Since(start)
}

func percentileDuration(values []time.Duration, quantile float64) time.Duration {
	if len(values) == 0 {
		return 0
	}
	if quantile <= 0 {
		return values[0]
	}
	if quantile >= 1 {
		return values[len(values)-1]
	}
	index := int(math.Ceil(quantile*float64(len(values)))) - 1
	if index < 0 {
		index = 0
	}
	if index >= len(values) {
		index = len(values) - 1
	}
	return values[index]
}
