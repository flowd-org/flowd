// SPDX-License-Identifier: AGPL-3.0-or-later
package handlers

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/flowd-org/flowd/internal/server/sourcestore"
	"gopkg.in/yaml.v3"
)

var (
	addonSemverPattern = regexp.MustCompile(`^\d+\.\d+\.\d+(?:-[0-9A-Za-z.-]+)?(?:\+[0-9A-Za-z.-]+)?$`)
	addonIDPattern     = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9_.-]{1,62}[a-z0-9])$`)
	addonJobIDPattern  = regexp.MustCompile(`^[a-z][a-z0-9_.:-]{2,}$`)
	validArgTypes      = map[string]struct{}{
		"string":  struct{}{},
		"integer": struct{}{},
		"number":  struct{}{},
		"boolean": struct{}{},
		"array":   struct{}{},
		"object":  struct{}{},
	}
	validArgFormats = map[string]struct{}{
		"":          struct{}{},
		"path":      struct{}{},
		"file":      struct{}{},
		"directory": struct{}{},
		"secret":    struct{}{},
	}
)

type addonManifest struct {
	APIVersion string                 `yaml:"apiVersion" json:"apiVersion"`
	Kind       string                 `yaml:"kind" json:"kind"`
	Metadata   *addonManifestMeta     `yaml:"metadata" json:"metadata"`
	Requires   *addonManifestRequires `yaml:"requires" json:"requires"`
	Jobs       []addonManifestJob     `yaml:"jobs" json:"jobs"`
}

type addonManifestMeta struct {
	Name        string                    `yaml:"name" json:"name"`
	ID          string                    `yaml:"id" json:"id"`
	Version     string                    `yaml:"version" json:"version"`
	Summary     string                    `yaml:"summary" json:"summary"`
	Description string                    `yaml:"description" json:"description"`
	Homepage    string                    `yaml:"homepage" json:"homepage"`
	Maintainers []addonManifestMaintainer `yaml:"maintainers" json:"maintainers"`
	License     string                    `yaml:"license" json:"license"`
}

type addonManifestMaintainer struct {
	Name  string `yaml:"name" json:"name"`
	Email string `yaml:"email" json:"email"`
	URL   string `yaml:"url" json:"url"`
}

type addonManifestRequires struct {
	Runner      map[string]string    `yaml:"flwd" json:"flwd"`
	Permissions []string             `yaml:"permissions" json:"permissions"`
	Containers  []addonManifestImage `yaml:"containers" json:"containers"`
}

type addonManifestImage struct {
	Image            string  `yaml:"image" json:"image"`
	Platform         string  `yaml:"platform" json:"platform"`
	VerifySignatures *string `yaml:"verify_signatures" json:"verify_signatures"`
}

type addonManifestJob struct {
	ID           string                `yaml:"id" json:"id"`
	Name         string                `yaml:"name" json:"name"`
	Summary      string                `yaml:"summary" json:"summary"`
	Description  string                `yaml:"description" json:"description"`
	Extends      []string              `yaml:"extends" json:"extends"`
	Argspec      *addonManifestArgspec `yaml:"argspec" json:"argspec"`
	Requirements addonManifestJobReqs  `yaml:"requirements" json:"requirements"`
}

type addonManifestJobReqs struct {
	Tools []addonManifestTool `yaml:"tools" json:"tools"`
}

type addonManifestTool struct {
	Name    string `yaml:"name" json:"name"`
	Version string `yaml:"version" json:"version"`
}

type addonManifestArgspec struct {
	Args []addonManifestArg `yaml:"args" json:"args"`
}

type addonManifestArg struct {
	Name        string      `yaml:"name" json:"name"`
	Title       string      `yaml:"title" json:"title"`
	Description string      `yaml:"description" json:"description"`
	Type        string      `yaml:"type" json:"type"`
	Format      string      `yaml:"format" json:"format"`
	Secret      bool        `yaml:"secret" json:"secret"`
	Required    bool        `yaml:"required" json:"required"`
	Default     interface{} `yaml:"default" json:"default"`
	Enum        []string    `yaml:"enum" json:"enum"`
	ItemsType   string      `yaml:"items_type" json:"items_type"`
	ItemsEnum   []string    `yaml:"items_enum" json:"items_enum"`
	ValueType   string      `yaml:"value_type" json:"value_type"`
	MinLength   *int        `yaml:"minLength" json:"minLength"`
	MaxLength   *int        `yaml:"maxLength" json:"maxLength"`
	MinItems    *int        `yaml:"minItems" json:"minItems"`
	MaxItems    *int        `yaml:"maxItems" json:"maxItems"`
	Deprecated  bool        `yaml:"deprecated" json:"deprecated"`
	Minimum     interface{} `yaml:"minimum" json:"minimum"`
	Maximum     interface{} `yaml:"maximum" json:"maximum"`
	MultipleOf  interface{} `yaml:"multipleOf" json:"multipleOf"`
}

func parseAndValidateAddonManifest(data []byte) (*addonManifest, []string, error) {
	var manifest addonManifest
	if err := yaml.Unmarshal(data, &manifest); err != nil {
		return nil, nil, err
	}

	var errs []string

	if manifest.APIVersion != "flwd.addon/v1" {
		errs = append(errs, "apiVersion must be flwd.addon/v1")
	}
	if manifest.Kind != "AddOn" {
		errs = append(errs, "kind must be AddOn")
	}
	if manifest.Metadata == nil {
		errs = append(errs, "metadata is required")
	} else {
		meta := manifest.Metadata
		if strings.TrimSpace(meta.Name) == "" {
			errs = append(errs, "metadata.name is required")
		} else if len([]rune(strings.TrimSpace(meta.Name))) < 3 {
			errs = append(errs, "metadata.name must be at least 3 characters per "+manifestSchemaRef)
		}
		if !addonIDPattern.MatchString(meta.ID) {
			errs = append(errs, "metadata.id must match ^[a-z0-9](?:[a-z0-9_.-]{1,62}[a-z0-9])$")
		}
		if !addonSemverPattern.MatchString(meta.Version) {
			errs = append(errs, "metadata.version must be a SemVer string (e.g., 1.2.3)")
		}
		if len(meta.Summary) > 240 {
			errs = append(errs, "metadata.summary must be <=240 characters per "+manifestSchemaRef)
		}
		for i, maint := range meta.Maintainers {
			if strings.TrimSpace(maint.Name) == "" {
				errs = append(errs, fmt.Sprintf("metadata.maintainers[%d].name is required", i))
			}
		}
	}
	if manifest.Requires == nil {
		errs = append(errs, "requires section is required")
	} else {
		for i, container := range manifest.Requires.Containers {
			if strings.TrimSpace(container.Image) == "" {
				errs = append(errs, fmt.Sprintf("requires.containers[%d].image is required", i))
			}
			if container.VerifySignatures != nil {
				mode := strings.ToLower(strings.TrimSpace(*container.VerifySignatures))
				switch mode {
				case "required", "permissive", "disabled":
				case "":
					// treat empty as unset
				default:
					errs = append(errs, fmt.Sprintf("requires.containers[%d].verify_signatures must be required|permissive|disabled", i))
				}
			}
		}
	}
	if len(manifest.Jobs) == 0 {
		errs = append(errs, "jobs must contain at least one entry")
	} else {
		for i, job := range manifest.Jobs {
			prefix := fmt.Sprintf("jobs[%d]", i)
			if !addonJobIDPattern.MatchString(job.ID) {
				errs = append(errs, fmt.Sprintf("%s.id must match ^[a-z][a-z0-9_.:-]{2,}$", prefix))
			}
			if strings.TrimSpace(job.Name) == "" {
				errs = append(errs, fmt.Sprintf("%s.name is required", prefix))
			}
			if strings.TrimSpace(job.Summary) == "" {
				errs = append(errs, fmt.Sprintf("%s.summary is required", prefix))
			} else if len([]rune(job.Summary)) > 240 {
				errs = append(errs, fmt.Sprintf("%s.summary must be <=240 characters per %s", prefix, manifestSchemaRef))
			}
			if job.Argspec == nil {
				errs = append(errs, fmt.Sprintf("%s.argspec is required", prefix))
				continue
			}
			for j, arg := range job.Argspec.Args {
				argPrefix := fmt.Sprintf("%s.args[%d]", prefix, j)
				if strings.TrimSpace(arg.Name) == "" {
					errs = append(errs, fmt.Sprintf("%s.name is required", argPrefix))
				}
				if _, ok := validArgTypes[arg.Type]; !ok {
					errs = append(errs, fmt.Sprintf("%s.type %q is invalid", argPrefix, arg.Type))
				}
				if _, ok := validArgFormats[arg.Format]; !ok {
					errs = append(errs, fmt.Sprintf("%s.format %q is invalid", argPrefix, arg.Format))
				}
			}
		}
	}

	schemaErrs, schemaValidationErr := validateManifestSchemaConstraints(data)
	if schemaValidationErr != nil {
		return nil, nil, schemaValidationErr
	}
	errs = append(errs, schemaErrs...)

	return &manifest, errs, nil
}

func manifestSummary(m *addonManifest) map[string]any {
	if m == nil {
		return nil
	}
	summary := map[string]any{
		"jobs": len(m.Jobs),
	}
	if m.Metadata != nil {
		if m.Metadata.Name != "" {
			summary["name"] = m.Metadata.Name
		}
		if m.Metadata.ID != "" {
			summary["id"] = m.Metadata.ID
		}
		if m.Metadata.Version != "" {
			summary["version"] = m.Metadata.Version
		}
	}
	return summary
}

func loadAddonManifestFromSource(src sourcestore.Source) (*addonManifest, error) {
	if src.Metadata == nil {
		return nil, fmt.Errorf("manifest metadata missing")
	}
	rawPath, ok := src.Metadata["manifest_path"]
	if !ok {
		return nil, fmt.Errorf("manifest path missing")
	}
	path, ok := rawPath.(string)
	if !ok || strings.TrimSpace(path) == "" {
		return nil, fmt.Errorf("manifest path invalid")
	}
	path = filepath.Clean(path)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read manifest: %w", err)
	}
	manifest, errs, parseErr := parseAndValidateAddonManifest(data)
	if parseErr != nil {
		return nil, fmt.Errorf("parse manifest: %w", parseErr)
	}
	if len(errs) > 0 {
		return nil, fmt.Errorf("manifest validation failed: %s", strings.Join(errs, "; "))
	}
	return manifest, nil
}

func composeOCIJobID(sourceName, jobID string) string {
	sourceName = strings.TrimSpace(sourceName)
	jobID = strings.TrimSpace(jobID)
	if sourceName == "" {
		return jobID
	}
	if jobID == "" {
		return sourceName
	}
	return fmt.Sprintf("%s/%s", sourceName, jobID)
}
