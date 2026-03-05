package argsloader

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

const cfg = `interpreter: "/usr/bin/env bash"
argspec:
  args:
    - name: name
      type: string
      description: "name"
      required: true
    - name: loud
      type: boolean
      description: "loud"
      default: false
    - name: tags
      type: array
      items_type: string
      description: "tags"
    - name: meta
      type: object
      value_type: string
      description: "meta"
`

func TestAttachFlags_FromArgSpec(t *testing.T) {
	tmp := t.TempDir()
	if err := os.MkdirAll(filepath.Join(tmp, "config.d"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmp, "config.d", "config.yaml"), []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}

	cmd := &cobra.Command{Use: "demo"}
	if err := AttachFlags(cmd, tmp); err != nil {
		t.Fatalf("AttachFlags error: %v", err)
	}

	cases := map[string]string{
		"name": "string",
		"loud": "bool",
		"tags": "stringArray",
		"meta": "stringArray",
	}
	for flagName, wantType := range cases {
		f := cmd.Flags().Lookup(flagName)
		if f == nil {
			t.Fatalf("flag %s not registered", flagName)
		}
		if f.Value.Type() != wantType {
			t.Fatalf("flag %s type=%s want=%s", flagName, f.Value.Type(), wantType)
		}
	}

	if f := cmd.Flags().Lookup("name"); f != nil {
		if f.Annotations == nil {
			t.Fatalf("expected name flag annotations for required")
		}
		if _, ok := f.Annotations[cobra.BashCompOneRequiredFlag]; !ok {
			t.Fatalf("expected name flag to be marked required")
		}
	}

	// Ensure array accepts repeated values
	if err := cmd.Flags().Set("tags", "alpha"); err != nil {
		t.Fatalf("set tags: %v", err)
	}
	if err := cmd.Flags().Set("tags", "beta"); err != nil {
		t.Fatalf("set tags: %v", err)
	}
	gotTags, _ := cmd.Flags().GetStringArray("tags")
	if len(gotTags) != 2 || strings.Join(gotTags, ",") != "alpha,beta" {
		t.Fatalf("unexpected tags %v", gotTags)
	}
}

func TestAttachFlags_ConfigMissing(t *testing.T) {
	tmp := t.TempDir()
	cmd := &cobra.Command{Use: "demo"}
	err := AttachFlags(cmd, tmp)
	if err != nil {
		t.Fatalf("expected nil error when config missing, got %v", err)
	}
}

func TestAttachFlags_ArgSpecEmpty(t *testing.T) {
	tmp := t.TempDir()
	cfgPath := filepath.Join(tmp, "config.d")
	if err := os.MkdirAll(cfgPath, 0o755); err != nil {
		t.Fatal(err)
	}
	yaml := `interpreter: "/usr/bin/env bash"`
	if err := os.WriteFile(filepath.Join(cfgPath, "config.yaml"), []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}

	cmd := &cobra.Command{Use: "demo"}
	err := AttachFlags(cmd, tmp)
	if err != nil {
		t.Fatalf("expected nil when ArgSpec empty, got %v", err)
	}
}

func TestAttachFlags_InvalidArgName(t *testing.T) {
	tmp := t.TempDir()
	cfgPath := filepath.Join(tmp, "config.d")
	if err := os.MkdirAll(cfgPath, 0o755); err != nil {
		t.Fatal(err)
	}
	yaml := `interpreter: "/usr/bin/env bash"
argspec:
  args:
    - name: "-invalid"
      type: string
`
	if err := os.WriteFile(filepath.Join(cfgPath, "config.yaml"), []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}

	cmd := &cobra.Command{Use: "demo"}
	err := AttachFlags(cmd, tmp)
	if err == nil || !strings.Contains(err.Error(), "invalid argspec name") {
		t.Fatalf("expected error for invalid arg name, got %v", err)
	}
}

func TestAttachFlags_IntegerDefault(t *testing.T) {
	tmp := t.TempDir()
	cfgPath := filepath.Join(tmp, "config.d")
	if err := os.MkdirAll(cfgPath, 0o755); err != nil {
		t.Fatal(err)
	}
	yaml := `interpreter: "/usr/bin/env bash"
argspec:
  args:
    - name: count
      type: integer
      default: 42
`
	if err := os.WriteFile(filepath.Join(cfgPath, "config.yaml"), []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}

	cmd := &cobra.Command{Use: "demo"}
	err := AttachFlags(cmd, tmp)
	if err != nil {
		t.Fatalf("AttachFlags error: %v", err)
	}
	f := cmd.Flags().Lookup("count")
	if f == nil {
		t.Fatalf("flag count not registered")
	}
	val, _ := cmd.Flags().GetInt("count")
	if val != 42 {
		t.Fatalf("expected default 42, got %d", val)
	}
}

func TestAttachFlags_ArrayAndObject(t *testing.T) {
	tmp := t.TempDir()
	cfgPath := filepath.Join(tmp, "config.d")
	if err := os.MkdirAll(cfgPath, 0o755); err != nil {
		t.Fatal(err)
	}
	yaml := `interpreter: "/usr/bin/env bash"
argspec:
  args:
    - name: tags
      type: array
    - name: meta
      type: object
`
	if err := os.WriteFile(filepath.Join(cfgPath, "config.yaml"), []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}

	cmd := &cobra.Command{Use: "demo"}
	err := AttachFlags(cmd, tmp)
	if err != nil {
		t.Fatalf("AttachFlags error: %v", err)
	}
	for _, name := range []string{"tags", "meta"} {
		f := cmd.Flags().Lookup(name)
		if f == nil {
			t.Fatalf("flag %s not registered", name)
		}
		if f.Value.Type() != "stringArray" {
			t.Fatalf("flag %s type=%s want=stringArray", name, f.Value.Type())
		}
	}
}

func TestAttachFlags_UnsupportedType(t *testing.T) {
	tmp := t.TempDir()
	cfgPath := filepath.Join(tmp, "config.d")
	if err := os.MkdirAll(cfgPath, 0o755); err != nil {
		t.Fatal(err)
	}
	yaml := `interpreter: "/usr/bin/env bash"
argspec:
  args:
    - name: custom
      type: custom
`
	if err := os.WriteFile(filepath.Join(cfgPath, "config.yaml"), []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}

	cmd := &cobra.Command{Use: "demo"}
	err := AttachFlags(cmd, tmp)
	if err == nil || !strings.Contains(err.Error(), "unsupported arg type") {
		t.Fatalf("expected error for unsupported type, got %v", err)
	}
}

func TestAttachFlags_EnumCompletion(t *testing.T) {
	tmp := t.TempDir()
	cfgPath := filepath.Join(tmp, "config.d")
	if err := os.MkdirAll(cfgPath, 0o755); err != nil {
		t.Fatal(err)
	}
	yaml := `interpreter: "/usr/bin/env bash"
argspec:
  args:
    - name: mode
      type: string
      enum: [quick, full]
`
	if err := os.WriteFile(filepath.Join(cfgPath, "config.yaml"), []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}

	cmd := &cobra.Command{Use: "demo"}
	err := AttachFlags(cmd, tmp)
	if err != nil {
		t.Fatalf("AttachFlags error: %v", err)
	}
	f := cmd.Flags().Lookup("mode")
	if f == nil {
		t.Fatalf("flag mode not registered")
	}
	if f.Value.Type() != "string" {
		t.Fatalf("expected string type, got %s", f.Value.Type())
	}
}
