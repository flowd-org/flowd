// SPDX-License-Identifier: AGPL-3.0-or-later
package executor

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/flowd-org/flowd/internal/configloader"
	"github.com/flowd-org/flowd/internal/events"
	"github.com/flowd-org/flowd/internal/executor/container"
	"github.com/flowd-org/flowd/internal/paths"
	"github.com/flowd-org/flowd/internal/server/metrics"
	"github.com/flowd-org/flowd/internal/types"
)

// ExecutorConfig holds runtime execution options.
type ExecutorConfig struct {
	Flags     map[string]interface{} // parsed CLI args
	DryRun    bool
	Verbosity int
	Strict    bool
	// Engine bindings
	ArgEnv                  map[string]string // ARG_<UPPER>=value (scalars only)
	ArgsJSON                string            // FLWD_ARGS_JSON content
	ArgValues               map[string]interface{}
	RunID                   string
	JobID                   string
	Emitter                 events.Sink
	RunDir                  string
	StdoutWriter            io.Writer
	StderrWriter            io.Writer
	LineRedactor            func(string) string
	ContainerRuntime        container.Runtime
	EnvInherit              bool
	ContainerNetwork        string
	ContainerRootfsWritable bool
	ContainerCapabilities   []string
	SecretsDir              string
}

// ScriptResult holds per-script run outcome.
type ScriptResult struct {
	Name     string
	ExitCode int
	Duration time.Duration
	Err      error
}

func sanitizeName(id string) string {
	if id == "" {
		return "step"
	}
	lower := strings.ToLower(id)
	var b strings.Builder
	for _, r := range lower {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '-' || r == '_':
			b.WriteRune('-')
		default:
			b.WriteRune('-')
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		out = "step"
	}
	if len(out) > 63 {
		out = out[:63]
	}
	return out
}

func isDAGConfig(cfg *types.Config) bool {
	if cfg == nil {
		return false
	}
	if len(cfg.Steps) == 0 {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(cfg.Composition), "steps")
}

func RunScripts(ctx context.Context, dir string, ecfg ExecutorConfig) ([]ScriptResult, error) {
	cfg, err := configloader.LoadConfig(dir)
	if err != nil {
		return nil, fmt.Errorf("loading config: %w", err)
	}
	if isDAGConfig(cfg) {
		return runDAGSteps(ctx, dir, cfg, ecfg)
	}
	if strings.HasPrefix(strings.ToLower(cfg.Interpreter), "container:") {
		if ecfg.ContainerRuntime == "" {
			runtime, detectErr := container.DetectRuntime(nil)
			if detectErr != nil {
				return nil, fmt.Errorf("container runtime unavailable: %w", detectErr)
			}
			ecfg.ContainerRuntime = runtime
		}
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("reading dir: %w", err)
	}

	var scripts []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if strings.HasPrefix(name, "000_") || strings.HasPrefix(name, "100_") || strings.HasPrefix(name, "999_") {
			scripts = append(scripts, name)
		}
	}
	sort.Strings(scripts)

	var results []ScriptResult
	retryPolicy := strings.ToLower(cfg.ErrorHandling.Policy)
	maxRetries := cfg.ErrorHandling.Retries
	retryBackoff := cfg.ErrorHandling.RetryBackoff

	for _, script := range scripts {
		scriptPath := filepath.Join(dir, script)
		interpreter := cfg.Interpreter
		if interpreter == "" {
			return nil, fmt.Errorf("no interpreter defined in config.yaml for script %s", script)
		}

		stepID := script
		if ecfg.Emitter != nil {
			ecfg.Emitter.EmitStepStart(ecfg.RunID, stepID)
		}

		var flagArgs []string
		for name, val := range ecfg.Flags {
			switch v := val.(type) {
			case bool:
				if v {
					flagArgs = append(flagArgs, "--"+name)
				}
			case string:
				flagArgs = append(flagArgs, fmt.Sprintf("--%s=%s", name, v))
			case int:
				flagArgs = append(flagArgs, fmt.Sprintf("--%s=%d", name, v))
			}
		}
		if strings.HasPrefix(interpreter, "container:") {
			exitCode, dur, err := runContainerStep(ctx, cfg, ecfg, scriptPath, interpreter, flagArgs, ecfg.Emitter, stepID)
			if ecfg.Emitter != nil {
				ecfg.Emitter.EmitStepFinish(ecfg.RunID, stepID, exitCode, err)
			}
			results = append(results, ScriptResult{Name: script, ExitCode: exitCode, Duration: dur, Err: err})
			if err != nil {
				return results, err
			}
			continue
		}

		if ecfg.Verbosity >= 1 {
			// Avoid printing potentially sensitive flag values
			fmt.Printf("[RUN] %s %s\n", interpreter, scriptPath)
		}

		if ecfg.DryRun {
			continue
		}

		result := executeProcessStep(ctx, cfg, ecfg, scriptPath, script, interpreter, flagArgs, stepID, retryPolicy, maxRetries, retryBackoff)
		if ecfg.Emitter != nil {
			ecfg.Emitter.EmitStepFinish(ecfg.RunID, stepID, result.ExitCode, result.Err)
		}
		results = append(results, result)
		if result.Err != nil && ecfg.Strict {
			return results, fmt.Errorf("script %s failed: %w", script, result.Err)
		}
	}

	return results, nil
}

func runDAGSteps(ctx context.Context, dir string, cfg *types.Config, ecfg ExecutorConfig) ([]ScriptResult, error) {
	executor := strings.ToLower(strings.TrimSpace(cfg.Executor))
	if executor == "" {
		return nil, fmt.Errorf("dag executor not configured")
	}
	retryPolicy := strings.ToLower(cfg.ErrorHandling.Policy)
	maxRetries := cfg.ErrorHandling.Retries
	retryBackoff := cfg.ErrorHandling.RetryBackoff

	results := make([]ScriptResult, 0, len(cfg.Steps))
	for idx, step := range cfg.Steps {
		stepID := strings.TrimSpace(step.ID)
		if stepID == "" {
			stepID = fmt.Sprintf("step-%03d", idx)
		}
		scriptPath := strings.TrimSpace(step.Script)
		if scriptPath == "" {
			return results, fmt.Errorf("step %s missing script path", stepID)
		}
		if !filepath.IsAbs(scriptPath) {
			scriptPath = filepath.Join(dir, scriptPath)
		}
		if ecfg.Emitter != nil {
			ecfg.Emitter.EmitStepStart(ecfg.RunID, stepID)
		}

		flagArgs := make([]string, 0, len(ecfg.Flags))
		for name, val := range ecfg.Flags {
			switch v := val.(type) {
			case bool:
				if v {
					flagArgs = append(flagArgs, "--"+name)
				}
			case string:
				flagArgs = append(flagArgs, fmt.Sprintf("--%s=%s", name, v))
			case int:
				flagArgs = append(flagArgs, fmt.Sprintf("--%s=%d", name, v))
			}
		}

		var (
			result ScriptResult
			err    error
		)

		switch executor {
		case "container":
			merged := mergeContainerConfigs(cfg.Container, step.Container)
			image := strings.TrimSpace(merged.Image)
			if image == "" {
				err = fmt.Errorf("step %s missing container image", stepID)
				result = ScriptResult{Name: stepID, ExitCode: -1, Err: err}
			} else {
				interpreter := "container:" + image
				stepCfg := &types.Config{
					Container:      merged,
					Env:            cfg.Env,
					EnvInheritance: cfg.EnvInheritance,
				}
				exitCode, dur, runErr := runContainerStep(ctx, stepCfg, ecfg, scriptPath, interpreter, flagArgs, ecfg.Emitter, stepID)
				result = ScriptResult{Name: stepID, ExitCode: exitCode, Duration: dur, Err: runErr}
				err = runErr
			}
		case "proc":
			interpreter := cfg.Interpreter
			if interpreter == "" {
				err = fmt.Errorf("no interpreter defined for DAG job")
				result = ScriptResult{Name: stepID, ExitCode: -1, Err: err}
			} else {
				result = executeProcessStep(ctx, cfg, ecfg, scriptPath, stepID, interpreter, flagArgs, stepID, retryPolicy, maxRetries, retryBackoff)
				err = result.Err
			}
		default:
			err = fmt.Errorf("unsupported executor %s", executor)
			result = ScriptResult{Name: stepID, ExitCode: -1, Err: err}
		}

		if ecfg.Emitter != nil {
			ecfg.Emitter.EmitStepFinish(ecfg.RunID, stepID, result.ExitCode, err)
		}
		results = append(results, result)
		if err != nil {
			if ecfg.Strict {
				return results, fmt.Errorf("step %s failed: %w", stepID, err)
			}
		}
	}
	return results, nil
}

func executeProcessStep(ctx context.Context, cfg *types.Config, ecfg ExecutorConfig, scriptPath, scriptLabel, interpreter string, flagArgs []string, stepID string, retryPolicy string, maxRetries, retryBackoff int) ScriptResult {
	result := ScriptResult{Name: scriptLabel}
	for attempt := 0; attempt <= maxRetries; attempt++ {
		start := time.Now()
		profilePath, cleanup, err := GenerateRunnerProfile(filepath.Dir(scriptPath), interpreter, ecfg.Verbosity, cfg.ArgSpec, ecfg.ArgValues)
		if err != nil {
			result.ExitCode = -1
			result.Err = fmt.Errorf("profile generation failed for %s: %w", scriptLabel, err)
			return result
		}
		defer cleanup()

		interpCmd, interpArgs, splitErr := splitInterpreter(interpreter)
		if splitErr != nil {
			result.ExitCode = -1
			result.Err = splitErr
			return result
		}

		var cmd *exec.Cmd
		switch {
		case strings.Contains(interpreter, "bash"):
			cmdArgs := append([]string{}, interpArgs...)
			cmdArgs = append(cmdArgs, append([]string{scriptPath}, flagArgs...)...)
			cmd = exec.CommandContext(ctx, interpCmd, cmdArgs...)
		case strings.Contains(interpreter, "pwsh"), strings.Contains(interpreter, "powershell"):
			newArgs := append([]string{}, interpArgs...)
			newArgs = append(newArgs,
				"-NoProfile", "-ExecutionPolicy", "Bypass",
				"-File", profilePath,
				"-TargetScript", scriptPath,
			)
			newArgs = append(newArgs, flagArgs...)
			cmd = exec.CommandContext(ctx, interpCmd, newArgs...)
		default:
			cmdArgs := append([]string{}, interpArgs...)
			cmdArgs = append(cmdArgs, append([]string{scriptPath}, flagArgs...)...)
			cmd = exec.CommandContext(ctx, interpCmd, cmdArgs...)
		}

		stdoutSink := ecfg.StdoutWriter
		if stdoutSink == nil {
			stdoutSink = os.Stdout
		}
		stderrSink := ecfg.StderrWriter
		if stderrSink == nil {
			stderrSink = os.Stderr
		}
		stdoutWriter := events.NewStepWriter(ecfg.Emitter, ecfg.RunID, stepID, "stdout", stdoutSink, ecfg.LineRedactor)
		stderrWriter := events.NewStepWriter(ecfg.Emitter, ecfg.RunID, stepID, "stderr", stderrSink, ecfg.LineRedactor)
		cmd.Stdout = stdoutWriter
		cmd.Stderr = stderrWriter

		inherit := ecfg.EnvInherit
		if !inherit && cfg != nil && cfg.EnvInheritance {
			inherit = true
		}
		env := buildSecureEnv(cfg, ecfg.ArgEnv, ecfg.ArgsJSON, inherit)
		runDir := ecfg.RunDir
		if runDir == "" {
			runDir = filepath.Dir(scriptPath)
		}
		dataDir := paths.DataDir()
		env = upsertEnv(env, "DATA_DIR", dataDir)
		env = upsertEnv(env, "FLOWD_DATA_DIR", dataDir)
		env = upsertEnv(env, "FLOWD_RUN_DIR", runDir)
		env = upsertEnv(env, "RUN_DIR", runDir)
		env = upsertEnv(env, "FLWD_RUN_DIR", runDir)
		if strings.Contains(interpreter, "bash") {
			cmd.Env = append(env, fmt.Sprintf("BASH_ENV=%s", profilePath))
		} else {
			cmd.Env = env
		}

		restoreUmask := applySecureUmask()
		err = cmd.Run()
		if restoreUmask != nil {
			restoreUmask()
		}
		stdoutWriter.Flush()
		stderrWriter.Flush()
		duration := time.Since(start)
		result.Duration = duration

		if err == nil {
			result.ExitCode = 0
			result.Err = nil
			if ecfg.Verbosity >= 1 {
				msg := "[OK]  %s success in %s\n"
				if attempt > 0 {
					msg = "[OK]  %s recovered after %d attempt(s) in %s\n"
					fmt.Printf(msg, scriptLabel, attempt+1, duration)
				} else {
					fmt.Printf(msg, scriptLabel, duration)
				}
			}
			return result
		}

		exitCode := -1
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		}
		result.ExitCode = exitCode
		result.Err = err

		if ecfg.Verbosity >= 1 {
			fmt.Printf("[!]  %s failed (attempt %d/%d): %v\n", scriptLabel, attempt+1, maxRetries+1, err)
		}

		if attempt < maxRetries && retryPolicy == "retry" {
			if ecfg.Verbosity >= 1 {
				fmt.Printf("     Retrying in %ds...\n", retryBackoff)
			}
			time.Sleep(time.Duration(retryBackoff) * time.Second)
			continue
		}

		if ecfg.Verbosity >= 1 {
			fmt.Printf("[ERR] %s failed after %d attempt(s), exit %d in %s\n", scriptLabel, attempt+1, exitCode, duration)
		}
		return result
	}
	return result
}

func mergeContainerConfigs(jobCfg, stepCfg *types.ContainerConfig) *types.ContainerConfig {
	base := cloneContainer(jobCfg)
	if base == nil {
		base = &types.ContainerConfig{}
	}
	if stepCfg == nil {
		return base
	}
	if strings.TrimSpace(stepCfg.Image) != "" {
		base.Image = strings.TrimSpace(stepCfg.Image)
	}
	if stepCfg.Resources != nil {
		base.Resources = &types.ContainerResources{
			CPU:    strings.TrimSpace(stepCfg.Resources.CPU),
			Memory: strings.TrimSpace(stepCfg.Resources.Memory),
		}
	}
	if strings.TrimSpace(stepCfg.Network) != "" {
		base.Network = strings.TrimSpace(stepCfg.Network)
	}
	base.RootfsWritable = stepCfg.RootfsWritable
	if len(stepCfg.Capabilities) > 0 {
		base.Capabilities = append([]string{}, stepCfg.Capabilities...)
	}
	if len(stepCfg.ExtraArgs) > 0 {
		base.ExtraArgs = append([]string{}, stepCfg.ExtraArgs...)
	}
	if len(stepCfg.Entrypoint) > 0 {
		base.Entrypoint = append([]string{}, stepCfg.Entrypoint...)
	}
	return base
}

func cloneContainer(cfg *types.ContainerConfig) *types.ContainerConfig {
	if cfg == nil {
		return nil
	}
	clone := &types.ContainerConfig{
		Image:          strings.TrimSpace(cfg.Image),
		Network:        strings.TrimSpace(cfg.Network),
		RootfsWritable: cfg.RootfsWritable,
		Capabilities:   append([]string{}, cfg.Capabilities...),
		ExtraArgs:      append([]string{}, cfg.ExtraArgs...),
		Entrypoint:     append([]string{}, cfg.Entrypoint...),
	}
	if cfg.Resources != nil {
		clone.Resources = &types.ContainerResources{
			CPU:    strings.TrimSpace(cfg.Resources.CPU),
			Memory: strings.TrimSpace(cfg.Resources.Memory),
		}
	}
	return clone
}

func quoteArgs(args []string) string {
	quoted := make([]string, len(args))
	for i, arg := range args {
		quoted[i] = fmt.Sprintf("'%s'", strings.ReplaceAll(arg, "'", "''"))
	}
	return strings.Join(quoted, ", ")
}

func upsertEnv(env []string, key, value string) []string {
	prefix := key + "="
	for i, kv := range env {
		if strings.HasPrefix(kv, prefix) {
			env[i] = prefix + value
			return env
		}
	}
	return append(env, prefix+value)
}

func buildSecureEnv(cfg *types.Config, argEnv map[string]string, argsJSON string, inherit bool) []string {
	type entry struct {
		key string
		val string
	}
	ordered := make([]entry, 0)
	envSet := make(map[string]string)
	set := func(k, v string) {
		if _, exists := envSet[k]; !exists {
			ordered = append(ordered, entry{key: k, val: v})
		}
		envSet[k] = v
	}

	if cfg != nil && cfg.Env != nil {
		for k, v := range cfg.Env {
			set(k, v)
		}
	}
	if _, ok := envSet["PATH"]; !ok {
		if path := os.Getenv("PATH"); path != "" {
			set("PATH", path)
		}
	}
	for k, v := range argEnv {
		set(k, v)
	}
	if argsJSON != "" {
		set("FLWD_ARGS_JSON", argsJSON)
	}
	if inherit {
		for _, kv := range os.Environ() {
			parts := strings.SplitN(kv, "=", 2)
			if len(parts) != 2 {
				continue
			}
			if _, exists := envSet[parts[0]]; exists {
				continue
			}
			set(parts[0], parts[1])
		}
	}
	env := make([]string, 0, len(ordered))
	for _, e := range ordered {
		env = append(env, fmt.Sprintf("%s=%s", e.key, envSet[e.key]))
	}
	return env
}

func splitInterpreter(interpreter string) (string, []string, error) {
	fields := strings.Fields(interpreter)
	if len(fields) == 0 {
		return "", nil, fmt.Errorf("invalid interpreter: %q", interpreter)
	}
	return fields[0], fields[1:], nil
}
func runContainerStep(ctx context.Context, cfg *types.Config, ecfg ExecutorConfig, scriptPath, interpreter string, flagArgs []string, sink events.Sink, stepID string) (int, time.Duration, error) {
	parts := strings.SplitN(interpreter, ":", 2)
	if len(parts) != 2 {
		return -1, 0, fmt.Errorf("invalid container interpreter: %s", interpreter)
	}
	image := parts[1]
	runtime := ecfg.ContainerRuntime
	if runtime == "" {
		var err error
		runtime, err = container.DetectRuntime(nil)
		if err != nil {
			return -1, 0, err
		}
	}
	containerName := ecfg.RunID
	if stepID != "" {
		containerName = fmt.Sprintf("%s-%s", ecfg.RunID, sanitizeName(stepID))
	}
	if containerName == "" {
		containerName = fmt.Sprintf("flwd-%d", time.Now().UnixNano())
	}
	if err := container.RemoveContainer(context.Background(), runtime, containerName); err != nil {
		return -1, 0, fmt.Errorf("prepare container %s: %w", containerName, err)
	}

	inherit := ecfg.EnvInherit
	if !inherit && cfg != nil && cfg.EnvInheritance {
		inherit = true
	}
	envList := buildSecureEnv(cfg, ecfg.ArgEnv, ecfg.ArgsJSON, inherit)
	envMap := make(map[string]string, len(envList))
	for _, kv := range envList {
		parts := strings.SplitN(kv, "=", 2)
		if len(parts) != 2 {
			continue
		}
		envMap[parts[0]] = parts[1]
	}
	scriptDir := filepath.Dir(scriptPath)
	absScriptDir, err := filepath.Abs(scriptDir)
	if err != nil {
		return -1, 0, err
	}
	// Ensure the command we exec inside the container uses an absolute path that
	// matches the mount destination, so the script is resolvable regardless of
	// the caller's working directory.
	scriptAbs := filepath.Join(absScriptDir, filepath.Base(scriptPath))
	runDir := ecfg.RunDir
	if runDir == "" {
		runDir = absScriptDir
	}
	dataDir := paths.DataDir()
	updates := map[string]string{
		"DATA_DIR":       dataDir,
		"FLOWD_DATA_DIR": dataDir,
		"FLOWD_RUN_DIR":  runDir,
		"RUN_DIR":        runDir,
		"FLWD_RUN_DIR":   runDir,
	}
	for k, v := range updates {
		envList = upsertEnv(envList, k, v)
		envMap[k] = v
	}

	mounts := []container.Mount{{Source: absScriptDir, Destination: absScriptDir, ReadOnly: true}}
	if runDir != absScriptDir {
		mounts = append(mounts, container.Mount{Source: runDir, Destination: runDir, ReadOnly: false})
	} else {
		mounts[0].ReadOnly = false
	}
	if ecfg.SecretsDir != "" {
		mounts = append(mounts, container.Mount{Source: ecfg.SecretsDir, Destination: "/run/secrets", ReadOnly: true})
	}

	opts := container.RunOptions{
		Runtime:        runtime,
		Image:          image,
		Command:        append([]string{scriptAbs}, flagArgs...),
		Env:            envMap,
		WorkDir:        runDir,
		Mounts:         mounts,
		Remove:         true,
		Name:           containerName,
		NetworkMode:    strings.TrimSpace(ecfg.ContainerNetwork),
		WritableRootfs: ecfg.ContainerRootfsWritable,
		Capabilities:   append([]string{}, ecfg.ContainerCapabilities...),
	}
	if cfg != nil && cfg.Container != nil {
		if opts.NetworkMode == "" {
			opts.NetworkMode = strings.TrimSpace(cfg.Container.Network)
		}
		if !ecfg.ContainerRootfsWritable {
			opts.WritableRootfs = cfg.Container.RootfsWritable
		}
		if len(opts.Capabilities) == 0 && len(cfg.Container.Capabilities) > 0 {
			opts.Capabilities = append(opts.Capabilities, cfg.Container.Capabilities...)
		}
		if len(cfg.Container.ExtraArgs) > 0 {
			opts.ExtraArgs = append(opts.ExtraArgs, cfg.Container.ExtraArgs...)
		}
	}
	args, err := container.BuildArgs(opts)
	if err != nil {
		return -1, 0, err
	}
	stdoutWriter := events.NewStepWriter(sink, ecfg.RunID, stepID, "stdout", ecfg.StdoutWriter, ecfg.LineRedactor)
	stderrWriter := events.NewStepWriter(sink, ecfg.RunID, stepID, "stderr", ecfg.StderrWriter, ecfg.LineRedactor)
	cmd := exec.CommandContext(ctx, args[0], args[1:]...)
	cmd.Stdout = stdoutWriter
	cmd.Stderr = stderrWriter
	cmd.Env = envList
	runStart := time.Now()
	err = cmd.Run()
	stdoutWriter.Flush()
	stderrWriter.Flush()
	dur := time.Since(runStart)
	exitCode := 0
	if ctx != nil && errors.Is(ctx.Err(), context.Canceled) {
		cancelCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		_ = container.StopContainer(cancelCtx, runtime, containerName, 10*time.Second)
		_ = container.KillContainer(cancelCtx, runtime, containerName)
		_ = container.RemoveContainer(cancelCtx, runtime, containerName)
		if err == nil {
			err = context.Canceled
		}
	}
	if errors.Is(err, context.Canceled) {
		cancelCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		_ = container.StopContainer(cancelCtx, runtime, containerName, 10*time.Second)
		_ = container.KillContainer(cancelCtx, runtime, containerName)
		_ = container.RemoveContainer(cancelCtx, runtime, containerName)
	}
	metrics.Default.RecordContainerRun(dur)
	metrics.Default.RecordContainerPull(dur)
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			exitCode = exitErr.ExitCode()
		} else {
			exitCode = -1
		}
	} else {
		exitCode = 0
	}
	return exitCode, dur, err
}
