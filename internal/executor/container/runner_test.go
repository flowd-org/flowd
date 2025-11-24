package container

import (
	"errors"
	"testing"
)

func TestDetectRuntimePrefersPodman(t *testing.T) {
	lookups := map[string]string{
		"podman": "/bin/podman",
		"docker": "/bin/docker",
	}
	runtime, err := DetectRuntime(func(cmd string) (string, error) {
		if path, ok := lookups[cmd]; ok {
			return path, nil
		}
		return "", errors.New("not found")
	})
	if err != nil {
		t.Fatalf("expected runtime detection, got error %v", err)
	}
	if runtime != RuntimePodman {
		t.Fatalf("expected podman runtime, got %s", runtime)
	}
}

func TestDetectRuntimeFallbackDocker(t *testing.T) {
	runtime, err := DetectRuntime(func(cmd string) (string, error) {
		if cmd == "docker" {
			return "/bin/docker", nil
		}
		return "", errors.New("missing")
	})
	if err != nil {
		t.Fatalf("expected detection, got %v", err)
	}
	if runtime != RuntimeDocker {
		t.Fatalf("expected docker fallback, got %s", runtime)
	}
}

func TestDetectRuntimeError(t *testing.T) {
	_, err := DetectRuntime(func(cmd string) (string, error) {
		return "", errors.New("missing")
	})
	if err == nil {
		t.Fatalf("expected error when no runtime available")
	}
}

func TestBuildArgsSecureDefaults(t *testing.T) {
	opts := RunOptions{
		Runtime: RuntimeDocker,
		Image:   "alpine:3.20",
		Command: []string{"echo", "hello"},
		Env: map[string]string{
			"FOO": "bar",
		},
		Mounts: []Mount{
			{Source: "/tmp/host", Destination: "/work", ReadOnly: true},
		},
		Remove: true,
	}
	args, err := BuildArgs(opts)
	if err != nil {
		t.Fatalf("build args: %v", err)
	}
    expect := []string{
        "docker",
        "run",
        "--rm",
        "--cap-drop=ALL",
		"--security-opt=no-new-privileges",
		"--read-only",
		"--network", "none",
		"--env", "FOO=bar",
		"--volume", "/tmp/host:/work:ro",
		"alpine:3.20",
		"echo", "hello",
	}
	if !containsSequence(args, expect) {
		t.Fatalf("expected args to contain %v, got %v", expect, args)
	}
}

func TestBuildArgsValidation(t *testing.T) {
	_, err := BuildArgs(RunOptions{Runtime: RuntimeDocker, Image: "", Command: []string{"sh"}})
	if err == nil {
		t.Fatalf("expected error when image missing")
	}
	_, err = BuildArgs(RunOptions{Runtime: "", Image: "busybox"})
	if err == nil {
		t.Fatalf("expected error when runtime missing")
	}
	_, err = BuildArgs(RunOptions{
		Runtime: RuntimeDocker,
		Image:   "busybox",
		Mounts:  []Mount{{Source: "/tmp", Destination: "relative"}},
	})
	if err == nil {
		t.Fatalf("expected error for invalid mount destination")
	}
}

func TestBuildArgsOverrides(t *testing.T) {
	opts := RunOptions{
		Runtime:        RuntimeDocker,
		Image:          "alpine:3.18",
		Command:        []string{"sleep", "1"},
		WritableRootfs: true,
		Capabilities:   []string{"NET_ADMIN", "cap_sys_time"},
		NetworkMode:    "bridge",
	}
	args, err := BuildArgs(opts)
	if err != nil {
		t.Fatalf("build args: %v", err)
	}
	for _, flag := range []string{"--read-only"} {
		for _, arg := range args {
			if arg == flag {
				t.Fatalf("did not expect %s when writable rootfs requested", flag)
			}
		}
	}
	if !containsSequence(args, []string{"--network", "bridge"}) {
		t.Fatalf("expected network override in args: %v", args)
	}
	if !containsSequence(args, []string{"--cap-add=NET_ADMIN"}) {
		t.Fatalf("expected cap-add for NET_ADMIN: %v", args)
	}
	if !containsSequence(args, []string{"--cap-add=cap_sys_time"}) {
		t.Fatalf("expected cap-add for cap_sys_time: %v", args)
	}
}

func containsSequence(args, expect []string) bool {
outer:
	for i := 0; i < len(args); i++ {
		if args[i] != expect[0] {
			continue
		}
		if len(expect) > len(args)-i {
			return false
		}
		for j := range expect {
			if args[i+j] != expect[j] {
				continue outer
			}
		}
		return true
	}
	return false
}
