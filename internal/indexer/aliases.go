// SPDX-License-Identifier: AGPL-3.0-or-later
package indexer

import (
	"fmt"
	"sort"
	"strings"

	"github.com/flowd-org/flowd/internal/types"
)

// AliasSet groups aliases originating from a specific source (empty source denotes local root).
type AliasSet struct {
	Source  string
	Aliases []types.CommandAlias
}

// AliasInfo describes a resolved alias entry.
type AliasInfo struct {
	Name        string `json:"name"`
	TargetPath  string `json:"target_path"`
	TargetID    string `json:"target_id"`
	Source      string `json:"source,omitempty"`
	Description string `json:"description,omitempty"`
}

// AliasIndex summarizes resolved aliases and any detected collisions by alias name.
type AliasIndex struct {
	Entries    []AliasInfo
	Collisions map[string][]AliasInfo
	Invalid    map[string]AliasValidation
}

// AliasValidation captures structured metadata for invalid alias definitions keyed by alias name.
type AliasValidation struct {
	Code   string `json:"code"`
	Detail string `json:"detail"`
}

// BuildAliasIndex validates alias definitions against discovered jobs and returns
// normalized alias entries plus any discovery errors encountered.
func BuildAliasIndex(jobs []JobInfo, aliasSets []AliasSet) (AliasIndex, []DiscoveryError) {
	if len(aliasSets) == 0 {
		return AliasIndex{}, nil
	}

	jobIDs := make(map[string]JobInfo, len(jobs))
	for _, job := range jobs {
		jobIDs[strings.ToLower(job.ID)] = job
	}

	byName := make(map[string][]AliasInfo)
	invalid := make(map[string]AliasValidation)
	var errs []DiscoveryError

	for _, set := range aliasSets {
		for _, alias := range set.Aliases {
			from := strings.TrimSpace(alias.From)
			to := strings.TrimSpace(alias.To)
			if from == "" || to == "" {
				errs = append(errs, DiscoveryError{
					Path: fmt.Sprintf("flwd.yaml (%s)", set.Source),
					Err:  "alias entries must include from and to",
				})
				continue
			}

			targetPath, targetID := normalizeAliasTarget(from)
			if targetID == "" {
				errs = append(errs, DiscoveryError{
					Path: fmt.Sprintf("flwd.yaml (%s)", set.Source),
					Err:  fmt.Sprintf("invalid alias target %q", from),
				})
				continue
			}

			aliasName := strings.TrimSpace(to)
			if aliasName == "" {
				errs = append(errs, DiscoveryError{
					Path: fmt.Sprintf("flwd.yaml (%s)", set.Source),
					Err:  fmt.Sprintf("alias %q target %q missing name", to, from),
				})
				continue
			}
			if strings.Contains(aliasName, "/") {
				errs = append(errs, DiscoveryError{
					Path: fmt.Sprintf("flwd.yaml (%s)", set.Source),
					Err:  fmt.Sprintf("alias name %q must be single-level (no '/')", aliasName),
				})
				continue
			}
			if strings.HasPrefix(aliasName, ":") {
				lower := strings.ToLower(aliasName)
				invalid[lower] = AliasValidation{Code: "alias.reserved", Detail: fmt.Sprintf("alias name %q uses reserved prefix", aliasName)}
				errs = append(errs, DiscoveryError{
					Path: fmt.Sprintf("flwd.yaml (%s)", set.Source),
					Err:  fmt.Sprintf("alias name %q cannot start with ':'", aliasName),
				})
				continue
			}

			job, ok := jobIDs[strings.ToLower(targetID)]
			if !ok {
				lower := strings.ToLower(aliasName)
				invalid[lower] = AliasValidation{Code: "alias.target.invalid", Detail: fmt.Sprintf("alias %q target %q not found", aliasName, from)}
				errs = append(errs, DiscoveryError{
					Path: fmt.Sprintf("flwd.yaml (%s)", set.Source),
					Err:  fmt.Sprintf("alias target %q not found", from),
				})
				continue
			}
			if strings.EqualFold(aliasName, job.ID) {
				lower := strings.ToLower(aliasName)
				invalid[lower] = AliasValidation{Code: "alias.name.conflict", Detail: fmt.Sprintf("alias name %q conflicts with job id", aliasName)}
				errs = append(errs, DiscoveryError{
					Path: fmt.Sprintf("flwd.yaml (%s)", set.Source),
					Err:  fmt.Sprintf("alias name %q conflicts with job id", aliasName),
				})
				continue
			}
			entry := AliasInfo{
				Name:        aliasName,
				TargetPath:  targetPath,
				TargetID:    job.ID,
				Source:      set.Source,
				Description: alias.Description,
			}
			key := strings.ToLower(aliasName)
			byName[key] = append(byName[key], entry)
		}
	}

	entries := make([]AliasInfo, 0, len(byName))
	collisions := make(map[string][]AliasInfo)
	for _, list := range byName {
		if len(list) == 0 {
			continue
		}
		// Preserve the first declaration for listing purposes.
		entries = append(entries, list[0])
		if len(list) > 1 {
			collisions[strings.ToLower(list[0].Name)] = append([]AliasInfo(nil), list...)
		}
	}

	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Source == entries[j].Source {
			return entries[i].Name < entries[j].Name
		}
		return entries[i].Source < entries[j].Source
	})

	sort.Slice(errs, func(i, j int) bool {
		if errs[i].Path == errs[j].Path {
			return errs[i].Err < errs[j].Err
		}
		return errs[i].Path < errs[j].Path
	})

	if len(invalid) == 0 {
		invalid = nil
	}

	return AliasIndex{Entries: entries, Collisions: collisions, Invalid: invalid}, errs
}

func normalizeAliasTarget(from string) (targetPath, targetID string) {
	trimmed := strings.TrimSpace(from)
	if trimmed == "" {
		return "", ""
	}
	normalized := strings.ReplaceAll(trimmed, "\\", "/")
	normalized = strings.ReplaceAll(normalized, ":", "/")
	normalized = strings.ReplaceAll(normalized, "..", ".")
	normalized = strings.ReplaceAll(normalized, " ", "")
	normalized = strings.ReplaceAll(normalized, "//", "/")
	normalized = strings.Trim(normalized, "/")
	if normalized == "" {
		return "", ""
	}
	targetPath = normalized
	targetID = strings.ReplaceAll(normalized, "/", ".")
	return targetPath, targetID
}
