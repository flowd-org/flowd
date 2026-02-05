// SC-REF: SC701 (Phase 7 — Aliasing & Completion usability)
// Non-functional traceability tag for reviewer mapping.
package indexer

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/flowd-org/flowd/internal/configloader"
)

func TestDiscoverJobMetadata(t *testing.T) {
	root := t.TempDir()
	jobDir := filepath.Join(root, "demo", "hello")
	if err := os.MkdirAll(filepath.Join(jobDir, "config.d"), 0o755); err != nil {
		t.Fatal(err)
	}
	config := `version: 0.8
job:
  id: demo.hello
  name: Demo Hello
  summary: Say hello
`
	if err := os.WriteFile(filepath.Join(jobDir, "config.d", "config.yaml"), []byte(config), 0o644); err != nil {
		t.Fatal(err)
	}

	res, err := Discover(root)
	if err != nil {
		t.Fatalf("Discover error: %v", err)
	}
	if len(res.Errors) != 0 {
		t.Fatalf("unexpected errors: %+v", res.Errors)
	}
	if len(res.Jobs) != 1 {
		t.Fatalf("expected 1 job, got %d", len(res.Jobs))
	}
	job := res.Jobs[0]
	if job.ID != "demo/hello" {
		t.Fatalf("unexpected id %s", job.ID)
	}
	if res.AliasCollisions != nil {
		t.Fatalf("expected no alias collisions, got %+v", res.AliasCollisions)
	}
	if res.AliasInvalid != nil {
		t.Fatalf("expected no alias invalid entries, got %+v", res.AliasInvalid)
	}
	if job.Name != "Demo Hello" {
		t.Fatalf("unexpected name %s", job.Name)
	}
	if job.Summary != "Say hello" {
		t.Fatalf("unexpected summary %s", job.Summary)
	}
}

func TestDiscoverIncludesAliases(t *testing.T) {
	root := t.TempDir()
	scriptsDir := filepath.Join(root, "scripts")
	jobDir := filepath.Join(scriptsDir, "demo", "build")
	if err := os.MkdirAll(filepath.Join(jobDir, "config.d"), 0o755); err != nil {
		t.Fatal(err)
	}
	config := `version: v1
job:
  id: demo/build
  name: Demo Build
`
	if err := os.WriteFile(filepath.Join(jobDir, "config.d", "config.yaml"), []byte(config), 0o644); err != nil {
		t.Fatal(err)
	}
	flwd := `aliases:
  - from: "demo/build"
    to: "build-demo"
    description: "shortcut"
`
	if err := os.WriteFile(filepath.Join(scriptsDir, "flwd.yaml"), []byte(flwd), 0o644); err != nil {
		t.Fatal(err)
	}

	res, err := Discover(scriptsDir)
	if err != nil {
		t.Fatalf("Discover error: %v", err)
	}
	if len(res.Aliases) != 1 {
		t.Fatalf("expected 1 alias, got %d", len(res.Aliases))
	}
	alias := res.Aliases[0]
	if alias.Name != "build-demo" {
		t.Fatalf("expected alias name build-demo, got %s", alias.Name)
	}
	if alias.TargetID != "demo/build" {
		t.Fatalf("expected target id demo/build, got %s", alias.TargetID)
	}
	if alias.TargetPath != "demo/build" {
		t.Fatalf("expected target path demo/build, got %s", alias.TargetPath)
	}
	if alias.Description != "shortcut" {
		t.Fatalf("unexpected description: %q", alias.Description)
	}
}

func TestDiscoverInvalidAliasTarget(t *testing.T) {
	root := t.TempDir()
	scriptsDir := filepath.Join(root, "scripts")
	if err := os.MkdirAll(filepath.Join(scriptsDir, "flwd"), 0o755); err != nil {
		t.Fatal(err)
	}
	flwd := `aliases:
  - from: "unknown/job"
    to: "shortcut"
`
	if err := os.WriteFile(filepath.Join(scriptsDir, "flwd.yaml"), []byte(flwd), 0o644); err != nil {
		t.Fatal(err)
	}

	res, err := Discover(scriptsDir)
	if err != nil {
		t.Fatalf("discover error: %v", err)
	}
	if len(res.Aliases) != 0 {
		t.Fatalf("expected no aliases, got %d", len(res.Aliases))
	}
	if len(res.Errors) == 0 {
		t.Fatal("expected alias validation error")
	}
	if res.AliasInvalid == nil {
		t.Fatalf("expected alias invalid map populated")
	}
	validation, ok := res.AliasInvalid["shortcut"]
	if !ok {
		t.Fatalf("expected alias invalid entry for shortcut, got %+v", res.AliasInvalid)
	}
	if validation.Code != "alias.target.invalid" {
		t.Fatalf("expected alias.target.invalid, got %+v", validation)
	}
}

func TestDiscoverInvalidYaml(t *testing.T) {
	root := t.TempDir()
	jobDir := filepath.Join(root, "demo")
	if err := os.MkdirAll(filepath.Join(jobDir, "config.d"), 0o755); err != nil {
		t.Fatal(err)
	}
	bad := "job: ["
	if err := os.WriteFile(filepath.Join(jobDir, "config.d", "config.yaml"), []byte(bad), 0o644); err != nil {
		t.Fatal(err)
	}

	res, err := Discover(root)
	if err != nil {
		t.Fatalf("Discover error: %v", err)
	}
	if len(res.Errors) != 1 {
		t.Fatalf("expected 1 error, got %d", len(res.Errors))
	}
	if len(res.Jobs) != 0 {
		t.Fatalf("expected 0 jobs, got %d", len(res.Jobs))
	}
}

func TestDiscoverPrimaryAndLegacySentinels(t *testing.T) {
	root := t.TempDir()
	primaryDir := filepath.Join(root, "alpha", "one")
	if err := os.MkdirAll(primaryDir, 0o755); err != nil {
		t.Fatal(err)
	}
	primaryConfig := `version: v1
job:
  name: Alpha One
`
	if err := os.WriteFile(filepath.Join(primaryDir, "config.yaml"), []byte(primaryConfig), 0o644); err != nil {
		t.Fatal(err)
	}

	legacyDir := filepath.Join(root, "beta", "two", "config.d")
	if err := os.MkdirAll(legacyDir, 0o755); err != nil {
		t.Fatal(err)
	}
	legacyConfig := `version: v1
job:
  name: Beta Two
`
	if err := os.WriteFile(filepath.Join(legacyDir, "config.yaml"), []byte(legacyConfig), 0o644); err != nil {
		t.Fatal(err)
	}

	res, err := Discover(root)
	if err != nil {
		t.Fatalf("Discover error: %v", err)
	}
	if len(res.Errors) != 0 {
		t.Fatalf("unexpected errors: %+v", res.Errors)
	}
	if len(res.Jobs) != 2 {
		t.Fatalf("expected 2 jobs, got %d", len(res.Jobs))
	}
	ids := make(map[string]struct{}, len(res.Jobs))
	for _, job := range res.Jobs {
		ids[job.ID] = struct{}{}
	}
	for _, expected := range []string{"alpha/one", "beta/two"} {
		if _, ok := ids[expected]; !ok {
			t.Fatalf("expected job id %s in results: %+v", expected, res.Jobs)
		}
	}
}

func TestDiscoverDualConfigSentinelError(t *testing.T) {
	root := t.TempDir()
	jobDir := filepath.Join(root, "demo")
	if err := os.MkdirAll(filepath.Join(jobDir, "config.d"), 0o755); err != nil {
		t.Fatal(err)
	}
	config := `version: v1
job:
  name: Demo
`
	if err := os.WriteFile(filepath.Join(jobDir, "config.yaml"), []byte(config), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(jobDir, "config.d", "config.yaml"), []byte(config), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := Discover(root)
	if err == nil {
		t.Fatal("expected dual-config error, got nil")
	}
	var dual *configloader.DualConfigError
	if !errors.As(err, &dual) {
		t.Fatalf("expected DualConfigError, got %v", err)
	}
	if dual.PrimaryPath == "" || dual.LegacyPath == "" {
		t.Fatalf("expected dual config paths set, got %+v", dual)
	}
}

func TestDiscoverRootJobMapping(t *testing.T) {
	root := t.TempDir()
	rootConfig := `version: v1
job:
  name: Root Job
`
	if err := os.WriteFile(filepath.Join(root, "config.yaml"), []byte(rootConfig), 0o644); err != nil {
		t.Fatal(err)
	}

	toolDir := filepath.Join(root, "Tools")
	if err := os.MkdirAll(toolDir, 0o755); err != nil {
		t.Fatal(err)
	}
	toolConfig := `version: v1
job:
  name: Tools Job
`
	if err := os.WriteFile(filepath.Join(toolDir, "config.yaml"), []byte(toolConfig), 0o644); err != nil {
		t.Fatal(err)
	}

	res, err := Discover(root)
	if err != nil {
		t.Fatalf("Discover error: %v", err)
	}
	if len(res.Errors) != 0 {
		t.Fatalf("unexpected errors: %+v", res.Errors)
	}
	if len(res.Jobs) != 2 {
		t.Fatalf("expected 2 jobs, got %d", len(res.Jobs))
	}
	var (
		foundRoot bool
		foundTool bool
	)
	for _, job := range res.Jobs {
		switch job.ID {
		case "":
			foundRoot = true
			if job.Name != "Root Job" {
				t.Fatalf("expected root job name Root Job, got %s", job.Name)
			}
		case "tools":
			foundTool = true
			if job.Name != "Tools Job" {
				t.Fatalf("expected tools job name Tools Job, got %s", job.Name)
			}
		}
	}
	if !foundRoot {
		t.Fatalf("expected root job with empty id, got %+v", res.Jobs)
	}
	if !foundTool {
		t.Fatalf("expected tools job id, got %+v", res.Jobs)
	}
}
