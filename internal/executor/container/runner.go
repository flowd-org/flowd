// SPDX-License-Identifier: AGPL-3.0-or-later
package container

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Runtime represents a supported container runtime CLI.
type Runtime string

const (
	RuntimePodman Runtime = "podman"
	RuntimeDocker Runtime = "docker"
)

// DetectRuntime returns the preferred available runtime, preferring Podman.
func DetectRuntime(lookPath func(string) (string, error)) (Runtime, error) {
	if lookPath == nil {
		lookPath = func(cmd string) (string, error) {
			return execLookPath(cmd)
		}
	}
	if _, err := lookPath(string(RuntimePodman)); err == nil {
		return RuntimePodman, nil
	}
	if _, err := lookPath(string(RuntimeDocker)); err == nil {
		return RuntimeDocker, nil
	}
	return "", fmt.Errorf("no supported container runtime found (podman or docker)")
}

// RunOptions encapsulates container execution parameters.
type RunOptions struct {
	Image          string
	Command        []string
	Env            map[string]string
	WorkDir        string
	Mounts         []Mount
	NetworkMode    string
	Name           string
	Runtime        Runtime
	StdoutPath     string
	StderrPath     string
	ExtraArgs      []string
	Remove         bool
	Interactive    bool
	WritableRootfs bool
	Capabilities   []string
}

// Mount describes a bind mount from host to container.
type Mount struct {
	Source      string
	Destination string
	ReadOnly    bool
}

// BuildArgs builds container runtime arguments enforcing secure defaults.
func BuildArgs(opts RunOptions) ([]string, error) {
	if opts.Image == "" {
		return nil, fmt.Errorf("image is required")
	}
	if opts.Runtime == "" {
		return nil, fmt.Errorf("runtime is required")
	}

	args := []string{string(opts.Runtime), "run"}
	if opts.Remove {
		args = append(args, "--rm")
	}
	if opts.Name != "" {
		args = append(args, "--name", opts.Name)
	}

	// Secure defaults
	args = append(args,
		"--cap-drop=ALL",
		"--security-opt=no-new-privileges",
	)
	if !opts.WritableRootfs {
		args = append(args, "--read-only")
	}

	networkMode := opts.NetworkMode
	if networkMode == "" {
		networkMode = "none"
	}
	args = append(args, "--network", networkMode)

	for _, cap := range opts.Capabilities {
		cap = strings.TrimSpace(cap)
		if cap == "" {
			continue
		}
		args = append(args, "--cap-add="+cap)
	}

	if opts.WorkDir != "" {
		args = append(args, "--workdir", opts.WorkDir)
	}

	for key, val := range opts.Env {
		args = append(args, "--env", fmt.Sprintf("%s=%s", key, val))
	}

	for _, m := range opts.Mounts {
		if err := validateMount(m); err != nil {
			return nil, err
		}
		mode := "rw"
		if m.ReadOnly {
			mode = "ro"
		}
		mountArg := fmt.Sprintf("%s:%s:%s", m.Source, m.Destination, mode)
		args = append(args, "--volume", mountArg)
	}

	if len(opts.ExtraArgs) > 0 {
		args = append(args, opts.ExtraArgs...)
	}

	args = append(args, opts.Image)
	args = append(args, opts.Command...)
	return args, nil
}

func validateMount(m Mount) error {
	if m.Source == "" || m.Destination == "" {
		return fmt.Errorf("invalid mount: missing source or destination")
	}
	if !filepath.IsAbs(m.Destination) {
		return fmt.Errorf("invalid mount destination %q: must be absolute", m.Destination)
	}
	return nil
}

// execLookPath is declared for test substitution.
var execLookPath = func(file string) (string, error) {
	return exec.LookPath(file)
}

var runtimeCommand = func(ctx context.Context, runtime Runtime, args ...string) ([]byte, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	cmd := exec.CommandContext(ctx, string(runtime), args...)
	return cmd.CombinedOutput()
}

// BuildEnv converts environment map to sorted list, preserving secrets for container env.
func BuildEnv(env map[string]string) []string {
	if len(env) == 0 {
		return nil
	}
	out := make([]string, 0, len(env))
	for k, v := range env {
		out = append(out, fmt.Sprintf("%s=%s", k, v))
	}
	// stable ordering for tests
	sort.Strings(out)
	return out
}

func StopContainer(ctx context.Context, runtime Runtime, name string, timeout time.Duration) error {
	if runtime == "" || name == "" {
		return nil
	}
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	seconds := int(timeout.Seconds())
	if seconds <= 0 {
		seconds = 1
	}
	runCtx, cancel := context.WithTimeout(backgroundContext(ctx), timeout+5*time.Second)
	defer cancel()
	args := []string{"stop", "--time", strconv.Itoa(seconds), name}
	output, err := runtimeCommand(runCtx, runtime, args...)
	if err != nil {
		if isContainerNotFound(output) {
			return nil
		}
		return fmt.Errorf("stop container %s: %w", name, err)
	}
	return nil
}

func KillContainer(ctx context.Context, runtime Runtime, name string) error {
	if runtime == "" || name == "" {
		return nil
	}
	runCtx, cancel := context.WithTimeout(backgroundContext(ctx), 10*time.Second)
	defer cancel()
	args := []string{"kill", name}
	output, err := runtimeCommand(runCtx, runtime, args...)
	if err != nil {
		if isContainerNotFound(output) {
			return nil
		}
		return fmt.Errorf("kill container %s: %w", name, err)
	}
	return nil
}

func RemoveContainer(ctx context.Context, runtime Runtime, name string) error {
	if runtime == "" || name == "" {
		return nil
	}
	runCtx, cancel := context.WithTimeout(backgroundContext(ctx), 10*time.Second)
	defer cancel()
	args := []string{"rm", "--force"}
	if runtime == RuntimePodman {
		args = append(args, "--ignore")
	}
	args = append(args, name)
	output, err := runtimeCommand(runCtx, runtime, args...)
	if err != nil {
		if isContainerNotFound(output) {
			return nil
		}
		return fmt.Errorf("remove container %s: %w", name, err)
	}
	return nil
}

func backgroundContext(ctx context.Context) context.Context {
	if ctx == nil {
		return context.Background()
	}
	return ctx
}

func isContainerNotFound(output []byte) bool {
	msg := strings.ToLower(strings.TrimSpace(string(output)))
	if msg == "" {
		return false
	}
	return strings.Contains(msg, "no such container") || strings.Contains(msg, "not found")
}
