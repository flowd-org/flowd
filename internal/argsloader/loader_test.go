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
