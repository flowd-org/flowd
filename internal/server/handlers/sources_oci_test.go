// SPDX-License-Identifier: AGPL-3.0-or-later
package handlers

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/flowd-org/flowd/internal/executor/container"
	"github.com/flowd-org/flowd/internal/paths"
)

func TestExtractAddonManifestProfileFlagsSecure(t *testing.T) {
	assertManifestFlags(t, "secure", "on-add", func(args []string) {
		joined := strings.Join(args, " ")
		if !strings.Contains(joined, "--pull=always") {
			t.Fatalf("expected pull=always, got %v", args)
		}
		if !strings.Contains(joined, "--network none") {
			t.Fatalf("expected network none, got %v", args)
		}
		if !strings.Contains(joined, "--read-only") {
			t.Fatalf("expected read-only flag, got %v", args)
		}
	})
}

func TestExtractAddonManifestProfileFlagsPermissive(t *testing.T) {
	assertManifestFlags(t, "permissive", "on-add", func(args []string) {
		joined := strings.Join(args, " ")
		if !strings.Contains(joined, "--network none") || !strings.Contains(joined, "--read-only") {
			t.Fatalf("expected secure defaults, got %v", args)
		}
	})
}

func TestExtractAddonManifestProfileFlagsDisabled(t *testing.T) {
	assertManifestFlags(t, "disabled", "on-add", func(args []string) {
		joined := strings.Join(args, " ")
		if strings.Contains(joined, "--read-only") {
			t.Fatalf("did not expect read-only flag, got %v", args)
		}
		if !strings.Contains(joined, "--network bridge") {
			t.Fatalf("expected network bridge, got %v", args)
		}
	})
}

func TestExtractAddonManifestPullPolicyOnRun(t *testing.T) {
	assertManifestFlags(t, "secure", "on-run", func(args []string) {
		joined := strings.Join(args, " ")
		if !strings.Contains(joined, "--pull=never") {
			t.Fatalf("expected pull=never, got %v", args)
		}
	})
}

func assertManifestFlags(t *testing.T, profile, pullPolicy string, assertions func([]string)) {
	t.Helper()
	var captured []string
	withOCIRuntimeStub(t, func(ctx context.Context, runtime container.Runtime, args ...string) ([]byte, error) {
		captured = append([]string{}, args...)
		return []byte("apiVersion: flwd.addon/v1\nkind: AddOn\nmetadata:\n  name: demo\n  id: demo.addon\n  version: 1.0.0\nrequires: {}\njobs: []\n"), nil
	})
	if _, err := extractAddonManifest(context.Background(), container.Runtime("podman"), "ghcr.io/example/addon:1.0", profile, pullPolicy); err != nil {
		t.Fatalf("extractAddonManifest failed: %v", err)
	}
	if len(captured) == 0 {
		t.Fatalf("expected runtime invocation to be captured")
	}
	assertions(captured)
}

func TestWriteAddonManifestRejectsTraversal(t *testing.T) {
	tmp := t.TempDir()
	prev := paths.DataDir()
	paths.SetDataDirOverride(tmp)
	t.Cleanup(func() {
		paths.SetDataDirOverride(prev)
		os.RemoveAll(tmp)
	})
	if _, err := writeAddonManifest(paths.OCICacheDir(), "../evil", []byte("data")); err == nil {
		t.Fatalf("expected traversal to fail")
	}
}
