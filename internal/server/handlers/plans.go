package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/flowd-org/flowd/internal/configloader"
	"github.com/flowd-org/flowd/internal/engine"
	"github.com/flowd-org/flowd/internal/executor/container"
	"github.com/flowd-org/flowd/internal/indexer"
	"github.com/flowd-org/flowd/internal/policy"
	"github.com/flowd-org/flowd/internal/policy/verify"
	"github.com/flowd-org/flowd/internal/server/requestctx"
	"github.com/flowd-org/flowd/internal/server/response"
	"github.com/flowd-org/flowd/internal/server/sourcestore"
	"github.com/flowd-org/flowd/internal/types"
	"github.com/spf13/pflag"
)

// PlansConfig configures the plan handler.
type PlansConfig struct {
	Root       string
	Discover   func(string) (indexer.Result, error)
	LoadConfig func(string) (*types.Config, error)
	Sources    *sourcestore.Store
	Profile    string
	Policy     *policy.Context
	Verifier   verify.ImageVerifier
	Runtime    container.Runtime
}

// NewPlansHandler returns an HTTP handler for POST /plans.
func NewPlansHandler(cfg PlansConfig) http.Handler {
	if cfg.Root == "" {
		cfg.Root = "scripts"
	}
	discoverFn := cfg.Discover
	if discoverFn == nil {
		discoverFn = indexer.Discover
	}
	loadConfig := cfg.LoadConfig
	if loadConfig == nil {
		loadConfig = configloader.LoadConfig
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			response.Write(w, response.New(http.StatusMethodNotAllowed, "method not allowed"))
			return
		}

		ctx := r.Context()

		req, err := decodePlanRequest(r.Body)
		if err != nil {
			response.Write(w, response.New(http.StatusBadRequest, "invalid request body", response.WithDetail(err.Error())))
			return
		}
		if req.JobID == "" {
			response.Write(w, response.New(http.StatusBadRequest, "job_id is required"))
			return
		}

		discoverRoot := cfg.Root
		if discoverRoot == "" {
			discoverRoot = "scripts"
		}

		if req.Source != nil && req.Source.Name != "" {
			if cfg.Sources == nil {
				response.Write(w, response.New(http.StatusNotFound, "source not found", response.WithDetail(req.Source.Name)))
				return
			}
			source, ok := cfg.Sources.Get(req.Source.Name)
			if !ok {
				response.Write(w, response.New(http.StatusNotFound, "source not found", response.WithDetail(req.Source.Name)))
				return
			}
			if source.LocalPath == "" {
				response.Write(w, response.New(http.StatusBadRequest, "source not materialized", response.WithDetail("source "+req.Source.Name+" has no local checkout")))
				return
			}
			discoverRoot = source.LocalPath
		}

		result, err := discoverFn(discoverRoot)
		if err != nil {
			response.Write(w, response.New(http.StatusInternalServerError, "job discovery failed", response.WithDetail(err.Error())))
			return
		}

		jobMap := make(map[string]indexer.JobInfo, len(result.Jobs))
		mergeJobInfo(jobMap, result)
		lookup := newAliasLookup()
		lookup.merge(result)

		requestedID := req.JobID
		effectiveID := req.JobID
		var aliasUsed *indexer.AliasInfo

		var jobPath string
		setJobPath := func(id string) bool {
			if job, ok := jobMap[strings.ToLower(id)]; ok {
				jobPath = filepath.Dir(job.Path)
				return true
			}
			return false
		}

		resolveAlias := func() bool {
			aliasInfo, hasAlias, colliders, hasCollision, validation, hasInvalid := lookup.resolve(requestedID)
			if hasInvalid {
				response.Write(w, *aliasValidationProblem(requestedID, validation))
				return true
			}
			if hasCollision && len(colliders) > 1 {
				response.Write(w, *aliasCollisionProblem(requestedID, colliders))
				return true
			}
			if hasAlias {
				effectiveID = aliasInfo.TargetID
				temp := aliasInfo
				aliasUsed = &temp
				setJobPath(effectiveID)
			}
			return false
		}

		annotatePlan := func(plan *types.Plan) {
			plan.JobID = effectiveID
			if plan.Provenance == nil {
				plan.Provenance = map[string]any{}
			}
			plan.Provenance["canonical_id"] = effectiveID
			canonicalPath := strings.ReplaceAll(effectiveID, ".", "/")
			if aliasUsed != nil {
				plan.Provenance["invoked_path"] = requestedID
				aliasMeta := map[string]any{
					"target_path": aliasUsed.TargetPath,
				}
				if aliasUsed.Source != "" {
					aliasMeta["source"] = aliasUsed.Source
				}
				if aliasUsed.Description != "" {
					aliasMeta["description"] = aliasUsed.Description
				}
				plan.Provenance["alias"] = aliasMeta
				canonicalPath = aliasUsed.TargetPath
			}
			plan.Provenance["canonical_path"] = canonicalPath
		}

		if !setJobPath(effectiveID) {
			if resolveAlias() {
				return
			}
		}

		if jobPath == "" {
			if req.Source != nil && req.Source.Name != "" && discoverRoot != cfg.Root {
				if alt, err := discoverFn(cfg.Root); err == nil {
					mergeJobInfo(jobMap, alt)
					lookup.merge(alt)
					if aliasUsed == nil {
						if resolveAlias() {
							return
						}
					}
					if jobPath == "" {
						setJobPath(effectiveID)
					}
				}
			}
		}

		if jobPath == "" {
			if aliasUsed != nil {
				validation := indexer.AliasValidation{Code: "alias.target.invalid", Detail: fmt.Sprintf("alias %q target %q not found", requestedID, aliasUsed.TargetPath)}
				response.Write(w, *aliasValidationProblem(requestedID, validation))
				return
			}
			ociPlan, attrs, handled, prob, planErr := tryBuildOCIPlan(r, req, cfg)
			if handled {
				if planErr != nil {
					response.Write(w, response.New(http.StatusInternalServerError, "plan generation failed", response.WithDetail(planErr.Error())))
					return
				}
				if prob != nil {
					response.Write(w, *prob)
					return
				}
				if logger := requestctx.Logger(ctx); logger != nil {
					logger.Info("plan.generated", attrs...)
				}
				writePlanResponse(w, ociPlan)
				return
			}
			response.Write(w, response.New(http.StatusNotFound, "job not found", response.WithDetail(requestedID)))
			return
		}

		cfgObj, err := loadConfig(jobPath)
		if err != nil {
			response.Write(w, response.New(http.StatusInternalServerError, "load config failed", response.WithDetail(err.Error())))
			return
		}
		isDAG := isDAGConfig(cfgObj)
		if isDAG {
			if prob := validateDAGConfig(cfgObj); prob != nil {
				response.Write(w, *prob)
				return
			}
		}

		spec := cfgObj.ArgSpec
		var binding *engine.Binding
		if spec != nil && len(spec.Args) > 0 {
			bind, bindErr := validatePlanArgs(*spec, req.Args)
			if bindErr != nil {
				var argErr *engine.ArgError
				if errors.As(bindErr, &argErr) {
					response.Write(w, response.New(http.StatusUnprocessableEntity, "argument validation failed",
						response.WithExtension("errors", []map[string]string{{"arg": argErr.Arg, "message": argErr.Msg}})))
					return
				}
				response.Write(w, response.New(http.StatusBadRequest, "invalid arguments", response.WithDetail(bindErr.Error())))
				return
			}
			binding = bind
		} else if len(req.Args) > 0 {
			response.Write(w, response.New(http.StatusBadRequest, "job does not accept arguments"))
			return
		}

		effProfile, err := resolveEffectiveProfile(req.RequestedSecurityProfile, cfg.Profile)
		if err != nil {
			response.Write(w, response.New(http.StatusUnprocessableEntity, "invalid security profile",
				response.WithExtension("code", "E_POLICY"),
				response.WithDetail(err.Error())))
			return
		}

		policyCtx := cfg.Policy
		if policyCtx == nil {
			policyCtx, _ = policy.NewContext(nil)
		}

		runtimeVal := cfg.Runtime
		runtimeStr := string(runtimeVal)
		ctx = requestctx.WithEffectiveProfile(ctx, effProfile)

		if isDAG {
			executor := strings.ToLower(strings.TrimSpace(cfgObj.Executor))
			if executor == "container" && runtimeVal == "" {
				detected, detectErr := detectContainerRuntime(nil)
				if detectErr != nil {
					response.Write(w, runtimeUnavailableProblem(detectErr))
					return
				}
				runtimeVal = detected
				runtimeStr = string(detected)
			}
			if runtimeStr != "" {
				ctx = requestctx.WithRuntime(ctx, runtimeStr)
			}
			r = r.WithContext(ctx)

			plan, attrs, prob, buildErr := buildDAGPlan(ctx, effectiveID, cfgObj, spec, binding, effProfile, policyCtx, cfg.Verifier, runtimeStr)
			if buildErr != nil {
				response.Write(w, response.New(http.StatusInternalServerError, "plan generation failed", response.WithDetail(buildErr.Error())))
				return
			}
			annotatePlan(&plan)
			if prob != nil {
				response.Write(w, *prob)
				return
			}
			if logger := requestctx.Logger(ctx); logger != nil {
				attrs = append(attrs,
					slog.String("job_id", effectiveID),
					slog.String("security_profile", effProfile),
				)
				if aliasUsed != nil {
					attrs = append(attrs, slog.String("invoked_path", requestedID))
				}
				logger.Info("plan.generated", attrs...)
			}
			writePlanResponse(w, plan)
			return
		}

		if runtimeStr != "" {
			ctx = requestctx.WithRuntime(ctx, runtimeStr)
		}
		r = r.WithContext(ctx)

		findings := []types.Finding{}
		var trustPreview *types.ImageTrustPreview

		image := containerImageFromConfig(cfgObj)
		if image != "" {
			if runtimeVal == "" {
				if _, detectErr := detectContainerRuntime(nil); detectErr != nil {
					response.Write(w, runtimeUnavailableProblem(detectErr))
					return
				}
			}
			if prob := enforceRegistryAllowList(ctx, image, policyCtx); prob != nil {
				response.Write(w, *prob)
				return
			}

			mode, err := policyCtx.VerifyModeForProfile(effProfile)
			if err != nil {
				response.Write(w, response.New(http.StatusUnprocessableEntity, "policy error",
					response.WithExtension("code", "E_POLICY"),
					response.WithDetail(err.Error())))
				return
			}

			outcome, prob := enforceImageVerification(ctx, image, mode, cfg.Verifier)
			if prob != nil {
				response.Write(w, *prob)
				return
			}
			if mode != policy.VerifyModeDisabled {
				trustPreview = &types.ImageTrustPreview{
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
				findings = append(findings, types.Finding{
					Code:    "image.signature.permissive",
					Level:   "warning",
					Message: reason,
				})
			}

			if prob := enforceResourceCeilings(ctx, cfgObj, policyCtx.ContainerCeilings()); prob != nil {
				response.Write(w, *prob)
				return
			}
		}

		overrideFindings, _, prob := evaluateOverrides(ctx, cfgObj, effProfile, policyCtx)
		if prob != nil {
			response.Write(w, *prob)
			return
		}
		if len(overrideFindings) > 0 {
			findings = append(findings, overrideFindings...)
		}

		plan := engine.BuildPlan(effectiveID, cfgObj, spec, binding)
		annotatePlan(&plan)
		plan.SecurityProfile = effProfile
		if len(findings) > 0 {
			plan.PolicyFindings = findings
		}
		if trustPreview != nil {
			plan.ImageTrust = trustPreview
		}
		if logger := requestctx.Logger(ctx); logger != nil {
			attrs := []any{
				slog.String("job_id", effectiveID),
				slog.String("security_profile", effProfile),
			}
			if aliasUsed != nil {
				attrs = append(attrs, slog.String("invoked_path", requestedID))
			}
			if runtimeStr != "" {
				attrs = append(attrs, slog.String("runtime", runtimeStr))
			}
			if image != "" {
				attrs = append(attrs, slog.String("image", image))
			}
			logger.Info("plan.generated", attrs...)
		}

		writePlanResponse(w, plan)
	})
}

func writePlanResponse(w http.ResponseWriter, plan types.Plan) {
	data, err := json.Marshal(plan)
	if err != nil {
		response.Write(w, response.New(http.StatusInternalServerError, "encode plan failed", response.WithDetail(err.Error())))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

type planRequest struct {
	JobID                    string                 `json:"job_id"`
	Args                     map[string]interface{} `json:"args"`
	Source                   *RunSourceRef          `json:"source"`
	RequestedSecurityProfile string                 `json:"requested_security_profile"`
}

func decodePlanRequest(body io.ReadCloser) (planRequest, error) {
	defer body.Close()
	var req planRequest
	dec := json.NewDecoder(body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		return req, err
	}
	if req.Args == nil {
		req.Args = map[string]interface{}{}
	}
	return req, nil
}

// resolveEffectiveProfile is defined in runs.go for the handlers package.

func validatePlanArgs(spec types.ArgSpec, args map[string]interface{}) (*engine.Binding, error) {
	fs := pflag.NewFlagSet("plans", pflag.ContinueOnError)
	fs.SetOutput(io.Discard)
	if err := attachSpecFlags(fs, spec); err != nil {
		return nil, err
	}

	for name := range args {
		if !hasArg(spec, name) {
			return nil, errors.New("unknown argument: " + name)
		}
	}

	for _, arg := range spec.Args {
		val, ok := args[arg.Name]
		if !ok {
			continue
		}
		if err := setFlagFromValue(fs, arg, val); err != nil {
			return nil, err
		}
	}

	return engine.ValidateAndBind(fs, spec)
}

func attachSpecFlags(fs *pflag.FlagSet, spec types.ArgSpec) error {
	for _, a := range spec.Args {
		switch a.Type {
		case "string":
			def, _ := a.Default.(string)
			fs.String(a.Name, def, "")
		case "boolean":
			def, _ := a.Default.(bool)
			fs.Bool(a.Name, def, "")
		case "integer":
			var defInt int
			switch v := a.Default.(type) {
			case int:
				defInt = v
			case int64:
				defInt = int(v)
			case float64:
				defInt = int(v)
			}
			fs.Int(a.Name, defInt, "")
		case "array", "object":
			fs.StringArray(a.Name, nil, "")
		default:
			return errors.New("unsupported arg type: " + a.Type)
		}
	}
	return nil
}

func setFlagFromValue(fs *pflag.FlagSet, arg types.Arg, val interface{}) error {
	switch arg.Type {
	case "string":
		s, ok := val.(string)
		if !ok {
			return errors.New("argument " + arg.Name + " must be a string")
		}
		return fs.Set(arg.Name, s)
	case "boolean":
		b, ok := val.(bool)
		if !ok {
			return errors.New("argument " + arg.Name + " must be a boolean")
		}
		return fs.Set(arg.Name, strconv.FormatBool(b))
	case "integer":
		switch v := val.(type) {
		case float64:
			return fs.Set(arg.Name, strconv.Itoa(int(v)))
		case int:
			return fs.Set(arg.Name, strconv.Itoa(v))
		case int64:
			return fs.Set(arg.Name, strconv.Itoa(int(v)))
		default:
			return errors.New("argument " + arg.Name + " must be an integer")
		}
	case "array":
		switch arr := val.(type) {
		case []interface{}:
			for _, item := range arr {
				s, ok := item.(string)
				if !ok {
					return errors.New("argument " + arg.Name + " array items must be strings")
				}
				if err := fs.Set(arg.Name, s); err != nil {
					return err
				}
			}
			return nil
		case []string:
			for _, item := range arr {
				if err := fs.Set(arg.Name, item); err != nil {
					return err
				}
			}
			return nil
		default:
			return errors.New("argument " + arg.Name + " must be an array of strings")
		}
	case "object":
		mp, ok := val.(map[string]interface{})
		if !ok {
			return errors.New("argument " + arg.Name + " must be an object")
		}
		for k, v := range mp {
			str, ok := v.(string)
			if !ok {
				return errors.New("argument " + arg.Name + " values must be strings")
			}
			if err := fs.Set(arg.Name, k+"="+str); err != nil {
				return err
			}
		}
		return nil
	default:
		return errors.New("unsupported arg type: " + arg.Type)
	}
}

func hasArg(spec types.ArgSpec, name string) bool {
	for _, arg := range spec.Args {
		if arg.Name == name {
			return true
		}
	}
	return false
}
