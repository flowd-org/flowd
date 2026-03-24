package container

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
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

func TestBuildEnv(t *testing.T) {
	// empty map
	nilResult := BuildEnv(nil)
	if nilResult != nil {
		t.Fatalf("expected nil for empty input, got %v", nilResult)
	}

	// single entry
	single := BuildEnv(map[string]string{"KEY": "value"})
	if len(single) != 1 || single[0] != "KEY=value" {
		t.Fatalf("expected [KEY=value], got %v", single)
	}

	// multiple entries, stable ordering
	multi := BuildEnv(map[string]string{
		"B": "2",
		"A": "1",
		"C": "3",
	})
	if len(multi) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(multi))
	}
	// sorted order
	expect := []string{"A=1", "B=2", "C=3"}
	for i := range expect {
		if multi[i] != expect[i] {
			t.Fatalf("entry %d: expected %q, got %q", i, expect[i], multi[i])
		}
	}
}

func TestStopContainer(t *testing.T) {
	// empty runtime or name returns nil
	if err := StopContainer(context.Background(), "", "test", 0); err != nil {
		t.Fatalf("expected nil for empty runtime, got %v", err)
	}
	if err := StopContainer(context.Background(), RuntimeDocker, "", 0); err != nil {
		t.Fatalf("expected nil for empty name, got %v", err)
	}

	// timeout normalization - use a fake runner that returns "not found" to simulate missing container
	originalRuntimeCommand := runtimeCommand
	runtimeCommand = func(ctx context.Context, runtime Runtime, args ...string) ([]byte, error) {
		return []byte("container test not found"), nil
	}
	defer func() { runtimeCommand = originalRuntimeCommand }()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // immediate cancellation
	if err := StopContainer(ctx, RuntimeDocker, "test", 0); err != nil {
		t.Fatalf("expected nil on canceled context for missing container, got %v", err)
	}
}

func TestKillContainer(t *testing.T) {
	// empty runtime or name returns nil
	if err := KillContainer(context.Background(), "", "test"); err != nil {
		t.Fatalf("expected nil for empty runtime, got %v", err)
	}
	if err := KillContainer(context.Background(), RuntimeDocker, ""); err != nil {
		t.Fatalf("expected nil for empty name, got %v", err)
	}

	// cancelled context with mocked runtimeCommand that returns "not found"
	originalRuntimeCommand := runtimeCommand
	runtimeCommand = func(ctx context.Context, runtime Runtime, args ...string) ([]byte, error) {
		return []byte("container test not found"), nil
	}
	defer func() { runtimeCommand = originalRuntimeCommand }()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := KillContainer(ctx, RuntimeDocker, "test"); err != nil {
		t.Fatalf("expected nil on canceled context for missing container, got %v", err)
	}
}

func TestRemoveContainer(t *testing.T) {
	// empty runtime or name returns nil
	if err := RemoveContainer(context.Background(), "", "test"); err != nil {
		t.Fatalf("expected nil for empty runtime, got %v", err)
	}
	if err := RemoveContainer(context.Background(), RuntimeDocker, ""); err != nil {
		t.Fatalf("expected nil for empty name, got %v", err)
	}

	// Podman --ignore flag with mocked runtimeCommand that returns "not found"
	originalRuntimeCommand := runtimeCommand
	runtimeCommand = func(ctx context.Context, runtime Runtime, args ...string) ([]byte, error) {
		return []byte("container test not found"), nil
	}
	defer func() { runtimeCommand = originalRuntimeCommand }()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := RemoveContainer(ctx, RuntimePodman, "test"); err != nil {
		t.Fatalf("expected nil on canceled context for missing container, got %v", err)
	}
}

func TestIsContainerNotFound(t *testing.T) {
	// Empty output
	if isContainerNotFound([]byte("")) {
		t.Fatalf("expected false for empty output")
	}

	// Podman "not found"
	if !isContainerNotFound([]byte("Error: container test not found")) {
		t.Fatalf("expected true for podman not found message")
	}

	// Docker "no such container"
	if !isContainerNotFound([]byte("Error: No such container: test")) {
		t.Fatalf("expected true for docker no such container message")
	}

	// Mixed case
	if !isContainerNotFound([]byte("error: NO SUCH CONTAINER: TEST")) {
		t.Fatalf("expected true for mixed case message")
	}
}

func TestStopContainer_Timeout(t *testing.T) {
	ctx := context.Background()

	originalRuntimeCommand := runtimeCommand
	runtimeCommand = func(ctx context.Context, runtime Runtime, args ...string) ([]byte, error) {
		return []byte(""), nil
	}
	defer func() { runtimeCommand = originalRuntimeCommand }()

	// Zero timeout should default to 10 seconds
	err := StopContainer(ctx, RuntimeDocker, "test", 0)
	if err != nil {
		t.Fatalf("expected nil for zero timeout, got %v", err)
	}

	// Negative timeout should also use default
	err = StopContainer(ctx, RuntimeDocker, "test", -5*time.Second)
	if err != nil {
		t.Fatalf("expected nil for negative timeout, got %v", err)
	}
}

func TestBackgroundContext(t *testing.T) {
	// With nil context should return Background
	ctx := backgroundContext(nil)
	if ctx == nil {
		t.Fatalf("expected non-nil context for nil input")
	}

	// With existing context should return same context
	orig := context.Background()
	result := backgroundContext(orig)
	if result != orig {
		t.Fatalf("expected same context, got different")
	}
}

func TestKillContainer_Success(t *testing.T) {
	// TestKillContainer_Success tests successful container kill
	originalRuntimeCommand := runtimeCommand
	defer func() { runtimeCommand = originalRuntimeCommand }()

	runtimeCommand = func(ctx context.Context, rt Runtime, args ...string) ([]byte, error) {
		return []byte(""), nil // success - container killed
	}

	ctx := context.Background()
	err := KillContainer(ctx, RuntimeDocker, "test-container")
	if err != nil {
		t.Fatalf("expected nil on successful kill, got %v", err)
	}
}

func TestRemoveContainer_SuccessWithPodman(t *testing.T) {
	// TestRemoveContainer_SuccessWithPodman tests successful container removal with podman runtime
	originalRuntimeCommand := runtimeCommand
	defer func() { runtimeCommand = originalRuntimeCommand }()

	runtimeCommand = func(ctx context.Context, rt Runtime, args ...string) ([]byte, error) {
		return []byte(""), nil // success - container removed
	}

	ctx := context.Background()
	err := RemoveContainer(ctx, RuntimePodman, "test-container")
	if err != nil {
		t.Fatalf("expected nil on successful removal with podman, got %v", err)
	}
}

func TestKillContainer_Error(t *testing.T) {
	// TestKillContainer_Error tests error path when kill fails with non-not-found error
	originalRuntimeCommand := runtimeCommand
	defer func() { runtimeCommand = originalRuntimeCommand }()

	runtimeCommand = func(ctx context.Context, rt Runtime, args ...string) ([]byte, error) {
		return nil, errors.New("permission denied") // actual error, not "not found"
	}

	ctx := context.Background()
	err := KillContainer(ctx, RuntimeDocker, "test-container")
	if err == nil {
		t.Fatalf("expected error on permission denied, got nil")
	}
	if !strings.Contains(err.Error(), "permission denied") {
		t.Fatalf("expected 'permission denied' in error, got %v", err)
	}
}

func TestRemoveContainer_Error(t *testing.T) {
	// TestRemoveContainer_Error tests error path when removal fails with non-not-found error
	originalRuntimeCommand := runtimeCommand
	defer func() { runtimeCommand = originalRuntimeCommand }()

	runtimeCommand = func(ctx context.Context, rt Runtime, args ...string) ([]byte, error) {
		return nil, errors.New("container is running") // actual error, not "not found"
	}

	ctx := context.Background()
	err := RemoveContainer(ctx, RuntimeDocker, "test-container")
	if err == nil {
		t.Fatalf("expected error on container running, got nil")
	}
	if !strings.Contains(err.Error(), "container is running") {
		t.Fatalf("expected 'container is running' in error, got %v", err)
	}
}
