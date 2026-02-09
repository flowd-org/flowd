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
// Path refers to the job directory containing the config sentinel.
// ID uses canonical slash format derived from the job directory path.
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

// Discover scans root (typically "scripts") for config.yaml (primary)
// and config.d/config.yaml (legacy) sentinels and returns job metadata.
func Discover(root string) (Result, error) {
	return discoverWithMountPath(root, ".")
}

// DiscoverWithMountPath scans root for job sentinels and applies mountPath
// as the canonical ID prefix (Core SoT 1.5).
func DiscoverWithMountPath(root, mountPath string) (Result, error) {
	mountPath = strings.TrimSpace(mountPath)
	if mountPath == "" {
		mountPath = "."
	}
	return discoverWithMountPath(root, mountPath)
}

func discoverWithMountPath(root, mountPath string) (Result, error) {
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

	type sentinelPaths struct {
		primary string
		legacy  string
	}

	cfgByDir := make(map[string]*sentinelPaths)
	walkErr := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && strings.EqualFold(d.Name(), "config.yaml") {
			dir := filepath.Dir(path)
			jobDir := dir
			legacy := false
			if filepath.Base(dir) == "config.d" {
				legacy = true
				jobDir = filepath.Dir(dir)
			}
			sentinel := cfgByDir[jobDir]
			if sentinel == nil {
				sentinel = &sentinelPaths{}
				cfgByDir[jobDir] = sentinel
			}
			if legacy {
				sentinel.legacy = path
			} else {
				sentinel.primary = path
			}
		}
		return nil
	})
	if walkErr != nil {
		return res, fmt.Errorf("walk root: %w", walkErr)
	}

	jobDirs := make([]string, 0, len(cfgByDir))
	for jobDir, sentinels := range cfgByDir {
		if sentinels.primary != "" && sentinels.legacy != "" {
			return res, &configloader.DualConfigError{
				ScriptDir:   jobDir,
				PrimaryPath: sentinels.primary,
				LegacyPath:  sentinels.legacy,
			}
		}
		jobDirs = append(jobDirs, jobDir)
	}

	sort.Strings(jobDirs)
	for _, jobDir := range jobDirs {
		sentinels := cfgByDir[jobDir]
		cfgPath := sentinels.primary
		if cfgPath == "" {
			cfgPath = sentinels.legacy
		}
		jobs, err := parseConfig(root, jobDir, cfgPath, mountPath)
		if err != nil {
			errPath := cfgPath
			var idErr JobIDError
			if errors.As(err, &idErr) {
				return res, InvalidJobIDError{
					JobDir:  jobDir,
					Path:    idErr.Path,
					Segment: idErr.Segment,
					Reason:  idErr.Reason,
				}
			}
			res.Errors = append(res.Errors, DiscoveryError{Path: errPath, Err: err.Error()})
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

func parseConfig(root, jobDir, cfgPath, mountPath string) ([]JobInfo, error) {
	data, err := os.ReadFile(cfgPath)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}

	canonicalID, err := canonicalJobIDFromDir(root, jobDir, mountPath)
	if err != nil {
		return nil, err
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
		return []JobInfo{{
			ID:   canonicalID,
			Name: canonicalID,
			Path: jobDir,
		}}, nil
	}

	jobs := make([]JobInfo, 0, len(blocks))
	for _, block := range blocks {
		name := block.Name
		if name == "" {
			name = canonicalID
		}
		jobs = append(jobs, JobInfo{
			ID:      canonicalID,
			Name:    name,
			Summary: block.Summary,
			Path:    jobDir,
		})
	}
	return jobs, nil
}

func canonicalJobIDFromDir(root, jobDir, mountPath string) (string, error) {
	rel, err := filepath.Rel(root, jobDir)
	if err != nil {
		rel = jobDir
	}
	rel = filepath.ToSlash(rel)
	if rel == "" {
		rel = "."
	}
	return CanonicalJobID(mountPath, rel)
}
