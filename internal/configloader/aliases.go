// SPDX-License-Identifier: AGPL-3.0-or-later
package configloader

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/flowd-org/flowd/internal/types"
	"gopkg.in/yaml.v3"
)

// LoadAliases reads flwd.yaml under root and returns declared command aliases.
func LoadAliases(root string) ([]types.CommandAlias, error) {
	if strings.TrimSpace(root) == "" {
		return nil, nil
	}
	configPath := filepath.Join(root, "flwd.yaml")
	data, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read flwd.yaml: %w", err)
	}
	var payload struct {
		Aliases []types.CommandAlias `yaml:"aliases"`
	}
	if err := yaml.Unmarshal(data, &payload); err != nil {
		return nil, fmt.Errorf("parse flwd.yaml: %w", err)
	}
	if len(payload.Aliases) == 0 {
		return nil, nil
	}
	out := make([]types.CommandAlias, 0, len(payload.Aliases))
	seen := make(map[string]struct{}, len(payload.Aliases))
	for _, alias := range payload.Aliases {
		from := strings.TrimSpace(alias.From)
		to := strings.TrimSpace(alias.To)
		if from == "" || to == "" {
			return nil, fmt.Errorf("alias entries must include from and to")
		}
		key := from + "→" + to
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, types.CommandAlias{
			From:        from,
			To:          to,
			Description: strings.TrimSpace(alias.Description),
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].From == out[j].From {
			return out[i].To < out[j].To
		}
		return out[i].From < out[j].From
	})
	return out, nil
}
