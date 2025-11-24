// SPDX-License-Identifier: AGPL-3.0-or-later
package handlers

import (
	"context"
	"log/slog"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/flowd-org/flowd/internal/engine"
	"github.com/flowd-org/flowd/internal/policy"
	policyverify "github.com/flowd-org/flowd/internal/policy/verify"
	"github.com/flowd-org/flowd/internal/server/response"
	"github.com/flowd-org/flowd/internal/types"
)

func buildDAGPlan(ctx context.Context, jobID string, cfgObj *types.Config, spec *types.ArgSpec, binding *engine.Binding, effProfile string, policyCtx *policy.Context, verifier policyverify.ImageVerifier, runtime string) (types.Plan, []any, *response.Problem, error) {
	plan := engine.BuildPlan(jobID, cfgObj, spec, binding)
	if plan.ExecutorPreview == nil {
		plan.ExecutorPreview = map[string]interface{}{}
	}
	plan.ExecutorPreview["composition"] = "steps"
	executor := strings.ToLower(strings.TrimSpace(cfgObj.Executor))
	if executor != "" {
		plan.ExecutorPreview["executor"] = executor
	}
	if cfgObj.Container != nil && strings.TrimSpace(cfgObj.Container.Image) != "" {
		plan.ExecutorPreview["container_image"] = strings.TrimSpace(cfgObj.Container.Image)
	}
	plan.SecurityProfile = effProfile

	var stepPreviews []types.PlanStepPreview
	var allFindings []types.Finding
	mode, err := policyCtx.VerifyModeForProfile(effProfile)
	if err != nil {
		prob := response.New(http.StatusUnprocessableEntity, "policy error",
			response.WithExtension("code", "E_POLICY"),
			response.WithDetail(err.Error()))
		return types.Plan{}, nil, &prob, nil
	}

	imageSet := map[string]struct{}{}
	for idx, step := range cfgObj.Steps {
		merged := mergeContainerConfig(cfgObj.Container, step.Container)
		preview := types.PlanStepPreview{
			ID:       strings.TrimSpace(step.ID),
			Name:     strings.TrimSpace(step.Name),
			Executor: executor,
		}

		if executor == "container" {
			image := strings.TrimSpace(merged.Image)
			preview.ContainerImage = image
			preview.Network = strings.TrimSpace(merged.Network)
			preview.RootfsWritable = merged.RootfsWritable
			if len(merged.Capabilities) > 0 {
				preview.Capabilities = append([]string{}, merged.Capabilities...)
			}
			if merged.Resources != nil {
				preview.Resources = &types.ContainerResources{
					CPU:    strings.TrimSpace(merged.Resources.CPU),
					Memory: strings.TrimSpace(merged.Resources.Memory),
				}
			}
			if image != "" {
				imageSet[image] = struct{}{}
			}

			if prob := enforceRegistryAllowList(ctx, image, policyCtx); prob != nil {
				return types.Plan{}, nil, prob, nil
			}
			outcome, prob := enforceImageVerification(ctx, image, mode, verifier)
			if prob != nil {
				return types.Plan{}, nil, prob, nil
			}
			if mode != policy.VerifyModeDisabled {
				preview.ImageTrust = &types.ImageTrustPreview{
					Image:    image,
					Mode:     string(mode),
					Verified: outcome.Verified,
					Reason:   outcome.Reason,
				}
			}
			if !outcome.Verified && outcome.Mode == policy.VerifyModePermissive {
				reason := outcome.Reason
				if reason == "" {
					reason = "signature verification failed under permissive policy"
				}
				allFindings = append(allFindings, types.Finding{
					Code:    "image.signature.permissive",
					Level:   "warning",
					Message: withStepContext(idx, reason),
				})
			}

			stepCfg := &types.Config{Container: merged, Executor: cfgObj.Executor}
			if prob := enforceResourceCeilings(ctx, stepCfg, policyCtx.ContainerCeilings()); prob != nil {
				return types.Plan{}, nil, prob, nil
			}
			overrideFindings, _, prob := evaluateOverrides(ctx, stepCfg, effProfile, policyCtx)
			if prob != nil {
				return types.Plan{}, nil, prob, nil
			}
			if len(overrideFindings) > 0 {
				allFindings = append(allFindings, withStepFindings(idx, overrideFindings)...) // helper to annotate message
			}
		} else {
			overrideFindings, _, prob := evaluateOverrides(ctx, &types.Config{Container: merged, Executor: cfgObj.Executor}, effProfile, policyCtx)
			if prob != nil {
				return types.Plan{}, nil, prob, nil
			}
			if len(overrideFindings) > 0 {
				allFindings = append(allFindings, withStepFindings(idx, overrideFindings)...) // annotate
			}
		}

		stepPreviews = append(stepPreviews, preview)
	}

	if len(allFindings) > 0 {
		plan.PolicyFindings = append(plan.PolicyFindings, allFindings...)
	}
	plan.Steps = stepPreviews
	if len(stepPreviews) == 1 && stepPreviews[0].ImageTrust != nil {
		plan.ImageTrust = stepPreviews[0].ImageTrust
	}

	attrs := []any{
		slog.String("composition", "steps"),
		slog.String("executor", executor),
		slog.Int("steps", len(stepPreviews)),
	}
	if runtime != "" {
		attrs = append(attrs, slog.String("runtime", runtime))
	}
	if len(imageSet) > 0 {
		images := make([]string, 0, len(imageSet))
		for img := range imageSet {
			images = append(images, img)
		}
		sort.Strings(images)
		attrs = append(attrs, slog.String("images", strings.Join(images, ",")))
	}

	return plan, attrs, nil, nil
}

func withStepContext(idx int, message string) string {
	return "step " + strconv.Itoa(idx) + ": " + message
}

func withStepFindings(idx int, src []types.Finding) []types.Finding {
	if len(src) == 0 {
		return nil
	}
	out := make([]types.Finding, 0, len(src))
	for _, f := range src {
		copy := f
		if copy.Message != "" {
			copy.Message = withStepContext(idx, copy.Message)
		}
		out = append(out, copy)
	}
	return out
}
