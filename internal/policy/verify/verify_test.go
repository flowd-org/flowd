package verify_test

import (
	"context"
	"os/exec"
	"strings"
	"testing"

	verify "github.com/flowd-org/flowd/internal/policy/verify"
)

func TestCosignVerifier_EmptyImage(t *testing.T) {
	v := verify.NewCosignVerifier()
	_, err := v.Verify(context.Background(), "")
	if err == nil {
		t.Fatal("expected error for empty image reference")
	}
	if !strings.Contains(err.Error(), "image reference is required") {
		t.Errorf("expected error to contain 'image reference is required', got %v", err)
	}
}

func TestCosignVerifier_NonZeroExit(t *testing.T) {
	v := &verify.CosignVerifier{
		Command: func(ctx context.Context, name string, args ...string) *exec.Cmd {
			cmd := exec.CommandContext(ctx, "sh", "-c", "exit 1")
			return cmd
		},
	}
	result, err := v.Verify(context.Background(), "example.com/image:tag")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Verified {
		t.Error("expected Verified=false")
	}
	if !strings.Contains(result.Reason, "exit status 1") {
		t.Errorf("expected reason to contain exit status, got %q", result.Reason)
	}
}

func TestCosignVerifier_Success(t *testing.T) {
	v := &verify.CosignVerifier{
		Command: func(ctx context.Context, name string, args ...string) *exec.Cmd {
			cmd := exec.CommandContext(ctx, "sh", "-c", "echo verification passed")
			return cmd
		},
	}
	result, err := v.Verify(context.Background(), "example.com/image:tag")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Verified {
		t.Error("expected Verified=true")
	}
}

func TestCosignVerifier_ExecFailure(t *testing.T) {
	v := &verify.CosignVerifier{
		Command: func(ctx context.Context, name string, args ...string) *exec.Cmd {
			return exec.CommandContext(ctx, "/does/not/exist")
		},
	}
	_, err := v.Verify(context.Background(), "example.com/image:tag")
	if err == nil {
		t.Fatal("expected error for missing binary")
	}
	if !strings.Contains(err.Error(), "cosign execute") {
		t.Errorf("expected error to wrap cosign execute, got %v", err)
	}
}

func TestCosignBundleVerifier_EmptyRef(t *testing.T) {
	v := verify.NewCosignBundleVerifier()
	err := v.Verify(context.Background(), "")
	if err == nil {
		t.Fatal("expected error for empty ref")
	}
	if !strings.Contains(err.Error(), "bundle reference is required") {
		t.Errorf("expected error to contain 'bundle reference is required', got %v", err)
	}
}

func TestCosignBundleVerifier_NonZeroExit(t *testing.T) {
	v := &verify.CosignBundleVerifier{
		Command: func(ctx context.Context, name string, args ...string) *exec.Cmd {
			cmd := exec.CommandContext(ctx, "sh", "-c", "exit 1")
			return cmd
		},
	}
	err := v.Verify(context.Background(), "example.com/bundle:tag")
	if err == nil {
		t.Fatal("expected error for non-zero exit")
	}
	if !strings.Contains(err.Error(), "cosign verify failed") {
		t.Errorf("expected error to contain cosign verify failed, got %v", err)
	}
}

func TestCosignBundleVerifier_Success(t *testing.T) {
	v := &verify.CosignBundleVerifier{
		Command: func(ctx context.Context, name string, args ...string) *exec.Cmd {
			cmd := exec.CommandContext(ctx, "sh", "-c", "echo verification passed")
			return cmd
		},
	}
	err := v.Verify(context.Background(), "example.com/bundle:tag")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
