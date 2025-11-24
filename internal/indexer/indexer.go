// SPDX-License-Identifier: AGPL-3.0-or-later
package indexer

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/flowd-org/flowd/internal/configloader"
	"gopkg.in/yaml.v3"
)

// JobInfo summarizes a discovered local job.
// Path refers to the directory containing config.d/config.yaml.
// ID defaults to the relative path when not provided explicitly.
// Summary is optional and may be empty.
type JobInfo struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Summary string `json:"summary,omitempty"`
	Path    string `json:"path"`
}

// DiscoveryError captures parsing or validation errors.
type DiscoveryError struct {
	Path string `json:"path"`
	Err  string `json:"error"`
}

// Result bundles discovered jobs and any errors encountered.
type Result struct {
	Jobs            []JobInfo                  `json:"jobs"`
	Aliases         []AliasInfo                `json:"aliases,omitempty"`
	AliasCollisions map[string][]AliasInfo     `json:"alias_collisions,omitempty"`
	AliasInvalid    map[string]AliasValidation `json:"alias_invalid,omitempty"`
	Errors          []DiscoveryError           `json:"errors,omitempty"`
}

// Discover scans root (typically "scripts") for config.d/config.yaml files
// and returns job metadata according to the Runner specification.
func Discover(root string) (Result, error) {
	var res Result

	info, err := os.Stat(root)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return res, nil
		}
		return res, fmt.Errorf("stat root: %w", err)
	}
	if !info.IsDir() {
		return res, fmt.Errorf("root %s is not a directory", root)
	}

	var cfgPaths []string
	walkErr := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && strings.EqualFold(d.Name(), "config.yaml") {
			if filepath.Base(filepath.Dir(path)) == "config.d" {
				cfgPaths = append(cfgPaths, path)
			}
		}
		return nil
	})
	if walkErr != nil {
		return res, fmt.Errorf("walk root: %w", walkErr)
	}

	sort.Strings(cfgPaths)
	for _, cfgPath := range cfgPaths {
		jobs, err := parseConfig(root, cfgPath)
		if err != nil {
			res.Errors = append(res.Errors, DiscoveryError{Path: cfgPath, Err: err.Error()})
			continue
		}
		res.Jobs = append(res.Jobs, jobs...)
	}

	aliases, err := configloader.LoadAliases(root)
	if err != nil {
		res.Errors = append(res.Errors, DiscoveryError{Path: filepath.Join(root, "flwd.yaml"), Err: err.Error()})
		return res, nil
	}
	if len(aliases) > 0 {
		aliasIndex, aliasErrs := BuildAliasIndex(res.Jobs, []AliasSet{{Source: "", Aliases: aliases}})
		res.Aliases = aliasIndex.Entries
		if len(aliasIndex.Collisions) > 0 {
			res.AliasCollisions = make(map[string][]AliasInfo, len(aliasIndex.Collisions))
			for key, list := range aliasIndex.Collisions {
				res.AliasCollisions[key] = append([]AliasInfo(nil), list...)
			}
		}
		if aliasIndex.Invalid != nil {
			res.AliasInvalid = make(map[string]AliasValidation, len(aliasIndex.Invalid))
			for key, val := range aliasIndex.Invalid {
				res.AliasInvalid[key] = val
			}
		}
		if len(aliasErrs) > 0 {
			res.Errors = append(res.Errors, aliasErrs...)
		}
	}

	return res, nil
}

type singleJob struct {
	Version string     `yaml:"version"`
	Job     jobBlock   `yaml:"job"`
	Jobs    []jobBlock `yaml:"jobs"`
}

type jobBlock struct {
	ID      string `yaml:"id"`
	Name    string `yaml:"name"`
	Summary string `yaml:"summary"`
}

func parseConfig(root, cfgPath string) ([]JobInfo, error) {
	data, err := os.ReadFile(cfgPath)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}

	var cfg singleJob
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse yaml: %w", err)
	}

	var blocks []jobBlock
	if cfg.Job.ID != "" || cfg.Job.Name != "" || cfg.Job.Summary != "" {
		blocks = append(blocks, cfg.Job)
	}
	if len(cfg.Jobs) > 0 {
		blocks = append(blocks, cfg.Jobs...)
	}
	if len(blocks) == 0 {
		derived := deriveID(root, cfgPath)
		return []JobInfo{{
			ID:   derived,
			Name: derived,
			Path: filepath.Dir(cfgPath),
		}}, nil
	}

	jobs := make([]JobInfo, 0, len(blocks))
	for _, block := range blocks {
		id := block.ID
		if id == "" {
			id = deriveID(root, cfgPath)
		}
		name := block.Name
		if name == "" {
			name = id
		}
		jobs = append(jobs, JobInfo{
			ID:      id,
			Name:    name,
			Summary: block.Summary,
			Path:    filepath.Dir(cfgPath),
		})
	}
	return jobs, nil
}

func deriveID(root, cfgPath string) string {
	jobDir := filepath.Dir(filepath.Dir(cfgPath)) // strip config.d/config.yaml
	rel, err := filepath.Rel(root, jobDir)
	if err != nil {
		return filepath.ToSlash(jobDir)
	}
	rel = filepath.ToSlash(rel)
	rel = strings.Trim(rel, "/")
	if rel == "" {
		return filepath.Base(jobDir)
	}
	return strings.ReplaceAll(rel, "/", ".")
}
