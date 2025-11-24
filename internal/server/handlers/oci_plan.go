// SPDX-License-Identifier: AGPL-3.0-or-later
package handlers

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"log/slog"

	"github.com/flowd-org/flowd/internal/engine"
	"github.com/flowd-org/flowd/internal/policy"
	"github.com/flowd-org/flowd/internal/server/requestctx"
	"github.com/flowd-org/flowd/internal/server/response"
	"github.com/flowd-org/flowd/internal/server/sourcestore"
	"github.com/flowd-org/flowd/internal/types"
)

func tryBuildOCIPlan(r *http.Request, req planRequest, cfg PlansConfig) (types.Plan, []any, bool, *response.Problem, error) {
	if cfg.Sources == nil {
		return types.Plan{}, nil, false, nil, nil
	}
	jobID := strings.TrimSpace(req.JobID)
	if jobID == "" {
		return types.Plan{}, nil, false, nil, nil
	}

	ctx := r.Context()
	for _, src := range cfg.Sources.List() {
		if !strings.EqualFold(src.Type, "oci") {
			continue
		}
		manifest, err := loadAddonManifestFromSource(src)
		if err != nil {
			continue
		}
		for _, job := range manifest.Jobs {
			if composeOCIJobID(src.Name, job.ID) != jobID {
				continue
			}
			plan, attrs, prob, err := buildOCIPlan(ctx, req, cfg, src, job)
			return plan, attrs, true, prob, err
		}
	}
	return types.Plan{}, nil, false, nil, nil
}

func buildOCIPlan(ctx context.Context, req planRequest, cfg PlansConfig, src sourcestore.Source, job addonManifestJob) (types.Plan, []any, *response.Problem, error) {
	effProfile, err := resolveEffectiveProfile(req.RequestedSecurityProfile, cfg.Profile)
	if err != nil {
		prob := response.New(http.StatusUnprocessableEntity, "invalid security profile",
			response.WithExtension("code", "E_POLICY"),
			response.WithDetail(err.Error()))
		return types.Plan{}, nil, &prob, nil
	}
	ctx = requestctx.WithEffectiveProfile(ctx, effProfile)

	runtimeVal := cfg.Runtime
	if runtimeVal == "" {
		detected, detectErr := detectContainerRuntime(nil)
		if detectErr != nil {
			prob := runtimeUnavailableProblem(detectErr)
			return types.Plan{}, nil, &prob, nil
		}
		runtimeVal = detected
	}
	runtimeStr := string(runtimeVal)
	if runtimeStr != "" {
		ctx = requestctx.WithRuntime(ctx, runtimeStr)
	}

	spec := convertManifestArgSpec(job.Argspec)
	var binding *engine.Binding
	if len(spec.Args) > 0 {
		bind, bindErr := validatePlanArgs(spec, req.Args)
		if bindErr != nil {
			var argErr *engine.ArgError
			if errors.As(bindErr, &argErr) {
				prob := response.New(http.StatusUnprocessableEntity, "argument validation failed",
					response.WithExtension("errors", []map[string]string{{"arg": argErr.Arg, "message": argErr.Msg}}))
				return types.Plan{}, nil, &prob, nil
			}
			prob := response.New(http.StatusBadRequest, "invalid arguments", response.WithDetail(bindErr.Error()))
			return types.Plan{}, nil, &prob, nil
		}
		binding = bind
	} else if len(req.Args) > 0 {
		prob := response.New(http.StatusBadRequest, "job does not accept arguments")
		return types.Plan{}, nil, &prob, nil
	}

	policyCtx := cfg.Policy
	if policyCtx == nil {
		var newCtxErr error
		policyCtx, newCtxErr = policy.NewContext(nil)
		if newCtxErr != nil {
			prob := response.New(http.StatusUnprocessableEntity, "policy error",
				response.WithExtension("code", "E_POLICY"),
				response.WithDetail(newCtxErr.Error()))
			return types.Plan{}, nil, &prob, nil
		}
	}

	imageRef := strings.TrimSpace(src.Ref)
	if imageRef == "" {
		prob := response.New(http.StatusInternalServerError, "oci source ref missing")
		return types.Plan{}, nil, &prob, nil
	}

	if prob := enforceRegistryAllowList(ctx, imageRef, policyCtx); prob != nil {
		return types.Plan{}, nil, prob, nil
	}

	mode, err := policyCtx.VerifyModeForProfile(effProfile)
	if err != nil {
		prob := response.New(http.StatusUnprocessableEntity, "policy error",
			response.WithExtension("code", "E_POLICY"),
			response.WithDetail(err.Error()))
		return types.Plan{}, nil, &prob, nil
	}

	verifyImage := imageRef
	if digest := strings.TrimSpace(src.Digest); digest != "" {
		verifyImage = appendDigestReference(imageRef, digest)
	}

	outcome, prob := enforceImageVerification(ctx, verifyImage, mode, cfg.Verifier)
	if prob != nil {
		return types.Plan{}, nil, prob, nil
	}

	var findings []types.Finding
	if mode != policy.VerifyModeDisabled && !outcome.Verified && outcome.Mode == policy.VerifyModePermissive {
		reason := outcome.Reason
		if reason == "" {
			reason = "signature verification failed under permissive policy"
		}
		findings = append(findings, types.Finding{
			Code:    "image.signature.permissive",
			Level:   "warning",
			Message: reason,
		})
	}

	digest := strings.TrimSpace(src.Digest)
	if digest == "" && strings.EqualFold(strings.TrimSpace(src.PullPolicy), "on-run") {
		findings = append(findings, types.Finding{
			Code:    "oci.digest.missing",
			Level:   "warning",
			Message: "resolved digest unavailable; pull_policy=on-run",
		})
	}

	plan := engine.BuildPlan(req.JobID, nil, &spec, binding)
	plan.SecurityProfile = effProfile
	plan.Provenance = map[string]interface{}{"source": sourceToProvenance(src)}
	if plan.ExecutorPreview == nil {
		plan.ExecutorPreview = map[string]interface{}{}
	}
	plan.ExecutorPreview["executor"] = "container"
	plan.ExecutorPreview["container_image"] = imageRef
	if digest != "" {
		plan.ExecutorPreview["resolved_digest"] = digest
	}
	if pullPolicy := strings.TrimSpace(src.PullPolicy); pullPolicy != "" {
		plan.ExecutorPreview["pull_policy"] = pullPolicy
	}

	if mode != policy.VerifyModeDisabled {
		plan.ImageTrust = &types.ImageTrustPreview{
			Image:    verifyImage,
			Mode:     string(mode),
			Verified: outcome.Verified,
			Reason:   outcome.Reason,
		}
	}

	if len(findings) > 0 {
		plan.PolicyFindings = findings
	}

	if len(job.Requirements.Tools) > 0 {
		tools := make([]types.ToolRequirement, 0, len(job.Requirements.Tools))
		for _, tool := range job.Requirements.Tools {
			tools = append(tools, types.ToolRequirement{
				Name:    tool.Name,
				Version: tool.Version,
			})
		}
		plan.Requirements = &types.PlanRequirements{Tools: tools, Status: "unknown"}
	}

	attrs := []any{
		slog.String("job_id", req.JobID),
		slog.String("security_profile", effProfile),
		slog.String("source", src.Name),
		slog.String("image", imageRef),
	}
	if digest != "" {
		attrs = append(attrs, slog.String("resolved_digest", digest))
	}

	return plan, attrs, nil, nil
}

func convertManifestArgSpec(spec *addonManifestArgspec) types.ArgSpec {
	if spec == nil || len(spec.Args) == 0 {
		return types.ArgSpec{}
	}
	args := make([]types.Arg, 0, len(spec.Args))
	for _, a := range spec.Args {
		args = append(args, manifestArgToTypeArg(a))
	}
	return types.ArgSpec{Args: args}
}

func manifestArgToTypeArg(a addonManifestArg) types.Arg {
	arg := types.Arg{
		Name:        a.Name,
		Type:        a.Type,
		Format:      a.Format,
		Secret:      a.Secret,
		Required:    a.Required,
		Default:     a.Default,
		Description: a.Description,
		Enum:        a.Enum,
		ItemsType:   a.ItemsType,
		ItemsEnum:   a.ItemsEnum,
		ValueType:   a.ValueType,
	}
	return arg
}

func appendDigestReference(ref, digest string) string {
	if strings.Contains(ref, "@") {
		base := strings.Split(ref, "@")[0]
		return fmt.Sprintf("%s@%s", strings.TrimSpace(base), digest)
	}
	return fmt.Sprintf("%s@%s", strings.TrimSpace(ref), digest)
}
