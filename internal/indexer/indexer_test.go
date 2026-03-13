// SC-REF: SC701 (Phase 7 — Aliasing & Completion usability)
// Non-functional traceability tag for reviewer mapping.
package indexer

import (
	"errors"
	"github.com/flowd-org/flowd/internal/configloader"
	"github.com/flowd-org/flowd/internal/types"
	"os"
	"path/filepath"
	"strings"
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

func TestDiscoverInvalidJobIDSegment(t *testing.T) {
	root := t.TempDir()
	jobDir := filepath.Join(root, "!!!")
	if err := os.MkdirAll(jobDir, 0o755); err != nil {
		t.Fatal(err)
	}
	config := `version: v1
job:
  name: Bad Job
`
	if err := os.WriteFile(filepath.Join(jobDir, "config.yaml"), []byte(config), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := Discover(root)
	if err == nil {
		t.Fatal("expected invalid job id error, got nil")
	}
	var invalid InvalidJobIDError
	if !errors.As(err, &invalid) {
		t.Fatalf("expected InvalidJobIDError, got %v", err)
	}
	if invalid.Segment == "" {
		t.Fatalf("expected invalid segment to be set")
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

func TestDiscoverWithMountPathPrefix(t *testing.T) {
	root := t.TempDir()
	jobDir := filepath.Join(root, "Demo")
	if err := os.MkdirAll(jobDir, 0o755); err != nil {
		t.Fatal(err)
	}
	config := `version: v1
job:
  name: Demo
`
	if err := os.WriteFile(filepath.Join(jobDir, "config.yaml"), []byte(config), 0o644); err != nil {
		t.Fatal(err)
	}

	res, err := DiscoverWithMountPath(root, "Scripts")
	if err != nil {
		t.Fatalf("DiscoverWithMountPath error: %v", err)
	}
	if len(res.Errors) != 0 {
		t.Fatalf("unexpected errors: %+v", res.Errors)
	}
	if len(res.Jobs) != 1 {
		t.Fatalf("expected 1 job, got %d", len(res.Jobs))
	}
	if res.Jobs[0].ID != "scripts/demo" {
		t.Fatalf("expected prefixed job id scripts/demo, got %s", res.Jobs[0].ID)
	}
}

func TestBuildAliasIndexDuplicatesAndConflicts(t *testing.T) {
	root := t.TempDir()
	scriptsDir := filepath.Join(root, "scripts")
	jobDir := filepath.Join(scriptsDir, "demo", "test")
	if err := os.MkdirAll(filepath.Join(jobDir, "config.d"), 0o755); err != nil {
		t.Fatal(err)
	}
	config := `version: v1
job:
  id: demo/test
  name: Demo Test
`
	if err := os.WriteFile(filepath.Join(jobDir, "config.d", "config.yaml"), []byte(config), 0o644); err != nil {
		t.Fatal(err)
	}

	flwd := `aliases:
  - from: "demo/test"
    to: "test-demo"
    description: "first alias"
  - from: "demo/test"
    to: "test-demo"
    description: "duplicate alias"
`
	if err := os.WriteFile(filepath.Join(scriptsDir, "flwd.yaml"), []byte(flwd), 0o644); err != nil {
		t.Fatal(err)
	}

	res, err := Discover(scriptsDir)
	if err != nil {
		t.Fatalf("Discover error: %v", err)
	}
	if len(res.Aliases) != 1 {
		t.Fatalf("expected 1 alias (first declaration), got %d", len(res.Aliases))
	}
	// Identical duplicates should not create collisions; only first is kept
	if len(res.AliasCollisions) != 0 {
		t.Fatalf("expected no collisions for identical duplicates, got %+v", res.AliasCollisions)
	}

	flwdCollision := `aliases:
  - from: "demo/test"
    to: "test-alias"
    description: "first alias"
  - from: "demo/test2"
    to: "test-alias"
    description: "second alias (collision)"
`
	jobDir2 := filepath.Join(scriptsDir, "demo", "test2")
	if err := os.MkdirAll(filepath.Join(jobDir2, "config.d"), 0o755); err != nil {
		t.Fatal(err)
	}
	config2 := `version: v1
job:
  id: demo/test2
  name: Demo Test2
`
	if err := os.WriteFile(filepath.Join(jobDir2, "config.d", "config.yaml"), []byte(config2), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(scriptsDir, "flwd.yaml"), []byte(flwdCollision), 0o644); err != nil {
		t.Fatal(err)
	}

	res3, err := Discover(scriptsDir)
	if err != nil {
		t.Fatalf("Discover error: %v", err)
	}
	if len(res3.Aliases) != 1 {
		t.Fatalf("expected 1 alias (first declaration), got %d", len(res3.Aliases))
	}
	if res3.AliasCollisions == nil || len(res3.AliasCollisions) != 1 {
		t.Fatalf("expected collision for duplicate alias names, got %+v", res3.AliasCollisions)
	}
	if _, ok := res3.AliasCollisions["test-alias"]; !ok {
		t.Fatalf("expected collision under key test-alias")
	}

	flwdConflict := `aliases:
  - from: "demo/test"
    to: "demotest"
`
	if err := os.WriteFile(filepath.Join(scriptsDir, "flwd.yaml"), []byte(flwdConflict), 0o644); err != nil {
		t.Fatal(err)
	}

	res2, err := Discover(scriptsDir)
	if err != nil {
		t.Fatalf("Discover error: %v", err)
	}
	// The alias name "demotest" doesn't conflict with job ID "demo/test"
	// so we expect 1 valid alias
	if len(res2.Aliases) != 1 {
		t.Fatalf("expected 1 alias, got %d", len(res2.Aliases))
	}
	if res2.Aliases[0].Name != "demotest" {
		t.Fatalf("expected alias name 'demotest', got '%s'", res2.Aliases[0].Name)
	}
}

func TestParseConfigEmptyJobsBlock(t *testing.T) {
	root := t.TempDir()
	jobDir := filepath.Join(root, "empty")
	if err := os.MkdirAll(jobDir, 0o755); err != nil {
		t.Fatal(err)
	}
	config := `version: v1
jobs:
`
	if err := os.WriteFile(filepath.Join(jobDir, "config.yaml"), []byte(config), 0o644); err != nil {
		t.Fatal(err)
	}

	jobs, err := parseConfig(root, jobDir, filepath.Join(jobDir, "config.yaml"), ".")
	if err != nil {
		t.Fatalf("parseConfig error: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("expected 1 job with empty jobs block, got %d", len(jobs))
	}
	if jobs[0].Name != "empty" {
		t.Fatalf("expected name to match canonical ID, got %s", jobs[0].Name)
	}
}

func TestCanonicalJobIDFromDirEdgeCases(t *testing.T) {
	tests := []struct {
		name      string
		root      string
		jobDir    string
		mountPath string
		wantID    string
		wantErr   bool
	}{
		{
			name:      "root directory",
			root:      "/repo",
			jobDir:    "/repo",
			mountPath: ".",
			wantID:    "",
			wantErr:   false,
		},
		{
			name:      "with mount path prefix",
			root:      "/repo",
			jobDir:    filepath.Join("/repo", "scripts", "demo"),
			mountPath: ".",
			wantID:    "scripts/demo",
			wantErr:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := canonicalJobIDFromDir(tt.root, tt.jobDir, tt.mountPath)
			if (err != nil) != tt.wantErr {
				t.Fatalf("canonicalJobIDFromDir() error = %v, wantErr %v", err, tt.wantErr)
			}
			if got != tt.wantID {
				t.Errorf("canonicalJobIDFromDir() = %v, want %v", got, tt.wantID)
			}
		})
	}
}

func TestNormalizeSegmentEdgeCases(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{
			name:    "empty string",
			input:   "",
			want:    "",
			wantErr: true,
		},
		{
			name:    "whitespace only",
			input:   "   ",
			want:    "",
			wantErr: true,
		},
		{
			name:    "starts with dash",
			input:   "-test",
			want:    "test",
			wantErr: false,
		},
		{
			name:    "ends with dash",
			input:   "test-",
			want:    "test",
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := normalizeSegment(tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("normalizeSegment() error = %v, wantErr %v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("normalizeSegment() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNormalizeAliasTarget(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		input    string
		wantPath string
		wantID   string
	}{
		{"empty input", "", "", ""},
		{"whitespace only", "   ", "", ""},
		{"simple path", "scripts/demo", "scripts/demo", "scripts/demo"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotPath, gotID := normalizeAliasTarget(tt.input)
			if gotPath != tt.wantPath || gotID != tt.wantID {
				t.Errorf("normalizeAliasTarget(%q) = (%q, %q), want (%q, %q)", tt.input, gotPath, gotID, tt.wantPath, tt.wantID)
			}
		})
	}
}

func TestJobIDErrorFormatting(t *testing.T) {
	t.Parallel()
	err1 := JobIDError{Segment: "abc", Reason: "invalid"}
	if err1.Error() != `invalid job id segment "abc": invalid` {
		t.Errorf("JobIDError.Error() = %q, want %q", err1.Error(), `invalid job id segment "abc": invalid`)
	}

	err2 := JobIDError{Path: "path/to", Segment: "def", Reason: "bad"}
	if err2.Error() != `invalid job id segment "def" in "path/to": bad` {
		t.Errorf("JobIDError with path = %q", err2.Error())
	}

	err3 := InvalidJobIDError{Path: "path", Segment: "seg", Reason: "reason"}
	if err3.Error() != `invalid job id segment "seg" in "path": reason` {
		t.Errorf("InvalidJobIDError without JobDir = %q", err3.Error())
	}

	err4 := InvalidJobIDError{JobDir: "jobs/test", Path: "path", Segment: "seg", Reason: "reason"}
	if !strings.Contains(err4.Error(), "(job dir jobs/test)") {
		t.Errorf("InvalidJobIDError with JobDir missing job dir info: %q", err4.Error())
	}

	var unwrapped error = InvalidJobIDError{Path: "p", Segment: "s", Reason: "r"}
	if _, ok := unwrapped.(interface{ Unwrap() error }); !ok {
		t.Errorf("InvalidJobIDError should implement Unwrap")
	}
}

func TestInvalidJobIDErrorUnwrap(t *testing.T) {
	t.Parallel()
	err := InvalidJobIDError{Path: "path", Segment: "seg", Reason: "reason"}
	var unwrapped error = err.Unwrap()
	if _, ok := unwrapped.(JobIDError); !ok {
		t.Errorf("InvalidJobIDError.Unwrap() did not return JobIDError")
	}
}

func TestBuildAliasIndex(t *testing.T) {
	t.Parallel()
	jobs := []JobInfo{
		{ID: "scripts/demo", Name: "Demo Script", Path: "/jobs/scripts/demo"},
	}
	tests := []struct {
		name     string
		aliasSet AliasSet
		wantErr  bool
	}{
		{"empty alias sets", AliasSet{}, false},
		{"valid alias", AliasSet{
			Source:  "test.yaml",
			Aliases: []types.CommandAlias{{From: "scripts/demo", To: "demo"}},
		}, false},
		{"invalid from (empty)", AliasSet{
			Source:  "test.yaml",
			Aliases: []types.CommandAlias{{From: "", To: "demo"}},
		}, true},
		{"invalid to (empty)", AliasSet{
			Source:  "test.yaml",
			Aliases: []types.CommandAlias{{From: "scripts/demo", To: ""}},
		}, true},
		{"invalid target path", AliasSet{
			Source:  "test.yaml",
			Aliases: []types.CommandAlias{{From: "nonexistent/path", To: "demo"}},
		}, true},
		{"alias name with slash", AliasSet{
			Source:  "test.yaml",
			Aliases: []types.CommandAlias{{From: "scripts/demo", To: "foo/bar"}},
		}, true},
		{"alias name starts with colon", AliasSet{
			Source:  "test.yaml",
			Aliases: []types.CommandAlias{{From: "scripts/demo", To: ":reserved"}},
		}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, errs := BuildAliasIndex(jobs, []AliasSet{tt.aliasSet})
			if (len(errs) > 0) != tt.wantErr {
				t.Errorf("BuildAliasIndex() errors = %v, wantErr %v", len(errs), tt.wantErr)
			}
		})
	}
}
