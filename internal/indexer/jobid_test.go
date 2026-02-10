// SPDX-License-Identifier: AGPL-3.0-or-later
package indexer

import (
	"errors"
	"strings"
	"testing"
)

func TestCanonicalJobIDNormalization(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name      string
		mountPath string
		jobDirRel string
		want      string
	}{
		{
			name:      "mixed-case and spaces",
			mountPath: ".",
			jobDirRel: "Foo/Bar Baz",
			want:      "foo/bar-baz",
		},
		{
			name:      "mountPath prefix normalized",
			mountPath: "Scripts",
			jobDirRel: "Jobs/Hello_World",
			want:      "scripts/jobs/hello-world",
		},
		{
			name:      "characters require normalization",
			mountPath: "root",
			jobDirRel: "demo@job/ci+run",
			want:      "root/demo-job/ci-run",
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := CanonicalJobID(tc.mountPath, tc.jobDirRel)
			if err != nil {
				t.Fatalf("CanonicalJobID error: %v", err)
			}
			if got != tc.want {
				t.Fatalf("expected %q, got %q", tc.want, got)
			}
			if strings.Contains(got, ".") {
				t.Fatalf("expected slash-only canonical ID, got %q", got)
			}
		})
	}
}

func TestCanonicalJobIDRootMapping(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name      string
		mountPath string
		jobDirRel string
		want      string
	}{
		{
			name:      "mountPath dot and job root",
			mountPath: ".",
			jobDirRel: ".",
			want:      "",
		},
		{
			name:      "mountPath dot and empty rel",
			mountPath: ".",
			jobDirRel: "",
			want:      "",
		},
		{
			name:      "mountPath prefix and job root",
			mountPath: "Scripts",
			jobDirRel: ".",
			want:      "scripts",
		},
		{
			name:      "mountPath prefix and empty rel",
			mountPath: "Scripts",
			jobDirRel: "",
			want:      "scripts",
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got, err := CanonicalJobID(tc.mountPath, tc.jobDirRel)
			if err != nil {
				t.Fatalf("CanonicalJobID error: %v", err)
			}
			if got != tc.want {
				t.Fatalf("expected %q, got %q", tc.want, got)
			}
		})
	}
}

func TestCanonicalJobIDInvalidSegments(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name      string
		mountPath string
		jobDirRel string
		wantErr   string
		wantSeg   string
	}{
		{
			name:      "empty segment",
			mountPath: ".",
			jobDirRel: "demo//child",
			wantErr:   "empty segment",
			wantSeg:   "",
		},
		{
			name:      "segment normalizes to empty",
			mountPath: "!!!",
			jobDirRel: "demo",
			wantErr:   "segment normalizes to empty",
			wantSeg:   "!!!",
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			_, err := CanonicalJobID(tc.mountPath, tc.jobDirRel)
			if err == nil {
				t.Fatalf("expected error")
			}
			var jobErr JobIDError
			if !errors.As(err, &jobErr) {
				t.Fatalf("expected JobIDError, got %T", err)
			}
			if tc.wantErr != "" && !strings.Contains(jobErr.Reason, tc.wantErr) {
				t.Fatalf("expected error to contain %q, got %q", tc.wantErr, jobErr.Reason)
			}
			if jobErr.Segment != tc.wantSeg {
				t.Fatalf("expected segment %q, got %q", tc.wantSeg, jobErr.Segment)
			}
		})
	}
}

func TestJobIDHelpers(t *testing.T) {
	t.Parallel()

	t.Run("dot to slash", func(t *testing.T) {
		t.Parallel()

		if got := DotJobIDToSlash("demo.build"); got != "demo/build" {
			t.Fatalf("expected demo/build, got %q", got)
		}
		if got := DotJobIDToSlash(" . "); got != "" {
			t.Fatalf("expected empty string, got %q", got)
		}
		if got := DotJobIDToSlash("Demo..Build"); got != "Demo/Build" {
			t.Fatalf("expected Demo/Build, got %q", got)
		}
		if got := DotJobIDToSlash("a...b"); got != "a/b" {
			t.Fatalf("expected a/b, got %q", got)
		}
		if got := DotJobIDToSlash("a///b"); got != "a/b" {
			t.Fatalf("expected a/b, got %q", got)
		}
		if got := DotJobIDToSlash("a././b"); got != "a/b" {
			t.Fatalf("expected a/b, got %q", got)
		}
	})

	t.Run("slash to dot", func(t *testing.T) {
		t.Parallel()

		if got := SlashJobIDToDot("demo/build"); got != "demo.build" {
			t.Fatalf("expected demo.build, got %q", got)
		}
		if got := SlashJobIDToDot("/"); got != "" {
			t.Fatalf("expected empty string, got %q", got)
		}
		if got := SlashJobIDToDot("demo//Build"); got != "demo.Build" {
			t.Fatalf("expected demo.Build, got %q", got)
		}
		if got := SlashJobIDToDot("a///b"); got != "a.b" {
			t.Fatalf("expected a.b, got %q", got)
		}
		if got := SlashJobIDToDot("a...b"); got != "a.b" {
			t.Fatalf("expected a.b, got %q", got)
		}
		if got := SlashJobIDToDot("a././b"); got != "a.b" {
			t.Fatalf("expected a.b, got %q", got)
		}
	})
}
