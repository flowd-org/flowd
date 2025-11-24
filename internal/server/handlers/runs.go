package handlers

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/flowd-org/flowd/internal/configloader"
	"github.com/flowd-org/flowd/internal/coredb"
	"github.com/flowd-org/flowd/internal/engine"
	"github.com/flowd-org/flowd/internal/events"
	"github.com/flowd-org/flowd/internal/executor"
	"github.com/flowd-org/flowd/internal/executor/container"
	"github.com/flowd-org/flowd/internal/indexer"
	"github.com/flowd-org/flowd/internal/paths"
	"github.com/flowd-org/flowd/internal/policy"
	"github.com/flowd-org/flowd/internal/policy/verify"
	"github.com/flowd-org/flowd/internal/server/requestctx"
	"github.com/flowd-org/flowd/internal/server/response"
	"github.com/flowd-org/flowd/internal/server/runstore"
	"github.com/flowd-org/flowd/internal/server/sourcestore"
	"github.com/flowd-org/flowd/internal/server/sse"
	"github.com/flowd-org/flowd/internal/types"
)

const (
	defaultRunStatus          = "queued"
	defaultIdempotencyTTL     = 10 * time.Minute
	defaultRunsPage           = 1
	defaultRunsPerPage        = 50
	maxRunsPerPage            = 200
	storageQuotaProblemType   = "https://flowd.dev/problems/storage-quota-exceeded"
	storageQuotaProblemDetail = "Core storage quota exceeded; free up space or increase the configured quota before retrying."
)

var (
	idempotencyKeyPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{20,128}$`)
	sha256Pattern         = regexp.MustCompile(`^[a-f0-9]{64}$`)
)

func scopedIdempotencyKey(principal, key string) string {
	if principal == "" {
		return key
	}
	sum := sha256.Sum256([]byte(principal))
	return hex.EncodeToString(sum[:]) + ":" + key
}

var detectContainerRuntime = container.DetectRuntime

// RunsConfig configures the run handler.
type RunsConfig struct {
	Root           string
	Discover       func(string) (indexer.Result, error)
	LoadConfig     func(string) (*types.Config, error)
	Now            func() time.Time
	IdempotencyTTL time.Duration
	Store          *runstore.Store
	Events         EventSink
	ResolveSource  func(jobID string, ref *RunSourceRef) (map[string]any, bool)
	Sources        *sourcestore.Store
	Profile        string
	Policy         *policy.Context
	Verifier       verify.ImageVerifier
	Runtime        container.Runtime
	DB             *coredb.DB
}

type RunsHandler struct {
	root           string
	discover       func(string) (indexer.Result, error)
	loadConfig     func(string) (*types.Config, error)
	now            func() time.Time
	idempotency    idempotencyStore
	idempotencyTTL time.Duration
	store          *runstore.Store
	events         EventSink
	resolveSrc     func(jobID string, ref *RunSourceRef) (map[string]any, bool)
	sources        *sourcestore.Store
	profile        string
	policy         *policy.Context
	verifier       verify.ImageVerifier
	runtime        container.Runtime
	running        sync.Map // runID -> *runExecutionContext
}

// NewRunsHandler returns an HTTP handler for POST /runs.
func NewRunsHandler(cfg RunsConfig) *RunsHandler {
	root := cfg.Root
	if root == "" {
		root = "scripts"
	}
	discoverFn := cfg.Discover
	if discoverFn == nil {
		discoverFn = indexer.Discover
	}
	loadCfg := cfg.LoadConfig
	if loadCfg == nil {
		loadCfg = configloader.LoadConfig
	}
	nowFn := cfg.Now
	if nowFn == nil {
		nowFn = func() time.Time { return time.Now().UTC() }
	}
	ttl := cfg.IdempotencyTTL
	if ttl <= 0 {
		ttl = defaultIdempotencyTTL
	}

	store := cfg.Store
	if store == nil {
		store = runstore.New()
	}

	var idemStore idempotencyStore
	if cfg.DB != nil {
		idemStore = newDBIdempotencyStore(cfg.DB)
	} else {
		idemStore = newMemoryIdempotencyCache(ttl)
	}

	return &RunsHandler{
		root:           root,
		discover:       discoverFn,
		loadConfig:     loadCfg,
		now:            nowFn,
		idempotency:    idemStore,
		idempotencyTTL: ttl,
		store:          store,
		events:         cfg.Events,
		resolveSrc:     cfg.ResolveSource,
		sources:        cfg.Sources,
		profile:        cfg.Profile,
		policy:         cfg.Policy,
		verifier:       cfg.Verifier,
		runtime:        cfg.Runtime,
	}
}

func (h *RunsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodPost:
		h.handleCreate(w, r)
	case http.MethodGet:
		h.handleList(w, r)
	default:
		response.Write(w, response.New(http.StatusMethodNotAllowed, "method not allowed"))
	}
}

func (h *RunsHandler) handleCreate(w http.ResponseWriter, r *http.Request) {
	req, rawBody, err := decodeRunRequest(r.Body)
	if err != nil {
		response.Write(w, response.New(http.StatusBadRequest, "invalid request body", response.WithDetail(err.Error())))
		return
	}
	if req.JobID == "" {
		response.Write(w, response.New(http.StatusBadRequest, "job_id is required"))
		return
	}

	canonicalBody, err := canonicalizeJSON(rawBody)
	if err != nil {
		response.Write(w, response.New(http.StatusBadRequest, "invalid request body", response.WithDetail(err.Error())))
		return
	}
	bodyHash := sha256.Sum256(canonicalBody)
	bodyHashHex := hex.EncodeToString(bodyHash[:])

	headerHash := strings.TrimSpace(r.Header.Get("Idempotency-SHA256"))
	if headerHash != "" {
		if !sha256Pattern.MatchString(strings.ToLower(headerHash)) {
			response.Write(w, response.New(http.StatusBadRequest, "invalid Idempotency-SHA256 header"))
			return
		}
		if !strings.EqualFold(headerHash, bodyHashHex) {
			response.Write(w, response.New(http.StatusConflict, "idempotency hash mismatch",
				response.WithType("https://flowd.dev/problems/idempotency-key-conflict"),
				response.WithDetail("request hash does not match stored hash"),
				response.WithExtension("incoming_sha256", strings.ToLower(headerHash)),
				response.WithExtension("computed_sha256", bodyHashHex),
			))
			return
		}
	}

	ctx := r.Context()
	logger := requestctx.Logger(ctx)
	principal, _ := requestctx.Principal(ctx)
	idemKey := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if idemKey == "" {
		response.Write(w, response.New(http.StatusBadRequest, "Idempotency-Key header required"))
		return
	}
	if !idempotencyKeyPattern.MatchString(idemKey) {
		response.Write(w, response.New(http.StatusBadRequest, "invalid Idempotency-Key header"))
		return
	}
	scopedKey := scopedIdempotencyKey(principal, idemKey)
	endpoint := r.Method + " " + r.URL.Path
	now := h.now()
	if h.idempotency != nil {
		cached, status, storedHash, found, err := h.idempotency.Lookup(ctx, scopedKey, endpoint, now)
		if err != nil {
			response.Write(w, response.New(http.StatusInternalServerError, "idempotency lookup failed", response.WithDetail(err.Error())))
			return
		}
		if found {
			if storedHash != bodyHashHex {
				response.Write(w, response.New(http.StatusConflict, "idempotency key conflict",
					response.WithType("https://flowd.dev/problems/idempotency-key-conflict"),
					response.WithExtension("stored_sha256", storedHash),
					response.WithExtension("incoming_sha256", bodyHashHex),
				))
				return
			}
			w.Header().Set("Idempotent-Replay", "true")
			writeRunPayload(w, cached, status)
			return
		}
	}

	runRoot := h.root
	if runRoot == "" {
		runRoot = "scripts"
	}

	if req.Source != nil && req.Source.Name != "" {
		if h.sources != nil {
			src, ok := h.sources.Get(req.Source.Name)
			if !ok {
				response.Write(w, response.New(http.StatusNotFound, "source not found", response.WithDetail(req.Source.Name)))
				return
			}
			if src.LocalPath == "" {
				response.Write(w, response.New(http.StatusBadRequest, "source not materialized", response.WithDetail("source "+req.Source.Name+" has no local checkout")))
				return
			}
			runRoot = src.LocalPath
		}
	}

	result, err := h.discover(runRoot)
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

	var scriptDir string
	setScriptDir := func(id string) bool {
		if job, ok := jobMap[strings.ToLower(id)]; ok {
			scriptDir = filepath.Dir(job.Path)
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
			setScriptDir(effectiveID)
		}
		return false
	}

	if !setScriptDir(effectiveID) {
		if resolveAlias() {
			return
		}
	}

	if scriptDir == "" {
		if req.Source != nil && req.Source.Name != "" && runRoot != h.root {
			if alt, err := h.discover(h.root); err == nil {
				mergeJobInfo(jobMap, alt)
				lookup.merge(alt)
				if aliasUsed == nil {
					if resolveAlias() {
						return
					}
				}
				if scriptDir == "" {
					setScriptDir(effectiveID)
				}
			}
		}
	}

	if scriptDir == "" {
		if aliasUsed != nil {
			validation := indexer.AliasValidation{Code: "alias.target.invalid", Detail: fmt.Sprintf("alias %q target %q not found", requestedID, aliasUsed.TargetPath)}
			response.Write(w, *aliasValidationProblem(requestedID, validation))
			return
		}
		if prob := h.ociRunUnsupported(requestedID); prob != nil {
			response.Write(w, *prob)
			return
		}
		response.Write(w, response.New(http.StatusNotFound, "job not found", response.WithDetail(requestedID)))
		return
	}

	absScriptDir, err := filepath.Abs(scriptDir)
	if err != nil {
		response.Write(w, response.New(http.StatusInternalServerError, "resolve script directory", response.WithDetail(err.Error())))
		return
	}
	absScriptsRoot, err := filepath.Abs(runRoot)
	if err != nil {
		response.Write(w, response.New(http.StatusInternalServerError, "resolve scripts root", response.WithDetail(err.Error())))
		return
	}
	relJobPath, relErr := filepath.Rel(absScriptsRoot, absScriptDir)
	var execScriptDir string
	if relErr == nil {
		execScriptDir = filepath.Join(runRoot, relJobPath)
	} else {
		execScriptDir = absScriptDir
	}

	cfg, err := h.loadConfig(absScriptDir)
	if err != nil {
		response.Write(w, response.New(http.StatusInternalServerError, "load config failed", response.WithDetail(err.Error())))
		return
	}

	spec := cfg.ArgSpec
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

	executorMode := strings.ToLower(cfg.Executor)
	if strings.HasPrefix(cfg.Interpreter, "container:") && executorMode == "" {
		executorMode = "container"
	}
	if executorMode == "" {
		executorMode = "shell"
	}

	var runtime container.Runtime
	if executorMode == "container" {
		if h.runtime != "" {
			runtime = h.runtime
		} else {
			detected, detectErr := detectContainerRuntime(nil)
			if detectErr != nil {
				response.Write(w, runtimeUnavailableProblem(detectErr))
				return
			}
			runtime = detected
		}
	}
	runtimeStr := string(runtime)

	provenance := h.resolveProvenance(effectiveID, req.Source, execScriptDir, absScriptDir)
	if provenance == nil {
		provenance = map[string]any{}
	}
	provenance["canonical_id"] = effectiveID
	canonicalPath := strings.ReplaceAll(effectiveID, ".", "/")
	if aliasUsed != nil {
		canonicalPath = aliasUsed.TargetPath
		aliasMeta := map[string]any{
			"target_path": aliasUsed.TargetPath,
		}
		if aliasUsed.Source != "" {
			aliasMeta["source"] = aliasUsed.Source
		}
		if aliasUsed.Description != "" {
			aliasMeta["description"] = aliasUsed.Description
		}
		provenance["alias"] = aliasMeta
		provenance["invoked_path"] = requestedID
	}
	provenance["canonical_path"] = canonicalPath

	effProfile, err := resolveEffectiveProfile(req.RequestedSecurityProfile, h.profile)
	if err != nil {
		response.Write(w, response.New(http.StatusUnprocessableEntity, "invalid security profile",
			response.WithExtension("code", "E_POLICY"),
			response.WithDetail(err.Error())))
		return
	}

	policyCtx := h.policy
	if policyCtx == nil {
		policyCtx, _ = policy.NewContext(nil)
	}

	var findings []types.Finding
	var trustPreview *types.ImageTrustPreview
	ctx = requestctx.WithEffectiveProfile(r.Context(), effProfile)
	if runtimeStr != "" {
		ctx = requestctx.WithRuntime(ctx, runtimeStr)
	}
	r = r.WithContext(ctx)
	logger = requestctx.Logger(ctx)
	image := containerImageFromConfig(cfg)
	if image != "" {
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
		outcome, prob := enforceImageVerification(ctx, image, mode, h.verifier)
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
		if prob := enforceResourceCeilings(ctx, cfg, policyCtx.ContainerCeilings()); prob != nil {
			response.Write(w, *prob)
			return
		}
	}
	overrideFindings, decisions, prob := evaluateOverrides(ctx, cfg, effProfile, policyCtx)
	if prob != nil {
		if len(decisions) > 0 {
			tempPayload := &RunPayload{
				JobID:           effectiveID,
				SecurityProfile: effProfile,
				Executor:        executorMode,
				Provenance:      provenance,
			}
			publishPolicyDecisions(h.events, tempPayload, decisions)
		}
		response.Write(w, *prob)
		return
	}
	if len(overrideFindings) > 0 {
		findings = append(findings, overrideFindings...)
	}

	plan := engine.BuildPlan(effectiveID, cfg, spec, binding)
	plan.SecurityProfile = effProfile
	if len(findings) > 0 {
		plan.PolicyFindings = findings
	}
	if trustPreview != nil {
		plan.ImageTrust = trustPreview
	}
	runID := events.GenerateRunID()
	if executorMode == "container" && runtime != "" {
		if err := container.RemoveContainer(context.Background(), runtime, runID); err != nil {
			response.Write(w, containerNameConflictProblem(err))
			return
		}
	}
	resp := newRunPayload(runID, effectiveID, defaultRunStatus, now)
	resp.Executor = executorMode
	resp.SecurityProfile = effProfile
	if runtime != "" {
		resp.Runtime = string(runtime)
	}
	if len(plan.ResolvedArgs) > 0 {
		resp.Result = map[string]any{
			"resolved_args": plan.ResolvedArgs,
		}
	}
	resp.Provenance = provenance

	if h.idempotency != nil {
		expiresAt := now.Add(h.idempotencyTTL)
		if err := h.idempotency.Store(ctx, scopedKey, endpoint, bodyHashHex, resp, http.StatusCreated, expiresAt); err != nil {
			if logger != nil {
				logger.Error("idempotency store failed", slog.String("error", err.Error()))
			}
			if coredb.IsQuotaExceeded(err) {
				response.Write(w, storageQuotaExceededProblem())
			} else {
				response.Write(w, response.New(http.StatusInternalServerError, "idempotency store failed", response.WithDetail(err.Error())))
			}
			return
		}
	}

	h.store.Create(runstore.Run{
		ID:         resp.ID,
		JobID:      resp.JobID,
		Status:     resp.Status,
		StartedAt:  resp.StartedAt,
		Result:     resp.Result,
		Executor:   resp.Executor,
		Runtime:    resp.Runtime,
		Provenance: resp.Provenance,
	})

	if len(decisions) > 0 {
		publishPolicyDecisions(h.events, &resp, decisions)
	}
	runCtx := &runExecutionContext{
		ctx:        nil,
		cancel:     nil,
		runPayload: resp,
		scriptDir:  execScriptDir,
		config:     cfg,
		spec:       spec,
		binding:    binding,
		plan:       plan,
		executor:   executorMode,
		runtime:    runtime,
	}
	ctxWithCancel, cancel := context.WithCancel(context.Background())
	runCtx.ctx = ctxWithCancel
	runCtx.cancel = cancel
	h.running.Store(runID, runCtx)
	writeRunPayload(w, resp, http.StatusCreated)
	if logger != nil {
		attrs := []any{
			slog.String("run_id", runID),
			slog.String("job_id", effectiveID),
			slog.String("status", resp.Status),
			slog.String("executor", executorMode),
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
		logger.Info("run.accepted", attrs...)
	}
	go h.executeRun(runCtx)
}

func (h *RunsHandler) ociRunUnsupported(jobID string) *response.Problem {
	if h.sources == nil || strings.TrimSpace(jobID) == "" {
		return nil
	}
	for _, src := range h.sources.List() {
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
			detail := fmt.Sprintf("OCI add-on job %s from source %s cannot be executed in this phase.", jobID, src.Name)
			options := []response.Option{
				response.WithExtension("code", "E_OCI_RUN_UNSUPPORTED"),
				response.WithDetail(detail),
				response.WithExtension("source", sourceToProvenance(src)),
			}
			if summary := manifestSummary(manifest); len(summary) > 0 {
				options = append(options, response.WithExtension("manifest", summary))
			}
			prob := response.New(http.StatusNotImplemented, "oci add-on runs not yet supported", options...)
			return &prob
		}
	}
	return nil
}

func resolveEffectiveProfile(requested, cfgProfile string) (string, error) {
	if requested != "" {
		if prof, ok := normalizeProfile(requested); ok {
			return prof, nil
		}
		return "", fmt.Errorf("invalid requested security profile %q", requested)
	}
	if env := os.Getenv("FLWD_PROFILE"); env != "" {
		if prof, ok := normalizeProfile(env); ok {
			return prof, nil
		}
		return "", fmt.Errorf("invalid FLWD_PROFILE value %q", env)
	}
	if cfgProfile != "" {
		if prof, ok := normalizeProfile(cfgProfile); ok {
			return prof, nil
		}
		return "", fmt.Errorf("invalid configured security profile %q", cfgProfile)
	}
	return "secure", nil
}

func normalizeProfile(value string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "secure", "permissive", "disabled":
		return strings.ToLower(strings.TrimSpace(value)), true
	default:
		return "", false
	}
}

func (h *RunsHandler) resolveProvenance(jobID string, src *RunSourceRef, scriptDir, absScriptDir string) map[string]any {
	if h.resolveSrc != nil {
		if prov, ok := h.resolveSrc(jobID, src); ok && prov != nil {
			clone := make(map[string]any, len(prov))
			for k, v := range prov {
				clone[k] = v
			}
			return map[string]any{"source": clone}
		}
	}
	if src != nil && src.Name != "" && h.sources != nil {
		if storeSrc, ok := h.sources.Get(src.Name); ok {
			return map[string]any{"source": sourceToProvenance(storeSrc)}
		}
	}
	return map[string]any{
		"source": map[string]any{
			"type":         "local",
			"name":         jobID,
			"ref":          scriptDir,
			"resolved_ref": absScriptDir,
		},
	}
}

type runRequest struct {
	JobID                    string         `json:"job_id"`
	Args                     map[string]any `json:"args"`
	RequestedSecurityProfile string         `json:"requested_security_profile"`
	Source                   *RunSourceRef  `json:"source"`
}

// RunSourceRef represents a requested source reference for the run.
type RunSourceRef struct {
	Name string `json:"name"`
}

func decodeRunRequest(body io.ReadCloser) (runRequest, []byte, error) {
	defer body.Close()
	var req runRequest
	data, err := io.ReadAll(body)
	if err != nil {
		return req, nil, err
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		return req, data, err
	}
	if req.Args == nil {
		req.Args = map[string]any{}
	}
	return req, data, nil
}

func (h *RunsHandler) handleList(w http.ResponseWriter, r *http.Request) {
	page, perPage, err := parseRunsPagination(r)
	if err != nil {
		response.Write(w, response.New(http.StatusBadRequest, "invalid pagination", response.WithDetail(err.Error())))
		return
	}

	runs := h.store.List()
	start := (page - 1) * perPage
	if start >= len(runs) {
		runs = []runstore.Run{}
	} else {
		end := start + perPage
		if end > len(runs) {
			end = len(runs)
		}
		runs = runs[start:end]
	}

	payloads := make([]RunPayload, len(runs))
	for i, run := range runs {
		payloads[i] = payloadFromStore(run)
	}

	data, err := json.Marshal(payloads)
	if err != nil {
		response.Write(w, response.New(http.StatusInternalServerError, "encode runs failed", response.WithDetail(err.Error())))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

// HandleCancel processes POST /runs/{id}:cancel.
func (h *RunsHandler) HandleCancel(w http.ResponseWriter, r *http.Request, runID string) {
	if r.Method != http.MethodPost {
		response.Write(w, response.New(http.StatusMethodNotAllowed, "method not allowed"))
		return
	}
	if runID == "" {
		response.Write(w, response.New(http.StatusNotFound, "run not found"))
		return
	}
	run, ok := h.store.Get(runID)
	if !ok {
		response.Write(w, response.New(http.StatusNotFound, "run not found"))
		return
	}
	if isTerminalStatus(run.Status) {
		writeRunPayload(w, payloadFromStore(run), http.StatusOK)
		return
	}
	if value, ok := h.running.Load(runID); ok {
		if execCtx, ok := value.(*runExecutionContext); ok {
			if execCtx.cancel != nil {
				execCtx.cancel()
			}
		}
	}
	finished := time.Now().UTC()
	h.updateRunStatus(runID, "canceled", &finished)
	updated, _ := h.store.Get(runID)
	h.publishRunCanceled(updated, finished, "canceled by request")
	if logger := requestctx.Logger(r.Context()); logger != nil {
		logger.Info("run.cancel.request",
			slog.String("run_id", runID),
			slog.String("status", "canceled"),
			slog.String("reason", "canceled by request"),
		)
	}
	writeRunPayload(w, payloadFromStore(updated), http.StatusAccepted)
}

func parseRunsPagination(r *http.Request) (int, int, error) {
	page := defaultRunsPage
	perPage := defaultRunsPerPage

	q := r.URL.Query()

	if v := q.Get("page"); v != "" {
		val, err := strconv.Atoi(v)
		if err != nil || val <= 0 {
			return 0, 0, errors.New("page must be a positive integer")
		}
		page = val
	}

	if v := q.Get("per_page"); v != "" {
		val, err := strconv.Atoi(v)
		if err != nil {
			return 0, 0, errors.New("per_page must be numeric")
		}
		if val <= 0 || val > maxRunsPerPage {
			return 0, 0, errors.New("per_page must be between 1 and 200")
		}
		perPage = val
	}

	return page, perPage, nil
}

func encodeData(payload any) string {
	data, err := json.Marshal(payload)
	if err != nil {
		return "{}"
	}
	return string(data)
}

func sourceToProvenance(src sourcestore.Source) map[string]any {
	out := map[string]any{
		"name": src.Name,
		"type": src.Type,
	}
	if src.Ref != "" {
		out["ref"] = src.Ref
	}
	if src.Digest != "" {
		out["digest"] = src.Digest
	}
	if src.URL != "" {
		out["url"] = src.URL
	}
	if src.ResolvedRef != "" {
		out["resolved_ref"] = src.ResolvedRef
	}
	if src.PullPolicy != "" {
		out["pull_policy"] = src.PullPolicy
	}
	if len(src.Aliases) > 0 {
		aliasViews := make([]map[string]string, 0, len(src.Aliases))
		for _, alias := range src.Aliases {
			aliasViews = append(aliasViews, map[string]string{
				"from":        alias.From,
				"to":          alias.To,
				"description": alias.Description,
			})
		}
		out["aliases"] = aliasViews
	}
	if src.Trust != nil {
		trust := make(map[string]any, len(src.Trust))
		for k, v := range src.Trust {
			trust[k] = v
		}
		out["trust"] = trust
	}
	if src.Metadata != nil {
		meta := make(map[string]any, len(src.Metadata))
		for k, v := range src.Metadata {
			meta[k] = v
		}
		out["metadata"] = meta
	}
	if src.Digest != "" {
		if _, ok := out["resolved_ref"]; !ok || out["resolved_ref"] == "" {
			out["resolved_ref"] = src.Digest
		}
	}
	return out
}

func writePlanArtifact(plan types.Plan, runDir string) error {
	if runDir == "" {
		return fmt.Errorf("missing run directory")
	}
	if err := os.MkdirAll(runDir, 0o700); err != nil {
		return fmt.Errorf("create run dir: %w", err)
	}
	planPath := filepath.Join(runDir, "plan.json")
	f, err := os.OpenFile(planPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return fmt.Errorf("open plan file: %w", err)
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	if err := enc.Encode(plan); err != nil {
		return fmt.Errorf("write plan: %w", err)
	}
	return nil
}

func prepareSecrets(runDir string, binding *engine.Binding) (string, error) {
	if binding == nil || len(binding.SecretNames) == 0 {
		return "", nil
	}
	secretDir := filepath.Join(runDir, "secrets")
	if err := os.MkdirAll(secretDir, 0o700); err != nil {
		return "", fmt.Errorf("create secrets dir: %w", err)
	}
	for name := range binding.SecretNames {
		safeName := sanitizeSecretName(name)
		if safeName == "" {
			safeName = "secret"
		}
		path := filepath.Join(secretDir, safeName)
		value := ""
		if raw, ok := binding.Values[name]; ok && raw != nil {
			if s, ok := raw.(string); ok {
				value = s
			} else {
				value = fmt.Sprint(raw)
			}
		}
		if err := os.WriteFile(path, []byte(value), 0o600); err != nil {
			return "", fmt.Errorf("write secret %s: %w", name, err)
		}
	}
	return secretDir, nil
}

func sanitizeSecretName(name string) string {
	if name == "" {
		return ""
	}
	var b strings.Builder
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			b.WriteRune(r)
		} else {
			b.WriteRune('_')
		}
	}
	return b.String()
}

func publishPolicyDecisions(sink EventSink, payload *RunPayload, decisions []policyDecision) {
	if sink == nil || payload == nil || len(decisions) == 0 {
		return
	}
	base := map[string]any{
		"run_id":           payload.ID,
		"job_id":           payload.JobID,
		"security_profile": payload.SecurityProfile,
		"executor":         payload.Executor,
		"runtime":          payload.Runtime,
		"timestamp":        time.Now().UTC(),
	}
	if payload.Provenance != nil {
		base["provenance"] = payload.Provenance
	}
	for _, dec := range decisions {
		data := make(map[string]any, len(base)+4)
		for k, v := range base {
			data[k] = v
		}
		data["subject"] = dec.Subject
		data["decision"] = dec.Decision
		data["code"] = dec.Code
		if dec.Reason != "" {
			data["reason"] = dec.Reason
		}
		bytes, err := json.Marshal(data)
		if err != nil {
			bytes = []byte("{}")
		}
		sink.Publish(payload.ID, sse.Event{Event: "policy.decision", Data: string(bytes)})
	}
}

type runExecutionContext struct {
	ctx        context.Context
	cancel     context.CancelFunc
	runPayload RunPayload
	scriptDir  string
	config     *types.Config
	spec       *types.ArgSpec
	binding    *engine.Binding
	plan       types.Plan
	executor   string
	runtime    container.Runtime
	sink       events.Sink
}

func (h *RunsHandler) executeRun(execCtx *runExecutionContext) {
	if execCtx == nil {
		return
	}
	defer h.running.Delete(execCtx.runPayload.ID)
	if execCtx.cancel != nil {
		defer execCtx.cancel()
	}
	runID := execCtx.runPayload.ID
	jobID := execCtx.runPayload.JobID
	runDir := paths.RunDir(runID)
	absRunDir, err := filepath.Abs(runDir)
	if err != nil {
		h.failRun(runID, "failed", fmt.Errorf("resolve run dir: %w", err))
		return
	}
	runDir = absRunDir

	if err := os.MkdirAll(runDir, 0o700); err != nil {
		h.failRun(runID, "failed", fmt.Errorf("create run dir: %w", err))
		return
	}

	if err := writePlanArtifact(execCtx.plan, runDir); err != nil {
		h.failRun(runID, "failed", err)
		return
	}

	secretDir, err := prepareSecrets(runDir, execCtx.binding)
	if err != nil {
		h.failRun(runID, "failed", err)
		return
	}

	stdoutFile, err := os.OpenFile(filepath.Join(runDir, "stdout"), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		h.failRun(runID, "failed", fmt.Errorf("open stdout file: %w", err))
		return
	}
	defer stdoutFile.Close()

	stderrFile, err := os.OpenFile(filepath.Join(runDir, "stderr"), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		h.failRun(runID, "failed", fmt.Errorf("open stderr file: %w", err))
		return
	}
	defer stderrFile.Close()

	sink := events.NewCompositeSink(
		newSSESink(h.events, &execCtx.runPayload),
	)
	execCtx.sink = sink

	h.updateRunStatus(runID, "running", nil)
	if sink != nil {
		sink.EmitRunStart(runID, jobID)
	}

	stdoutWriter := io.MultiWriter(stdoutFile)
	stderrWriter := io.MultiWriter(stderrFile)

	execCfg := executor.ExecutorConfig{
		Flags:            map[string]interface{}{},
		Strict:           true,
		RunID:            runID,
		JobID:            jobID,
		Emitter:          sink,
		RunDir:           runDir,
		StdoutWriter:     stdoutWriter,
		StderrWriter:     stderrWriter,
		ContainerRuntime: execCtx.runtime,
	}
	if execCtx.binding != nil {
		execCfg.ArgEnv = execCtx.binding.ScalarEnv
		execCfg.ArgsJSON = execCtx.binding.ArgsJSON
		execCfg.ArgValues = execCtx.binding.Values
		execCfg.LineRedactor = events.NewLineRedactor(execCtx.binding.SecretValues)
	}
	if execCtx.config != nil {
		execCfg.EnvInherit = execCtx.config.EnvInheritance
		if c := execCtx.config.Container; c != nil {
			execCfg.ContainerNetwork = strings.TrimSpace(c.Network)
			execCfg.ContainerRootfsWritable = c.RootfsWritable
			if len(c.Capabilities) > 0 {
				execCfg.ContainerCapabilities = append([]string{}, c.Capabilities...)
			}
		}
	}
	if secretDir != "" {
		execCfg.SecretsDir = secretDir
	}

	runCtx := execCtx.ctx
	if runCtx == nil {
		runCtx = context.Background()
	}
	results, err := executor.RunScripts(runCtx, execCtx.scriptDir, execCfg)
	status := "completed"
	runErr := err
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(runCtx.Err(), context.Canceled) {
			status = "canceled"
			runErr = context.Canceled
		} else {
			status = "failed"
		}
	} else {
		for _, res := range results {
			if res.ExitCode != 0 {
				status = "failed"
				break
			}
		}
		if errors.Is(runCtx.Err(), context.Canceled) {
			status = "canceled"
			runErr = context.Canceled
		}
	}
	finished := time.Now().UTC()
	execCtx.runPayload.FinishedAt = &finished
	execCtx.runPayload.Status = status
	if sink != nil {
		sink.EmitRunFinish(runID, status, runErr)
	}
	prevStatus := ""
	if prev, ok := h.store.Get(runID); ok {
		prevStatus = prev.Status
	}
	h.updateRunStatus(runID, status, &finished)
	if status == "canceled" && prevStatus != "canceled" {
		if run, ok := h.store.Get(runID); ok {
			h.publishRunCanceled(run, finished, "canceled")
		}
	}
}

func (h *RunsHandler) updateRunStatus(runID, status string, finished *time.Time) {
	current, ok := h.store.Get(runID)
	if !ok {
		return
	}
	if isTerminalStatus(current.Status) && current.Status != status {
		return
	}
	current.Status = status
	if finished != nil {
		current.FinishedAt = finished
	}
	h.store.Update(current)
}

func (h *RunsHandler) failRun(runID string, status string, err error) {
	stamp := time.Now().UTC()
	h.updateRunStatus(runID, status, &stamp)
	if h.events != nil {
		payload := map[string]any{"status": status}
		if err != nil {
			payload["error"] = err.Error()
		}
		h.events.Publish(runID, sse.Event{
			Event: "run.finish",
			Data:  encodeData(payload),
		})
	}
}

func (h *RunsHandler) publishRunCanceled(run runstore.Run, finished time.Time, reason string) {
	if h.events == nil {
		return
	}
	payload := map[string]any{
		"run_id":    run.ID,
		"job_id":    run.JobID,
		"status":    "canceled",
		"timestamp": finished,
	}
	if reason != "" {
		payload["reason"] = reason
	}
	if run.Provenance != nil {
		payload["provenance"] = run.Provenance
	}
	if run.Runtime != "" {
		payload["runtime"] = run.Runtime
	}
	bytes, err := json.Marshal(payload)
	if err != nil {
		bytes = []byte("{}")
	}
	h.events.Publish(run.ID, sse.Event{Event: "run.canceled", Data: string(bytes)})
}

func isTerminalStatus(status string) bool {
	switch strings.ToLower(status) {
	case "completed", "failed", "canceled":
		return true
	default:
		return false
	}
}

// storageQuotaExceededProblem returns the RFC7807 payload used when Core DB
// storage limits prevent accepting new runs (spec §Scope — 429 reserved for
// storage-quota-exceeded conditions).
func storageQuotaExceededProblem() response.Problem {
	return response.New(http.StatusTooManyRequests, "storage quota exceeded",
		response.WithType(storageQuotaProblemType),
		response.WithDetail(storageQuotaProblemDetail),
	)
}

func canonicalizeJSON(raw []byte) ([]byte, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var val any
	if err := dec.Decode(&val); err != nil {
		return nil, err
	}
	buf := &bytes.Buffer{}
	if err := encodeCanonicalJSON(buf, val); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func encodeCanonicalJSON(buf *bytes.Buffer, v any) error {
	switch t := v.(type) {
	case map[string]any:
		keys := make([]string, 0, len(t))
		for k := range t {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		buf.WriteByte('{')
		for i, k := range keys {
			if i > 0 {
				buf.WriteByte(',')
			}
			writeJSONString(buf, k)
			buf.WriteByte(':')
			if err := encodeCanonicalJSON(buf, t[k]); err != nil {
				return err
			}
		}
		buf.WriteByte('}')
	case []any:
		buf.WriteByte('[')
		for i, elem := range t {
			if i > 0 {
				buf.WriteByte(',')
			}
			if err := encodeCanonicalJSON(buf, elem); err != nil {
				return err
			}
		}
		buf.WriteByte(']')
	case string:
		writeJSONString(buf, t)
	case json.Number:
		buf.WriteString(t.String())
	case float64:
		buf.WriteString(strconv.FormatFloat(t, 'f', -1, 64))
	case int:
		buf.WriteString(strconv.Itoa(t))
	case int64:
		buf.WriteString(strconv.FormatInt(t, 10))
	case bool:
		if t {
			buf.WriteString("true")
		} else {
			buf.WriteString("false")
		}
	case nil:
		buf.WriteString("null")
	default:
		b, err := json.Marshal(t)
		if err != nil {
			return err
		}
		buf.Write(b)
	}
	return nil
}

func writeJSONString(buf *bytes.Buffer, s string) {
	b, _ := json.Marshal(s)
	buf.Write(b)
}
