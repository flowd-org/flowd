// SPDX-License-Identifier: AGPL-3.0-or-later
package executor

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/flowd-org/flowd/internal/types"
)

func GenerateRunnerProfile(scriptDir string, interp string, verbosity int, spec *types.ArgSpec, argValues map[string]interface{}) (string, func(), error) {
	var ext, profileHeader string
	switch {
	case strings.Contains(interp, "bash"):
		ext = ".sh"
		profileHeader = "#!/bin/bash\n"
	case strings.Contains(interp, "pwsh"), strings.Contains(interp, "powershell"):
		ext = ".ps1"
		profileHeader = `# PowerShell flwd profile
param (
  [string]$TargetScript,
  [Parameter(ValueFromRemainingArguments = $true)][string[]]$ScriptArgs
)
` + "\n"
	default:
		return "", nil, fmt.Errorf("unsupported interpreter for profile: %s", interp)
	}

	// Determine the path hierarchy beginning at the nearest "scripts" segment.
	// This supports both local jobs (./scripts/...) and materialized sources
	// (e.g., dataDir/sources/<name>/scripts/...). If no "scripts" component is
	// present, fall back to using the provided scriptDir only.
	cleanDir := filepath.Clean(scriptDir)
	comps := strings.Split(cleanDir, string(filepath.Separator))
	// Handle leading separator on UNIX producing empty first element
	baseIdx := 0
	for i, c := range comps {
		if c == "scripts" {
			baseIdx = i
			break
		}
	}
	if baseIdx == 0 && (len(comps) == 0 || comps[0] != "scripts") {
		// No scripts component found; just use scriptDir
		baseIdx = -1
	}

	var levels []string
	if baseIdx >= 0 {
		for i := baseIdx; i < len(comps); i++ {
			part := filepath.Join(comps[:i+1]...)
			levels = append(levels, part)
		}
	} else {
		levels = []string{cleanDir}
	}

	var lines []string
	lines = append(lines, fmt.Sprintf("# Auto-generated flwd profile (%s)", time.Now().Format(time.RFC3339)))
	if ext == ".ps1" {
		lines = append([]string{profileHeader}, lines...)
	}

	for _, dir := range levels {
		varsDir := filepath.Join(dir, "config.d", "vars")
		libsDir := filepath.Join(dir, "config.d", "libs")

		if ext == ".sh" {
			varsAbs, _ := filepath.Abs(varsDir)
			libsAbs, _ := filepath.Abs(libsDir)

			lines = append(lines,
				fmt.Sprintf("# Loading: %s", varsAbs),
				fmt.Sprintf(`for f in "%s"/*.sh; do [ -f "$f" ] && source "$f"; done`, varsAbs),
				fmt.Sprintf("# Loading: %s", libsAbs),
				fmt.Sprintf(`for f in "%s"/*.sh; do [ -f "$f" ] && source "$f"; done`, libsAbs),
			)
		}

		if ext == ".ps1" {
			varsAbs, _ := filepath.Abs(varsDir)
			libsAbs, _ := filepath.Abs(libsDir)

			lines = append(lines,
				fmt.Sprintf("# Loading: %s", varsAbs),
				fmt.Sprintf(`if (Test-Path "%s") { Get-ChildItem "%s" -Filter *.ps1 | ForEach-Object { . $_.FullName } }`, varsAbs, varsAbs),
				fmt.Sprintf("# Loading: %s", libsAbs),
				fmt.Sprintf(`if (Test-Path "%s") { Get-ChildItem "%s" -Filter *.ps1 | ForEach-Object { . $_.FullName } }`, libsAbs, libsAbs),
			)
		}
	}

	if ext == ".sh" && spec != nil && len(spec.Args) > 0 {
		lines = append(lines, "", "# ArgSpec bindings (auto-generated)")
		for _, arg := range spec.Args {
			value, ok := argValues[arg.Name]
			if !ok {
				continue
			}
			varName := sanitizeVarName(arg.Name)
			switch arg.Type {
			case "string", "boolean", "integer":
				if s, ok := value.(string); ok {
					lines = append(lines, fmt.Sprintf("%s=%s", varName, shellQuote(s)))
				} else {
					lines = append(lines, fmt.Sprintf("%s=%v", varName, value))
				}
				lines = append(lines, fmt.Sprintf("export %s", varName))
			case "array":
				arr, ok := value.([]string)
				if !ok {
					// binding stores []interface{} sometimes; attempt conversion
					arr = toStringSlice(value)
				}
				if len(arr) > 0 {
					lines = append(lines, fmt.Sprintf("declare -a %s=(%s)", varName, joinShellList(arr)))
				} else {
					lines = append(lines, fmt.Sprintf("declare -a %s=()", varName))
				}
			case "object":
				m := toStringMap(value)
				lines = append(lines, fmt.Sprintf("declare -A %s=(%s)", varName, joinShellMap(m)))
			default:
				// Unsupported types skipped for now
			}
		}
	}

	if ext == ".ps1" {
		lines = append(lines,
			"",
			"# Export --arg[=value] pairs to environment variables",
			`foreach ($arg in $ScriptArgs) {`,
			`  if ($arg -match "^--([^=]+)=(.+)$") {`,
			`    $name = $matches[1]`,
			`    $value = $matches[2]`,
			`    Set-Item -Path "env:$name" -Value $value`,
			`  } elseif ($arg -match "^--(.+)$") {`,
			`    $name = $matches[1]`,
			`    Set-Item -Path "env:$name" -Value "true"`,
			`  }`,
			`}`,
			"",
			"# Resolve full script path before execution",
			//`$TargetScript = Resolve-Path -LiteralPath $TargetScript | Select-Object -ExpandProperty Path`,
			`$TargetScript = (Resolve-Path -LiteralPath $TargetScript).Path.ToString()`,
			`Write-Host "[RUN] $TargetScript $ScriptArgs"`,
			`& $TargetScript @ScriptArgs`,
			`exit $LASTEXITCODE`,
		)
	}

	tmpFile, err := os.CreateTemp("", "flwd_profile_*"+ext)
	if err != nil {
		return "", nil, fmt.Errorf("create temp profile: %w", err)
	}

	profilePath := tmpFile.Name()
	if _, err := tmpFile.WriteString(strings.Join(lines, "\n") + "\n"); err != nil {
		tmpFile.Close()
		return "", nil, fmt.Errorf("write profile: %w", err)
	}

	tmpFile.Close()

	cleanup := func() {
		if verbosity < 3 {
			_ = os.Remove(profilePath)
		} else {
			fmt.Fprintf(os.Stderr, "[DEBUG] Preserved profile: %s\n", profilePath)
		}
	}

	return profilePath, cleanup, nil
}

func sanitizeVarName(name string) string {
	sanitized := strings.ReplaceAll(name, "-", "_")
	sanitized = strings.ReplaceAll(sanitized, ".", "_")
	if sanitized == "" {
		return "ARG"
	}
	return sanitized
}

func shellQuote(val string) string {
	return fmt.Sprintf("'%s'", strings.ReplaceAll(val, "'", "'\\''"))
}

func joinShellList(items []string) string {
	quoted := make([]string, 0, len(items))
	for _, it := range items {
		quoted = append(quoted, shellQuote(it))
	}
	return strings.Join(quoted, " ")
}

func joinShellMap(m map[string]string) string {
	if len(m) == 0 {
		return ""
	}
	parts := make([]string, 0, len(m))
	for k, v := range m {
		parts = append(parts, fmt.Sprintf("[%s]=%s", shellQuote(k), shellQuote(v)))
	}
	return strings.Join(parts, " ")
}

func toStringSlice(v interface{}) []string {
	switch vv := v.(type) {
	case []string:
		return vv
	case []interface{}:
		out := make([]string, 0, len(vv))
		for _, item := range vv {
			if s, ok := item.(string); ok {
				out = append(out, s)
			}
		}
		return out
	default:
		return nil
	}
}

func toStringMap(v interface{}) map[string]string {
	out := map[string]string{}
	if v == nil {
		return out
	}
	switch vv := v.(type) {
	case map[string]string:
		return vv
	case map[string]interface{}:
		for k, val := range vv {
			if s, ok := val.(string); ok {
				out[k] = s
			}
		}
	}
	return out
}
