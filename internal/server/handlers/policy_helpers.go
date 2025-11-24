package handlers

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/flowd-org/flowd/internal/policy"
	"github.com/flowd-org/flowd/internal/policy/verify"
	"github.com/flowd-org/flowd/internal/server/metrics"
	"github.com/flowd-org/flowd/internal/server/requestctx"
	"github.com/flowd-org/flowd/internal/server/response"
	"github.com/flowd-org/flowd/internal/types"
)

type verificationOutcome struct {
	Mode     policy.VerifyMode
	Verified bool
	Reason   string
}

type policyDecision struct {
	Subject  string
	Decision string
	Code     string
	Reason   string
}

func containerImageFromConfig(cfg *types.Config) string {
	if cfg == nil {
		return ""
	}
	if cfg.Container != nil && strings.TrimSpace(cfg.Container.Image) != "" {
		return strings.TrimSpace(cfg.Container.Image)
	}
	if strings.HasPrefix(cfg.Interpreter, "container:") {
		return strings.TrimSpace(strings.TrimPrefix(cfg.Interpreter, "container:"))
	}
	return ""
}

func enforceRegistryAllowList(ctx context.Context, image string, policyCtx *policy.Context) *response.Problem {
	if policyCtx == nil {
		return nil
	}
	allowed := policyCtx.AllowedRegistries()
	if len(allowed) == 0 {
		return nil
	}
	registry, err := policy.RegistryFromImage(image)
	if err != nil {
		prob := response.New(http.StatusUnprocessableEntity, "invalid container image",
			response.WithExtension("code", "E_IMAGE_POLICY"),
			response.WithDetail(err.Error()))
		requestctx.LogPolicyDecision(ctx, "container.image", "denied", "E_IMAGE_POLICY", err.Error())
		metrics.Default.RecordPolicyDenial("E_IMAGE_POLICY")
		return &prob
	}
	if !policy.RegistryAllowed(registry, allowed) {
		detail := fmt.Sprintf("registry %s not allowed", registry)
		prob := response.New(http.StatusUnprocessableEntity, "image registry not allowed",
			response.WithExtension("code", "image.registry.not.allowed"),
			response.WithDetail(detail))
		requestctx.LogPolicyDecision(ctx, "container.image", "denied", "image.registry.not.allowed", detail)
		metrics.Default.RecordPolicyDenial("image.registry.not.allowed")
		return &prob
	}
	return nil
}

func enforceImageVerification(ctx context.Context, image string, mode policy.VerifyMode, verifier verify.ImageVerifier) (verificationOutcome, *response.Problem) {
	out := verificationOutcome{Mode: mode, Verified: true}
	if mode == policy.VerifyModeDisabled || verifier == nil {
		return out, nil
	}
	res, err := verifier.Verify(ctx, image)
	if err != nil {
		out.Verified = false
		out.Reason = err.Error()
	} else {
		out.Verified = res.Verified
		out.Reason = res.Reason
	}
	if out.Verified {
		return out, nil
	}
	if mode == policy.VerifyModeRequired {
		detail := out.Reason
		if detail == "" {
			detail = "signature verification failed"
		}
		prob := response.New(http.StatusUnprocessableEntity, "image signature required",
			response.WithExtension("code", "image.signature.required"),
			response.WithDetail(detail))
		requestctx.LogPolicyDecision(ctx, "container.image", "denied", "image.signature.required", detail)
		metrics.Default.RecordPolicyDenial("image.signature.required")
		return out, &prob
	}
	reason := out.Reason
	if reason == "" {
		reason = "signature verification failed"
	}
	requestctx.LogPolicyDecision(ctx, "container.image", "warn", "image.signature.permissive", reason)
	return out, nil
}

func enforceResourceCeilings(ctx context.Context, cfg *types.Config, limits *policy.ContainerLimits) *response.Problem {
	if limits == nil || cfg == nil || cfg.Container == nil || cfg.Container.Resources == nil {
		return nil
	}
	resources := cfg.Container.Resources
	if limits.CPUMillicores != nil && strings.TrimSpace(resources.CPU) != "" {
		cpuVal, err := policy.ParseCPUMillicores(resources.CPU)
		if err != nil {
			prob := response.New(http.StatusUnprocessableEntity, "invalid container cpu request",
				response.WithExtension("code", "E_IMAGE_POLICY"),
				response.WithDetail(err.Error()))
			requestctx.LogPolicyDecision(ctx, "container.resources", "denied", "E_IMAGE_POLICY", err.Error())
			metrics.Default.RecordPolicyDenial("E_IMAGE_POLICY")
			return &prob
		}
		if cpuVal > *limits.CPUMillicores {
			detail := fmt.Sprintf("requested cpu %dm exceeds ceiling %dm", cpuVal, *limits.CPUMillicores)
			prob := response.New(http.StatusUnprocessableEntity, "container cpu exceeds policy ceiling",
				response.WithExtension("code", "E_IMAGE_POLICY"),
				response.WithDetail(detail))
			requestctx.LogPolicyDecision(ctx, "container.resources", "denied", "E_IMAGE_POLICY", detail)
			metrics.Default.RecordPolicyDenial("E_IMAGE_POLICY")
			return &prob
		}
	}
	if limits.MemoryBytes != nil && strings.TrimSpace(resources.Memory) != "" {
		memVal, err := policy.ParseMemoryBytes(resources.Memory)
		if err != nil {
			prob := response.New(http.StatusUnprocessableEntity, "invalid container memory request",
				response.WithExtension("code", "E_IMAGE_POLICY"),
				response.WithDetail(err.Error()))
			requestctx.LogPolicyDecision(ctx, "container.resources", "denied", "E_IMAGE_POLICY", err.Error())
			metrics.Default.RecordPolicyDenial("E_IMAGE_POLICY")
			return &prob
		}
		if memVal > *limits.MemoryBytes {
			detail := fmt.Sprintf("requested memory %s exceeds ceiling %s", formatMemory(memVal), formatMemory(*limits.MemoryBytes))
			prob := response.New(http.StatusUnprocessableEntity, "container memory exceeds policy ceiling",
				response.WithExtension("code", "E_IMAGE_POLICY"),
				response.WithDetail(detail))
			requestctx.LogPolicyDecision(ctx, "container.resources", "denied", "E_IMAGE_POLICY", detail)
			metrics.Default.RecordPolicyDenial("E_IMAGE_POLICY")
			return &prob
		}
	}
	return nil
}

func formatMemory(bytes int64) string {
	const (
		mi = 1024 * 1024
	)
	if bytes%mi == 0 {
		return fmt.Sprintf("%dMi", bytes/mi)
	}
	return fmt.Sprintf("%d bytes", bytes)
}

func evaluateOverrides(ctx context.Context, cfg *types.Config, profile string, policyCtx *policy.Context) ([]types.Finding, []policyDecision, *response.Problem) {
	if cfg == nil {
		return nil, nil, nil
	}
	profile = strings.ToLower(strings.TrimSpace(profile))
	containerCfg := cfg.Container
	var findings []types.Finding
	var decisions []policyDecision
	policyOverrides := (*policy.Overrides)(nil)
	if policyCtx != nil {
		policyOverrides = policyCtx.Overrides()
	}

	recordDecision := func(subject, decision, code, reason string) {
		requestctx.LogPolicyDecision(ctx, subject, decision, code, reason)
		if decision == "denied" && code != "" {
			metrics.Default.RecordPolicyDenial(code)
		}
		decisions = append(decisions, policyDecision{Subject: subject, Decision: decision, Code: code, Reason: reason})
	}

	checkDenied := func(subject, reason string) *response.Problem {
		recordDecision(subject, "denied", "policy.denied", reason)
		prob := response.New(http.StatusUnprocessableEntity, "policy override denied",
			response.WithExtension("code", "policy.denied"),
			response.WithDetail(reason))
		return &prob
	}

	addFinding := func(message string, level string) {
		if level == "" {
			level = "info"
		}
		findings = append(findings, types.Finding{
			Code:    "policy.override.allowed",
			Level:   level,
			Message: message,
		})
	}

	allowDecision := func(subject, reason string, level string) {
		recordDecision(subject, "allowed", "policy.override.allowed", reason)
		addFinding(reason, level)
	}

	if containerCfg != nil {
		if network := strings.ToLower(strings.TrimSpace(containerCfg.Network)); network != "" && network != "none" {
			switch profile {
			case "secure":
				return findings, decisions, checkDenied("container.network", fmt.Sprintf("network override %q not permitted in secure profile", network))
			case "permissive":
				allowed := false
				if policyOverrides != nil {
					for _, n := range policyOverrides.Network {
						if strings.EqualFold(n, network) {
							allowed = true
							break
						}
					}
				}
				if !allowed {
					return findings, decisions, checkDenied("container.network", fmt.Sprintf("network override %q not allowed by policy", network))
				}
				allowDecision("container.network", fmt.Sprintf("network override %q allowed by policy", network), "info")
			case "disabled":
				allowDecision("container.network", fmt.Sprintf("network override %q allowed (profile disabled)", network), "warning")
			}
		}
		if containerCfg.RootfsWritable {
			switch profile {
			case "secure":
				return findings, decisions, checkDenied("container.rootfs", "writable rootfs not permitted in secure profile")
			case "permissive":
				if policyOverrides == nil || policyOverrides.RootfsWritable == nil || !*policyOverrides.RootfsWritable {
					return findings, decisions, checkDenied("container.rootfs", "writable rootfs not allowed by policy")
				}
				allowDecision("container.rootfs", "writable rootfs allowed by policy", "info")
			case "disabled":
				allowDecision("container.rootfs", "writable rootfs allowed (profile disabled)", "warning")
			}
		}
		if len(containerCfg.Capabilities) > 0 {
			caps := make([]string, len(containerCfg.Capabilities))
			for i, c := range containerCfg.Capabilities {
				caps[i] = strings.ToUpper(strings.TrimSpace(c))
			}
			switch profile {
			case "secure":
				return findings, decisions, checkDenied("container.capabilities", "adding capabilities not permitted in secure profile")
			case "permissive":
				allowedCaps := map[string]struct{}{}
				if policyOverrides != nil {
					for _, c := range policyOverrides.Caps {
						allowedCaps[strings.ToUpper(strings.TrimSpace(c))] = struct{}{}
					}
				}
				if len(allowedCaps) == 0 {
					return findings, decisions, checkDenied("container.capabilities", "policy does not allow adding capabilities")
				}
				for _, c := range caps {
					if _, ok := allowedCaps[c]; !ok {
						return findings, decisions, checkDenied("container.capabilities", fmt.Sprintf("capability %q not allowed by policy", c))
					}
				}
				allowDecision("container.capabilities", fmt.Sprintf("capabilities %v allowed by policy", caps), "info")
			case "disabled":
				allowDecision("container.capabilities", fmt.Sprintf("capabilities %v allowed (profile disabled)", caps), "warning")
			}
		}
	}

	if cfg.EnvInheritance {
		switch profile {
		case "secure":
			return findings, decisions, checkDenied("env.inheritance", "environment inheritance not permitted in secure profile")
		case "permissive":
			if policyOverrides == nil || policyOverrides.EnvInheritance == nil || !*policyOverrides.EnvInheritance {
				return findings, decisions, checkDenied("env.inheritance", "environment inheritance not allowed by policy")
			}
			allowDecision("env.inheritance", "environment inheritance allowed by policy", "info")
		case "disabled":
			allowDecision("env.inheritance", "environment inheritance allowed (profile disabled)", "warning")
		}
	}

	return findings, decisions, nil
}
