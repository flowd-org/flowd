// SPDX-License-Identifier: AGPL-3.0-or-later

package configloader

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/flowd-org/flowd/internal/paths"
	"github.com/flowd-org/flowd/internal/types"
	"gopkg.in/yaml.v3"
)

// fullConfig retained for potential future extended parsing; current path decodes directly into types.Config

func LoadConfig(scriptDir string) (*types.Config, error) {
	configPath := filepath.Join(scriptDir, "config.d", "config.yaml")

	f, err := os.Open(configPath)
	if err != nil {
		return nil, fmt.Errorf("open config: %w", err)
	}
	defer f.Close()

	var cfg types.Config
	decoder := yaml.NewDecoder(f)
	if err := decoder.Decode(&cfg); err != nil {
		return nil, fmt.Errorf("decode config: %w", err)
	}

	// Normalise alias definitions (Phase 7)
	normalised := make([]types.CommandAlias, 0, len(cfg.Aliases))
	for _, alias := range cfg.Aliases {
		from := strings.TrimSpace(alias.From)
		to := strings.TrimSpace(alias.To)
		if from == "" || to == "" {
			continue
		}
		if strings.Contains(to, "/") {
			return nil, fmt.Errorf("invalid alias %q -> %q: target must be single-level", from, to)
		}
		normalised = append(normalised, types.CommandAlias{
			From:        from,
			To:          to,
			Description: strings.TrimSpace(alias.Description),
		})
	}
	cfg.Aliases = normalised

	// Resolve data directory precedence: explicit env in config > process env > platform default.
	dataDir := ""
	if cfg.Env != nil {
		if val, ok := cfg.Env["DATA_DIR"]; ok && strings.TrimSpace(val) != "" {
			dataDir = strings.TrimSpace(val)
		}
	}
	if dataDir == "" {
		if env := os.Getenv("DATA_DIR"); env != "" {
			dataDir = env
		}
	}
	if dataDir != "" {
		paths.SetDataDirOverride(dataDir)
	}
	resolvedDataDir := paths.DataDir()
	paths.SetDataDirOverride(resolvedDataDir)
	if cfg.Env == nil {
		cfg.Env = make(map[string]string)
	}
	cfg.Env["DATA_DIR"] = resolvedDataDir

	// Backward-compat mapping: if ArgSpec absent but legacy Arguments present, synthesize ArgSpec
	if (cfg.ArgSpec == nil || len(cfg.ArgSpec.Args) == 0) && len(cfg.Arguments) > 0 {
		as := types.ArgSpec{Args: make([]types.Arg, 0, len(cfg.Arguments))}
		for name, def := range cfg.Arguments {
			t := def.Type
			switch t {
			case "bool":
				t = "boolean"
			case "int":
				t = "integer"
			case "string":
				t = "string"
			}
			a := types.Arg{
				Name:        name,
				Type:        t,
				Required:    def.Required,
				Description: def.Description,
				Default:     def.Default,
			}
			if len(def.Choices) > 0 {
				a.Enum = append(a.Enum, def.Choices...)
			}
			as.Args = append(as.Args, a)
		}
		cfg.ArgSpec = &as
	}

	return &cfg, nil
}
