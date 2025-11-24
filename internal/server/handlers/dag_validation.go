// SPDX-License-Identifier: AGPL-3.0-or-later
package handlers

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/flowd-org/flowd/internal/server/response"
	"github.com/flowd-org/flowd/internal/types"
)

func isDAGConfig(cfg *types.Config) bool {
	if cfg == nil {
		return false
	}
	if len(cfg.Steps) == 0 {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(cfg.Composition), "steps")
}

func validateDAGConfig(cfg *types.Config) *response.Problem {
	if !isDAGConfig(cfg) {
		return nil
	}
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(cfg.Interpreter)), "container:") {
		prob := response.New(http.StatusUnprocessableEntity, "invalid dag configuration",
			response.WithExtension("code", "E_CONFIG"),
			response.WithDetail("interpreter container form is not allowed in DAG composition"))
		return &prob
	}
	executor := strings.ToLower(strings.TrimSpace(cfg.Executor))
	if executor == "" {
		prob := response.New(http.StatusUnprocessableEntity, "invalid dag configuration",
			response.WithExtension("code", "E_CONFIG"),
			response.WithDetail("executor is required for DAG jobs"))
		return &prob
	}
	if executor != "proc" && executor != "container" {
		prob := response.New(http.StatusUnprocessableEntity, "invalid dag configuration",
			response.WithExtension("code", "E_CONFIG"),
			response.WithDetail("executor must be proc or container for DAG jobs"))
		return &prob
	}
	if len(cfg.Steps) == 0 {
		prob := response.New(http.StatusUnprocessableEntity, "invalid dag configuration",
			response.WithExtension("code", "E_CONFIG"),
			response.WithDetail("steps array is required for DAG composition"))
		return &prob
	}
	ids := make(map[string]struct{})
	for idx, step := range cfg.Steps {
		if strings.TrimSpace(step.Script) == "" {
			prob := response.New(http.StatusUnprocessableEntity, "invalid dag step",
				response.WithExtension("code", "E_CONFIG"),
				response.WithDetail(detailPrefix(idx)+"script is required"))
			return &prob
		}
		if strings.TrimSpace(step.Executor) != "" {
			prob := response.New(http.StatusUnprocessableEntity, "mixed executors not allowed",
				response.WithExtension("code", "E_POLICY"),
				response.WithDetail(detailPrefix(idx)+"step-level executor is not permitted; set executor on job"))
			return &prob
		}
		id := strings.TrimSpace(step.ID)
		if id != "" {
			if _, exists := ids[id]; exists {
				prob := response.New(http.StatusUnprocessableEntity, "invalid dag step",
					response.WithExtension("code", "E_CONFIG"),
					response.WithDetail(detailPrefix(idx)+"duplicate step id"))
				return &prob
			}
			ids[id] = struct{}{}
		}
		if executor == "proc" {
			if containerConfigHasSettings(step.Container) {
				prob := response.New(http.StatusUnprocessableEntity, "invalid dag step",
					response.WithExtension("code", "E_CONFIG"),
					response.WithDetail(detailPrefix(idx)+"container settings are not allowed when executor is proc"))
				return &prob
			}
		} else if executor == "container" {
			if effectiveStepImage(step.Container, cfg.Container) == "" {
				prob := response.New(http.StatusUnprocessableEntity, "invalid dag step",
					response.WithExtension("code", "E_CONFIG"),
					response.WithDetail(detailPrefix(idx)+"container image must be specified at job or step level"))
				return &prob
			}
		}
	}
	for idx, step := range cfg.Steps {
		for _, need := range step.Needs {
			need = strings.TrimSpace(need)
			if need == "" {
				continue
			}
			if _, ok := ids[need]; !ok {
				prob := response.New(http.StatusUnprocessableEntity, "invalid dag step",
					response.WithExtension("code", "E_CONFIG"),
					response.WithDetail(detailPrefix(idx)+"needs references unknown step: "+need))
				return &prob
			}
		}
	}
	return nil
}

func detailPrefix(idx int) string {
	return "steps[" + strconv.Itoa(idx) + "]: "
}

func containerConfigHasSettings(cfg *types.ContainerConfig) bool {
	if cfg == nil {
		return false
	}
	if strings.TrimSpace(cfg.Image) != "" {
		return true
	}
	if cfg.Resources != nil {
		if strings.TrimSpace(cfg.Resources.CPU) != "" || strings.TrimSpace(cfg.Resources.Memory) != "" {
			return true
		}
	}
	if strings.TrimSpace(cfg.Network) != "" {
		return true
	}
	if cfg.RootfsWritable {
		return true
	}
	if len(cfg.Capabilities) > 0 || len(cfg.ExtraArgs) > 0 || len(cfg.Entrypoint) > 0 {
		return true
	}
	return false
}

func effectiveStepImage(stepCfg *types.ContainerConfig, jobCfg *types.ContainerConfig) string {
	if stepCfg != nil && strings.TrimSpace(stepCfg.Image) != "" {
		return strings.TrimSpace(stepCfg.Image)
	}
	if jobCfg != nil {
		return strings.TrimSpace(jobCfg.Image)
	}
	return ""
}

func mergeContainerConfig(jobCfg, stepCfg *types.ContainerConfig) *types.ContainerConfig {
	base := cloneContainerConfig(jobCfg)
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
	// Booleans override explicitly
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

func cloneContainerConfig(cfg *types.ContainerConfig) *types.ContainerConfig {
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
