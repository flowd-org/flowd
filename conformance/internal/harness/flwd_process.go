package harness

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"strings"
	"time"
)

// FlwdProcess represents a managed flwd server process.
type FlwdProcess struct {
	Cmd     *exec.Cmd
	Addr    string
	BaseURL string
	Stdout  *bytes.Buffer
	Stderr  *bytes.Buffer
}

// PickBindAddr tries ports in the range 127.0.0.1:18080..18089 and returns the first free one.
func PickBindAddr() (string, error) {
	for port := 18080; port <= 18089; port++ {
		addr := fmt.Sprintf("127.0.0.1:%d", port)
		ln, err := net.Listen("tcp", addr)
		if err == nil {
			ln.Close()
			return addr, nil
		}
	}
	return "", fmt.Errorf("no free port in range 18080-18089")
}

// StartFlwd starts the flwd server process in :serve mode.
// It validates the binary exists and is executable, picks a bind address,
// and starts the process with stdout/stderr captured.
func StartFlwd(ctx context.Context, cfg Config, runRoot string) (*FlwdProcess, int, error) {
	// Validate binary exists and is executable
	if _, err := os.Stat(cfg.FlwdBinary); err != nil {
		if os.IsNotExist(err) {
			return nil, ExitInfra, fmt.Errorf("flwd binary not found: %s", cfg.FlwdBinary)
		}
		return nil, ExitInfra, fmt.Errorf("cannot stat flwd binary: %w", err)
	}

	info, err := os.Stat(cfg.FlwdBinary)
	if err != nil {
		return nil, ExitInfra, fmt.Errorf("cannot stat flwd binary: %w", err)
	}
	if info.IsDir() {
		return nil, ExitInfra, fmt.Errorf("flwd binary is a directory: %s", cfg.FlwdBinary)
	}

	// Select bind address: explicit from config or auto via PickBindAddr
	bindAddr := cfg.Bind
	if bindAddr == "" {
		addr, err := PickBindAddr()
		if err != nil {
			return nil, ExitInfra, err
		}
		bindAddr = addr
	}

	// Build command arguments
	args := []string{":serve", "--bind", bindAddr}
	if cfg.FlwdProfile != "" {
		args = append(args, "--profile", cfg.FlwdProfile)
	}

	cmd := exec.CommandContext(ctx, cfg.FlwdBinary, args...)
	cmd.Dir = runRoot

	// Avoid leaking token via environment
	env := make([]string, 0, len(os.Environ()))
	for _, e := range os.Environ() {
		if !strings.HasPrefix(e, "FLWD_TOKEN=") {
			env = append(env, e)
		}
	}
	cmd.Env = env

	// Capture stdout/stderr
	stdoutBuf := &bytes.Buffer{}
	stderrBuf := &bytes.Buffer{}
	cmd.Stdout = stdoutBuf
	cmd.Stderr = stderrBuf

	if err := cmd.Start(); err != nil {
		return nil, ExitInfra, fmt.Errorf("failed to start flwd: %w", err)
	}

	fp := &FlwdProcess{
		Cmd:     cmd,
		Addr:    bindAddr,
		BaseURL: fmt.Sprintf("http://%s", bindAddr),
		Stdout:  stdoutBuf,
		Stderr:  stderrBuf,
	}

	return fp, ExitOK, nil
}

// Stop terminates the flwd process gracefully with SIGINT, then SIGKILL if needed.
func (p *FlwdProcess) Stop(ctx context.Context) error {
	if p.Cmd == nil || p.Cmd.Process == nil {
		return nil
	}

	// Try graceful shutdown first
	_ = p.Cmd.Process.Signal(os.Interrupt)

	done := make(chan error, 1)
	go func() {
		done <- p.Cmd.Wait()
	}()

	// Wait for graceful exit with timeout
	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		// Timeout, force kill
		_ = p.Cmd.Process.Kill()
		return <-done
	case <-time.After(5 * time.Second):
		// Force kill
		_ = p.Cmd.Process.Kill()
		return <-done
	}
}

// Cleanup is an alias for Stop for convenience.
func (p *FlwdProcess) Cleanup(ctx context.Context) error {
	return p.Stop(ctx)
}
