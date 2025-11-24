// SC-REF: SC701 (Phase 7 — Aliasing & Completion usability)
// Non-functional traceability tag for reviewer mapping.
package indexer

import (
	"os"
	"path/filepath"
	"testing"
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
	if job.ID != "demo.hello" {
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
	jobDir := filepath.Join(scriptsDir, "demo")
	if err := os.MkdirAll(filepath.Join(jobDir, "config.d"), 0o755); err != nil {
		t.Fatal(err)
	}
	config := `version: v1
job:
  id: demo.build
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
	if alias.TargetID != "demo.build" {
		t.Fatalf("expected target id demo.build, got %s", alias.TargetID)
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
