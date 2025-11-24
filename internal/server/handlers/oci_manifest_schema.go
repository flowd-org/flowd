// SPDX-License-Identifier: AGPL-3.0-or-later
package handlers

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

const manifestSchemaRef = "docs/agent-context/SOT/addon_manifest_schema_1.2.0.yaml"

func validateManifestSchemaConstraints(data []byte) ([]string, error) {
	var raw map[string]any
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	var errs []string
	errs = append(errs, validateAllowedKeys(raw, allowedManifestKeys(), "manifest")...)

	if metaVal, ok := raw["metadata"]; ok {
		metaMap, mapOK := metaVal.(map[string]any)
		if !mapOK {
			errs = append(errs, fmt.Sprintf("metadata must be an object per %s", manifestSchemaRef))
		} else {
			errs = append(errs, validateAllowedKeys(metaMap, allowedMetadataKeys(), "metadata")...)
			if maintVal, ok := metaMap["maintainers"]; ok {
				maintSlice, ok := maintVal.([]any)
				if !ok {
					errs = append(errs, fmt.Sprintf("metadata.maintainers must be an array per %s", manifestSchemaRef))
				} else {
					for idx, entry := range maintSlice {
						maintMap, ok := entry.(map[string]any)
						if !ok {
							errs = append(errs, fmt.Sprintf("metadata.maintainers[%d] must be an object per %s", idx, manifestSchemaRef))
							continue
						}
						path := fmt.Sprintf("metadata.maintainers[%d]", idx)
						errs = append(errs, validateAllowedKeys(maintMap, allowedMaintainerKeys(), path)...)
						if name := strings.TrimSpace(fmt.Sprint(maintMap["name"])); name == "" {
							errs = append(errs, fmt.Sprintf("%s.name is required per %s", path, manifestSchemaRef))
						}
					}
				}
			}
		}
	}

	if requiresVal, ok := raw["requires"]; ok {
		reqMap, mapOK := requiresVal.(map[string]any)
		if !mapOK {
			errs = append(errs, fmt.Sprintf("requires must be an object per %s", manifestSchemaRef))
		} else {
			errs = append(errs, validateAllowedKeys(reqMap, allowedRequiresKeys(), "requires")...)
			if permsVal, ok := reqMap["permissions"]; ok {
				arr, ok := permsVal.([]any)
				if !ok {
					errs = append(errs, fmt.Sprintf("requires.permissions must be an array per %s", manifestSchemaRef))
				} else {
					for i, v := range arr {
						if _, ok := v.(string); !ok {
							errs = append(errs, fmt.Sprintf("requires.permissions[%d] must be a string per %s", i, manifestSchemaRef))
						}
					}
				}
			}
			if containersVal, ok := reqMap["containers"]; ok {
				arr, ok := containersVal.([]any)
				if !ok {
					errs = append(errs, fmt.Sprintf("requires.containers must be an array per %s", manifestSchemaRef))
				} else {
					for i, v := range arr {
						containerMap, ok := v.(map[string]any)
						if !ok {
							errs = append(errs, fmt.Sprintf("requires.containers[%d] must be an object per %s", i, manifestSchemaRef))
							continue
						}
						path := fmt.Sprintf("requires.containers[%d]", i)
						errs = append(errs, validateAllowedKeys(containerMap, allowedContainerKeys(), path)...)
						if image := strings.TrimSpace(fmt.Sprint(containerMap["image"])); image == "" {
							errs = append(errs, fmt.Sprintf("%s.image is required per %s", path, manifestSchemaRef))
						}
						if val, ok := containerMap["verify_signatures"]; ok && val != nil {
							if str, ok := val.(string); ok {
								allowed := map[string]struct{}{"required": {}, "permissive": {}, "disabled": {}}
								if _, ok := allowed[strings.ToLower(strings.TrimSpace(str))]; !ok {
									errs = append(errs, fmt.Sprintf("%s.verify_signatures must be required|permissive|disabled per %s", path, manifestSchemaRef))
								}
							} else {
								errs = append(errs, fmt.Sprintf("%s.verify_signatures must be a string or null per %s", path, manifestSchemaRef))
							}
						}
					}
				}
			}
		}
	}

	if jobsVal, ok := raw["jobs"]; ok {
		arr, ok := jobsVal.([]any)
		if !ok {
			errs = append(errs, fmt.Sprintf("jobs must be an array per %s", manifestSchemaRef))
		} else {
			for i, v := range arr {
				jobMap, ok := v.(map[string]any)
				if !ok {
					errs = append(errs, fmt.Sprintf("jobs[%d] must be an object per %s", i, manifestSchemaRef))
					continue
				}
				jobPath := fmt.Sprintf("jobs[%d]", i)
				errs = append(errs, validateAllowedKeys(jobMap, allowedJobKeys(), jobPath)...)
				if reqVal, ok := jobMap["requirements"]; ok {
					reqMap, ok := reqVal.(map[string]any)
					if !ok {
						errs = append(errs, fmt.Sprintf("%s.requirements must be an object per %s", jobPath, manifestSchemaRef))
					} else {
						errs = append(errs, validateAllowedKeys(reqMap, allowedJobRequirementsKeys(), jobPath+".requirements")...)
						if toolsVal, ok := reqMap["tools"]; ok {
							toolsArr, ok := toolsVal.([]any)
							if !ok {
								errs = append(errs, fmt.Sprintf("%s.requirements.tools must be an array per %s", jobPath, manifestSchemaRef))
							} else {
								for j, tool := range toolsArr {
									toolMap, ok := tool.(map[string]any)
									if !ok {
										errs = append(errs, fmt.Sprintf("%s.requirements.tools[%d] must be an object per %s", jobPath, j, manifestSchemaRef))
										continue
									}
									path := fmt.Sprintf("%s.requirements.tools[%d]", jobPath, j)
									errs = append(errs, validateAllowedKeys(toolMap, allowedToolKeys(), path)...)
									if name := strings.TrimSpace(fmt.Sprint(toolMap["name"])); name == "" {
										errs = append(errs, fmt.Sprintf("%s.name is required per %s", path, manifestSchemaRef))
									}
								}
							}
						}
					}
				}
				if argspecVal, ok := jobMap["argspec"]; ok {
					argspecMap, ok := argspecVal.(map[string]any)
					if !ok {
						errs = append(errs, fmt.Sprintf("%s.argspec must be an object per %s", jobPath, manifestSchemaRef))
					} else {
						errs = append(errs, validateAllowedKeys(argspecMap, allowedArgspecKeys(), jobPath+".argspec")...)
						if argsVal, ok := argspecMap["args"]; ok {
							argsArr, ok := argsVal.([]any)
							if !ok {
								errs = append(errs, fmt.Sprintf("%s.argspec.args must be an array per %s", jobPath, manifestSchemaRef))
							} else {
								for j, arg := range argsArr {
									argMap, ok := arg.(map[string]any)
									if !ok {
										errs = append(errs, fmt.Sprintf("%s.argspec.args[%d] must be an object per %s", jobPath, j, manifestSchemaRef))
										continue
									}
									path := fmt.Sprintf("%s.argspec.args[%d]", jobPath, j)
									errs = append(errs, validateAllowedKeys(argMap, allowedArgKeys(), path)...)
								}
							}
						}
					}
				}
			}
		}
	}

	return errs, nil
}

func validateAllowedKeys(m map[string]any, allowed map[string]struct{}, path string) []string {
	var errs []string
	for key := range m {
		if _, ok := allowed[key]; !ok {
			errs = append(errs, fmt.Sprintf("%s.%s is not allowed per %s", path, key, manifestSchemaRef))
		}
	}
	return errs
}

func allowedManifestKeys() map[string]struct{} {
	return map[string]struct{}{
		"apiVersion": {},
		"kind":       {},
		"metadata":   {},
		"requires":   {},
		"jobs":       {},
	}
}

func allowedMetadataKeys() map[string]struct{} {
	return map[string]struct{}{
		"name":        {},
		"id":          {},
		"version":     {},
		"summary":     {},
		"description": {},
		"homepage":    {},
		"maintainers": {},
		"license":     {},
	}
}

func allowedMaintainerKeys() map[string]struct{} {
	return map[string]struct{}{
		"name":  {},
		"email": {},
		"url":   {},
	}
}

func allowedRequiresKeys() map[string]struct{} {
	return map[string]struct{}{
		"flwd":        {},
		"permissions": {},
		"containers":  {},
	}
}

func allowedContainerKeys() map[string]struct{} {
	return map[string]struct{}{
		"image":             {},
		"platform":          {},
		"verify_signatures": {},
	}
}

func allowedJobKeys() map[string]struct{} {
	return map[string]struct{}{
		"id":           {},
		"name":         {},
		"summary":      {},
		"description":  {},
		"argspec":      {},
		"extends":      {},
		"requirements": {},
	}
}

func allowedJobRequirementsKeys() map[string]struct{} {
	return map[string]struct{}{
		"tools": {},
	}
}

func allowedToolKeys() map[string]struct{} {
	return map[string]struct{}{
		"name":    {},
		"version": {},
	}
}

func allowedArgspecKeys() map[string]struct{} {
	return map[string]struct{}{
		"args": {},
	}
}

func allowedArgKeys() map[string]struct{} {
	return map[string]struct{}{
		"name":        {},
		"title":       {},
		"description": {},
		"type":        {},
		"format":      {},
		"secret":      {},
		"required":    {},
		"default":     {},
		"enum":        {},
		"minLength":   {},
		"maxLength":   {},
		"minimum":     {},
		"maximum":     {},
		"multipleOf":  {},
		"items_type":  {},
		"items_enum":  {},
		"minItems":    {},
		"maxItems":    {},
		"value_type":  {},
		"deprecated":  {},
	}
}
