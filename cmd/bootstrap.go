// SPDX-License-Identifier: AGPL-3.0-or-later
package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/flowd-org/flowd/internal/argsloader"
	"github.com/flowd-org/flowd/internal/configloader"
	"github.com/flowd-org/flowd/internal/engine"
	"github.com/flowd-org/flowd/internal/events"
	"github.com/flowd-org/flowd/internal/executor"
	"github.com/flowd-org/flowd/internal/indexer"
	"github.com/flowd-org/flowd/internal/paths"
	"github.com/flowd-org/flowd/internal/types"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"gopkg.in/yaml.v3"
)

func RegisterScriptCommands(root *cobra.Command, scriptsDir string) error {
	leafScripts := make(map[string]string)

	cmds, err := os.ReadDir(scriptsDir)
	if err != nil {
		return fmt.Errorf("scanning %s: %w", scriptsDir, err)
	}

	for _, cmdEntry := range cmds {
		if !cmdEntry.IsDir() {
			continue
		}
		cmdName := cmdEntry.Name()

		// Skip config.d and hidden folders
		if cmdName == "config.d" || strings.HasPrefix(cmdName, ".") {
			continue
		}

		cmdPath := filepath.Join(scriptsDir, cmdName)

		// Check for standalone command config.d/config.yaml
		configPath := filepath.Join(cmdPath, "config.d", "config.yaml")
		if _, err := os.Stat(configPath); err == nil {
			cmd := &cobra.Command{
				Use:   cmdName,
				Short: fmt.Sprintf("Run %s scripts", cmdName),
				RunE:  makeRunE(cmdPath),
			}
			if cmd.Annotations == nil {
				cmd.Annotations = map[string]string{}
			}
			cmd.Annotations["scriptDir"] = cmdPath

			if err := argsloader.AttachFlags(cmd, cmdPath); err != nil {
				return fmt.Errorf("attach flags %s: %w", cmdPath, err)
			}
			addCommonFlags(cmd)
			cmd.Flags().Bool("json", false, "Stream events as NDJSON")

			root.AddCommand(cmd)
			leafScripts[cmdName] = cmdPath
			continue // prevent also being treated as a parent with subcommands
		}

		// Treat it as a parent with subcommands
		parent := &cobra.Command{
			Use:   cmdName,
			Short: fmt.Sprintf("Run %s scripts", cmdName),
		}
		if parent.Annotations == nil {
			parent.Annotations = map[string]string{}
		}
		parent.Annotations["scriptDir"] = cmdPath

		subs, err := os.ReadDir(cmdPath)
		if err != nil {
			return fmt.Errorf("scanning %s: %w", cmdPath, err)
		}
		for _, subEntry := range subs {
			if !subEntry.IsDir() {
				continue
			}
			subName := subEntry.Name()
			subPath := filepath.Join(cmdPath, subName)

			if _, err := os.Stat(filepath.Join(subPath, "config.d", "config.yaml")); err != nil {
				continue
			}

			scmd := &cobra.Command{
				Use:   subName,
				Short: fmt.Sprintf("Run %s %s scripts", cmdName, subName),
				RunE:  makeRunE(subPath),
			}
			if scmd.Annotations == nil {
				scmd.Annotations = map[string]string{}
			}
			scmd.Annotations["scriptDir"] = subPath
			//debug
			//fmt.Fprintf(os.Stderr, "[DEBUG] Attaching flags for %s\n", subPath)
			if err := argsloader.AttachFlags(scmd, subPath); err != nil {
				return fmt.Errorf("attach flags %s: %w", subPath, err)
			}
			addCommonFlags(scmd)
			scmd.Flags().Bool("json", false, "Stream events as NDJSON")

			parent.AddCommand(scmd)
			pathKey := fmt.Sprintf("%s/%s", cmdName, subName)
			leafScripts[pathKey] = subPath
		}

		// Only add parent if it has valid subcommands
		if len(parent.Commands()) > 0 {
			root.AddCommand(parent)
		}
	}

	res, err := indexer.Discover(scriptsDir)
	if err != nil {
		return err
	}
	for _, alias := range res.Aliases {
		targetPath := alias.TargetPath
		scriptDir, ok := leafScripts[targetPath]
		if !ok {
			continue
		}
		aliasCmd := &cobra.Command{
			Use:   alias.Name,
			Short: fmt.Sprintf("[alias] %s", strings.ReplaceAll(targetPath, "/", " ")),
			RunE:  makeRunE(scriptDir),
		}
		if aliasCmd.Annotations == nil {
			aliasCmd.Annotations = map[string]string{}
		}
		aliasCmd.Annotations["scriptDir"] = scriptDir
		aliasCmd.Annotations["isAlias"] = "true"
		aliasCmd.Annotations["aliasTarget"] = targetPath
		if alias.Description != "" {
			aliasCmd.Annotations["aliasDescription"] = alias.Description
		}
		if err := argsloader.AttachFlags(aliasCmd, scriptDir); err != nil {
			return fmt.Errorf("attach flags %s: %w", scriptDir, err)
		}
		addCommonFlags(aliasCmd)
		aliasCmd.Flags().Bool("json", false, "Stream events as NDJSON")
		root.AddCommand(aliasCmd)
	}

	return nil
}

func makeRunE(scriptDir string) func(cmd *cobra.Command, args []string) error {
	return func(cmd *cobra.Command, args []string) error {
		cfg, err := configloader.LoadConfig(scriptDir)
		if err != nil {
			return err
		}

		if cmd.Flags().Changed("on-error") {
			pol, _ := cmd.Flags().GetString("on-error")
			cfg.ErrorHandling.Policy = pol
		}

		// Validate CLI flags against ArgSpec and build bindings
		var bind *engine.Binding
		if cfg.ArgSpec != nil {
			b, vErr := engine.ValidateAndBind(cmd.Flags(), *cfg.ArgSpec)
			if vErr != nil {
				// E_ARGS: return with field-level message
				return fmt.Errorf("E_ARGS: %v", vErr)
			}
			bind = b
		}

		flagsMap := make(map[string]interface{})
		cmd.Flags().VisitAll(func(f *pflag.Flag) {
			switch f.Name {
			case "dry-run", "verbose", "quiet", "strict", "on-error", "report", "report-file", "json":
				return
			}
			switch f.Value.Type() {
			case "bool":
				v, _ := cmd.Flags().GetBool(f.Name)
				flagsMap[f.Name] = v
			case "int":
				v, _ := cmd.Flags().GetInt(f.Name)
				flagsMap[f.Name] = v
			case "stringArray":
				v, _ := cmd.Flags().GetStringArray(f.Name)
				flagsMap[f.Name] = v
			default:
				flagsMap[f.Name] = f.Value.String()
			}
		})

		dryRun, _ := cmd.Flags().GetBool("dry-run")
		verbosity, _ := cmd.Flags().GetCount("verbose")
		strict, _ := cmd.Flags().GetBool("strict")
		quiet, _ := cmd.Flags().GetBool("quiet")
		reportFormat, _ := cmd.Flags().GetString("report")
		reportFile, _ := cmd.Flags().GetString("report-file")
		jsonEvents, _ := cmd.Flags().GetBool("json")

		runID := events.GenerateRunID()
		jobID := cmd.CommandPath()

		plan := engine.BuildPlan(jobID, cfg, cfg.ArgSpec, bind)
		// Resolve profile precedence for CLI run: flag > env > default
		prof, _ := cmd.Flags().GetString("profile")
		if prof == "" {
			if env := os.Getenv("FLWD_PROFILE"); env != "" {
				prof = env
			}
		}
		if prof == "" {
			prof = "secure"
		}
		plan.SecurityProfile = strings.ToLower(prof)
		runDir := paths.RunDir(runID)
		if abs, err := filepath.Abs(runDir); err == nil {
			runDir = abs
		}
		if err := writePlanArtifact(plan, runDir); err != nil {
			return err
		}

		stdoutFile, err := os.OpenFile(filepath.Join(runDir, "stdout"), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
		if err != nil {
			return fmt.Errorf("open stdout file: %w", err)
		}
		defer stdoutFile.Close()
		stderrFile, err := os.OpenFile(filepath.Join(runDir, "stderr"), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
		if err != nil {
			return fmt.Errorf("open stderr file: %w", err)
		}
		defer stderrFile.Close()

		var consoleEmitter events.Sink
		if verbosity > 0 {
			consoleEmitter = events.NewEmitter(os.Stdout, jsonEvents)
		}
		emitter := events.NewCompositeSink(consoleEmitter)
		if emitter != nil {
			emitter.EmitRunStart(runID, jobID)
		}

		stdoutWriter := io.MultiWriter(stdoutFile, os.Stdout)
		stderrWriter := io.MultiWriter(stderrFile, os.Stderr)
		if quiet {
			stdoutWriter = io.Writer(stdoutFile)
			stderrWriter = io.Writer(stderrFile)
		}

		ecfg := executor.ExecutorConfig{
			Flags:        flagsMap,
			DryRun:       dryRun,
			Verbosity:    verbosity,
			Strict:       strict,
			RunID:        runID,
			JobID:        jobID,
			Emitter:      emitter,
			RunDir:       runDir,
			StdoutWriter: stdoutWriter,
			StderrWriter: stderrWriter,
		}
		if bind != nil {
			ecfg.ArgEnv = bind.ScalarEnv
			ecfg.ArgsJSON = bind.ArgsJSON
			ecfg.ArgValues = bind.Values
			ecfg.LineRedactor = events.NewLineRedactor(bind.SecretValues)
		}

		results, err := executor.RunScripts(context.Background(), scriptDir, ecfg)
		status := "completed"
		if err != nil {
			status = "failed"
		} else {
			for _, r := range results {
				if r.ExitCode != 0 {
					status = "failed"
					break
				}
			}
		}
		if emitter != nil {
			emitter.EmitRunFinish(runID, status, err)
		}

		if reportFile != "" && (reportFormat != "json" && reportFormat != "yaml") {
			return fmt.Errorf("[x] --report-file requires --report=json or --report=yaml")
		}

		// Report to stdout or file
		if reportFormat == "json" || reportFormat == "yaml" {
			if err := writeReport(results, reportFormat, reportFile); err != nil {
				fmt.Fprintf(os.Stderr, "[x] Report error: %v\n", err)
			}
		}

		return err
	}
}

func printReport(results []executor.ScriptResult, format string) error {
	switch format {
	case "json":
		out, err := json.MarshalIndent(results, "", "  ")
		if err != nil {
			return err
		}
		fmt.Println(string(out))
	case "yaml":
		out, err := yaml.Marshal(results)
		if err != nil {
			return err
		}
		fmt.Print(string(out))
	default:
		return fmt.Errorf("unsupported report format: %s", format)
	}
	return nil
}

func writeReport(results []executor.ScriptResult, format, outPath string) error {
	var data []byte
	var err error

	switch format {
	case "json":
		data, err = json.MarshalIndent(results, "", "  ")
	case "yaml":
		data, err = yaml.Marshal(results)
	default:
		return fmt.Errorf("unsupported report format: %s", format)
	}

	if err != nil {
		return err
	}

	if outPath == "" {
		fmt.Print(string(data))
	} else {
		if err := os.WriteFile(outPath, data, 0644); err != nil {
			return fmt.Errorf("write to %s: %w", outPath, err)
		}
		fmt.Printf("[OK] Report written to %s\n", outPath)
	}

	return nil
}

func writePlanArtifact(plan types.Plan, runDir string) error {
	if runDir == "" {
		return fmt.Errorf("missing run directory")
	}
	if err := os.MkdirAll(runDir, 0o700); err != nil {
		return fmt.Errorf("create run dir: %w", err)
	}
	planPath := filepath.Join(runDir, "plan.json")
	f, err := os.OpenFile(planPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return fmt.Errorf("open plan file: %w", err)
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	if err := enc.Encode(plan); err != nil {
		return fmt.Errorf("write plan: %w", err)
	}
	return nil
}
