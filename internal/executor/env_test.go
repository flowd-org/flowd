package executor

import (
	"os"
	"testing"

	"github.com/flowd-org/flowd/internal/types"
)

func TestBuildEnv_StripsParentEnv(t *testing.T) {
	os.Setenv("UNSAFE_VAR", "value")
	defer os.Unsetenv("UNSAFE_VAR")

	cfg := &types.Config{Env: map[string]string{"PATH": "/usr/bin"}}
	argEnv := map[string]string{"ARG_NAME": "alice"}

	env := buildSecureEnv(cfg, argEnv, "{}", false)

	for _, e := range env {
		if len(e) >= len("UNSAFE_VAR=") && e[:len("UNSAFE_VAR=")] == "UNSAFE_VAR=" {
			t.Fatalf("unexpected unsafe env in secure env: %s", e)
		}
	}

	foundArg := false
	for _, e := range env {
		if e == "ARG_NAME=alice" {
			foundArg = true
			break
		}
	}
	if !foundArg {
		t.Fatalf("ARG_NAME missing from env")
	}
}

func TestBuildSecureEnvAddsDefaultPath(t *testing.T) {
	t.Setenv("PATH", "/usr/local/bin")
	env := buildSecureEnv(nil, nil, "", false)
	expect := "PATH=/usr/local/bin"
	found := false
	for _, e := range env {
		if e == expect {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected %s in env: %v", expect, env)
	}
}

func TestBuildSecureEnvPrefersConfigPath(t *testing.T) {
	cfg := &types.Config{Env: map[string]string{"PATH": "/custom/bin"}}
	env := buildSecureEnv(cfg, nil, "", false)
	count := 0
	for _, e := range env {
		if e == "PATH=/custom/bin" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("expected config PATH to be preserved once, got %d in %v", count, env)
	}
}

func TestBuildSecureEnvInheritHost(t *testing.T) {
	key := "INHERITED_VAR"
	val := "present"
	prev := os.Getenv(key)
	t.Setenv(key, val)
	defer os.Setenv(key, prev)

	cfg := &types.Config{}
	env := buildSecureEnv(cfg, nil, "", true)
	found := false
	for _, e := range env {
		if e == key+"="+val {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected inherited env %s in %v", key, env)
	}
}
