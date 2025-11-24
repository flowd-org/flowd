package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/flowd-org/flowd/internal/configloader"
	"github.com/flowd-org/flowd/internal/executor/container"
	"github.com/flowd-org/flowd/internal/paths"
	"github.com/flowd-org/flowd/internal/policy"
	policyverify "github.com/flowd-org/flowd/internal/policy/verify"
	"github.com/flowd-org/flowd/internal/server/metrics"
	"github.com/flowd-org/flowd/internal/server/requestctx"
	"github.com/flowd-org/flowd/internal/server/response"
	"github.com/flowd-org/flowd/internal/server/sourcestore"
	"github.com/flowd-org/flowd/internal/types"
)

// SourcesConfig configures the Sources handler.
type SourcesConfig struct {
	Store           *sourcestore.Store
	AllowLocalRoots []string
	AllowGitHosts   []string
	CheckoutDir     string
	Profile         string
	Policy          *policy.Context
	Verifier        policyverify.ImageVerifier
	Runtime         container.Runtime
	RuntimeDetector func() (container.Runtime, error)
	AliasesPublic   bool
	ExposeAliases   func(*http.Request) bool
}

type sourceRequest struct {
	Name             string                 `json:"name"`
	Type             string                 `json:"type"`
	Ref              string                 `json:"ref"`
	URL              string                 `json:"url"`
	Trusted          bool                   `json:"trusted"`
	PullPolicy       string                 `json:"pull_policy"`
	Trust            map[string]interface{} `json:"trust"`
	Expose           string                 `json:"expose"`
	VerifySignatures bool                   `json:"verify_signatures"`
}

var (
	errOCIPullFailure      = errors.New("oci pull failed")
	errOCICommandFailure   = errors.New("oci runtime command failed")
	errManifestMissing     = errors.New("addon manifest missing")
	errManifestInvalid     = errors.New("addon manifest invalid")
	ociRuntimeCommand      = defaultOCIRuntimeCommand
	ociCacheDirName        = "oci"
	addonManifestFileName  = "manifest.yaml"
	addonManifestMountPath = "/flwd-addon/" + addonManifestFileName
)

const problemTypeSignatureInvalid = "https://flowd.dev/problems/source-signature-invalid"

func normalizeExpose(value string) (string, error) {
	v := strings.ToLower(strings.TrimSpace(value))
	if v == "" {
		return "read", nil
	}
	switch v {
	case "none", "read", "readwrite":
		return v, nil
	default:
		return "", fmt.Errorf("invalid expose value %q", value)
	}
}

func exposeAllowsAliases(expose string) bool {
	switch strings.ToLower(expose) {
	case "", "read", "readwrite":
		return true
	default:
		return false
	}
}

func normalizePullPolicy(value string) (stored string, internal string, err error) {
	v := strings.ToLower(strings.TrimSpace(value))
	switch v {
	case "", "on-add", "always":
		return "always", "on-add", nil
	case "on-run", "ifnotpresent", "if-not-present", "if_not_present":
		return "ifNotPresent", "on-run", nil
	case "never":
		return "never", "on-run", nil
	default:
		return "", "", fmt.Errorf("invalid pull policy %q", value)
	}
}

func sanitizeSourceForResponse(src sourcestore.Source, includeAliases bool) sourcestore.Source {
	clone := src
	if !includeAliases || !exposeAllowsAliases(clone.Expose) {
		clone.Aliases = nil
	}
	return clone
}

func buildSourceProvenance(src sourcestore.Source) map[string]any {
	if src.Provenance != nil {
		return src.Provenance
	}
	out := map[string]any{}
	if src.Type != "" {
		out["type"] = src.Type
	}
	if src.Ref != "" {
		out["ref"] = src.Ref
	}
	if src.URL != "" {
		out["url"] = src.URL
	}
	if src.ResolvedCommit != "" {
		out["resolved_commit"] = src.ResolvedCommit
	}
	if src.ResolvedRef != "" {
		out["resolved_ref"] = src.ResolvedRef
	}
	if src.Digest != "" {
		out["digest"] = src.Digest
	}
	if src.PullPolicy != "" {
		out["pull_policy"] = src.PullPolicy
	}
	if src.VerifySignatures {
		out["verify_signatures"] = true
	}
	return out
}

type ociImageMetadata struct {
	Digest    string
	Created   string
	ImageID   string
	SizeBytes int64
	Labels    map[string]string
}

func defaultOCIRuntimeCommand(ctx context.Context, runtime container.Runtime, args ...string) ([]byte, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	cmd := exec.CommandContext(ctx, string(runtime), args...)
	return cmd.CombinedOutput()
}

// NewSourcesHandler returns a handler for GET/POST /sources.
func NewSourcesHandler(cfg SourcesConfig) http.Handler {
	if cfg.Store == nil {
		cfg.Store = sourcestore.New()
	}
	allowRoots := make([]string, 0, len(cfg.AllowLocalRoots))
	for _, root := range cfg.AllowLocalRoots {
		if root == "" {
			continue
		}
		abs, err := filepath.Abs(root)
		if err != nil {
			continue
		}
		allowRoots = append(allowRoots, filepath.Clean(abs))
	}
	cfg.AllowLocalRoots = allowRoots
	if cfg.CheckoutDir == "" {
		cfg.CheckoutDir = paths.SourcesDir()
	}
	if abs, err := filepath.Abs(cfg.CheckoutDir); err == nil {
		cfg.CheckoutDir = filepath.Clean(abs)
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			handleListSources(w, r, cfg)
		case http.MethodPost:
			handleUpsertSource(r.Context(), w, r, cfg)
		default:
			response.Write(w, response.New(http.StatusMethodNotAllowed, "method not allowed"))
		}
	})
}

func handleListSources(w http.ResponseWriter, r *http.Request, cfg SourcesConfig) {
	store := cfg.Store
	if store == nil {
		store = sourcestore.New()
	}
	items := store.List()
	includeAliases := shouldExposeAliases(r, cfg)
	for i := range items {
		if items[i].Provenance == nil {
			items[i].Provenance = buildSourceProvenance(items[i])
		}
		items[i] = sanitizeSourceForResponse(items[i], includeAliases)
	}
	data, err := json.Marshal(items)
	if err != nil {
		response.Write(w, response.New(http.StatusInternalServerError, "encode sources failed", response.WithDetail(err.Error())))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

func handleUpsertSource(ctx context.Context, w http.ResponseWriter, r *http.Request, cfg SourcesConfig) {
	defer r.Body.Close()
	var req sourceRequest
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		response.Write(w, response.New(http.StatusBadRequest, "invalid request body", response.WithDetail(err.Error())))
		return
	}

	if req.Type == "" {
		response.Write(w, response.New(http.StatusBadRequest, "type is required"))
		return
	}
	if req.Name != "" && strings.ContainsAny(req.Name, "/\\") {
		response.Write(w, response.New(http.StatusBadRequest, "invalid name", response.WithDetail("name must not contain path separators")))
		return
	}

	switch req.Type {
	case "local":
		handleLocalSource(w, req, cfg)
	case "git":
		handleGitSource(ctx, w, req, cfg)
	case "oci":
		handleOCISource(ctx, w, req, cfg)
	default:
		response.Write(w, response.New(http.StatusBadRequest, "unsupported source type", response.WithDetail(req.Type)))
	}
}

func shouldExposeAliases(r *http.Request, cfg SourcesConfig) bool {
	if cfg.ExposeAliases != nil {
		return cfg.ExposeAliases(r)
	}
	return cfg.AliasesPublic
}

func handleLocalSource(w http.ResponseWriter, req sourceRequest, cfg SourcesConfig) {
	if req.Ref == "" {
		response.Write(w, response.New(http.StatusBadRequest, "ref is required for local sources"))
		return
	}
	cleanRef := filepath.Clean(req.Ref)
	var allowedRoot string
	var absRef string
	for _, root := range cfg.AllowLocalRoots {
		if root == "" {
			continue
		}
		base := root
		if filepath.IsAbs(cleanRef) {
			absRef = cleanRef
		} else {
			absRef = filepath.Join(base, cleanRef)
		}
		absRef = filepath.Clean(absRef)
		if isSubPath(absRef, base) {
			allowedRoot = base
			break
		}
	}
	if allowedRoot == "" {
		response.Write(w, response.New(http.StatusBadRequest, "source not allowed",
			response.WithExtension("code", "source.not.allowed"),
			response.WithDetail("local path outside allow-list")))
		return
	}
	expose, err := normalizeExpose(req.Expose)
	if err != nil {
		response.Write(w, response.New(http.StatusBadRequest, "invalid expose",
			response.WithDetail(err.Error())))
		return
	}

	aliasDefs, aliasErr := loadSourceAliases(absRef)
	if aliasErr != nil {
		response.Write(w, response.New(http.StatusBadRequest, "invalid alias configuration",
			response.WithExtension("code", "alias.configuration.invalid"),
			response.WithDetail(aliasErr.Error())))
		return
	}

	name := req.Name
	if name == "" {
		name = deriveLocalName(cleanRef)
	}
	src := sourcestore.Source{
		Name:        name,
		Type:        "local",
		Ref:         cleanRef,
		ResolvedRef: absRef,
		Trust:       cloneTrust(req.Trust),
		Metadata: map[string]any{
			"resolved_path": absRef,
		},
		LocalPath: absRef,
		Aliases:   aliasDefs,
		Expose:    expose,
		Provenance: map[string]any{
			"type":          "local",
			"resolved_path": absRef,
		},
	}

	created := cfg.Store.Upsert(src)
	writeSourceResponse(w, sanitizeSourceForResponse(src, true), created)
}

func handleGitSource(ctx context.Context, w http.ResponseWriter, req sourceRequest, cfg SourcesConfig) {
	repoURL := strings.TrimSpace(req.URL)
	if repoURL == "" {
		repoURL = strings.TrimSpace(req.Ref)
	}
	if repoURL == "" {
		response.Write(w, response.New(http.StatusBadRequest, "url is required for git sources"))
		return
	}

	refName := strings.TrimSpace(req.Ref)
	if refName == "" {
		refName = "HEAD"
	}

	parsed, err := url.Parse(repoURL)
	if err != nil {
		response.Write(w, response.New(http.StatusBadRequest, "invalid git url", response.WithDetail(err.Error())))
		return
	}

	repoForClone := repoURL
	if isLocalGitURL(parsed) {
		localPath := parsed.Path
		if parsed.Scheme == "file" {
			if parsed.Host != "" {
				localPath = "//" + parsed.Host + parsed.Path
			}
		}
		if localPath == "" {
			localPath = repoURL
		}
		absPath, err := filepath.Abs(localPath)
		if err != nil {
			response.Write(w, response.New(http.StatusBadRequest, "invalid git url", response.WithDetail(err.Error())))
			return
		}
		absPath = filepath.Clean(absPath)
		allowed := false
		for _, root := range cfg.AllowLocalRoots {
			if isSubPath(absPath, root) {
				allowed = true
				break
			}
		}
		if !allowed {
			response.Write(w, response.New(http.StatusBadRequest, "source not allowed",
				response.WithExtension("code", "source.not.allowed"),
				response.WithDetail("git path outside allow-list")))
			return
		}
		repoForClone = absPath
	} else {
		host := strings.ToLower(parsed.Host)
		if !hostAllowed(host, cfg.AllowGitHosts) {
			response.Write(w, response.New(http.StatusBadRequest, "source not allowed",
				response.WithExtension("code", "source.not.allowed"),
				response.WithDetail("git host "+host+" not allowed")))
			return
		}
	}

	name := req.Name
	if name == "" {
		name = deriveGitName(parsed)
	}

	expose, err := normalizeExpose(req.Expose)
	if err != nil {
		response.Write(w, response.New(http.StatusBadRequest, "invalid expose",
			response.WithDetail(err.Error())))
		return
	}

	commit, checkoutPath, err := materializeGitSource(ctx, cfg.CheckoutDir, name, repoForClone, refName)
	if err != nil {
		response.Write(w, response.New(http.StatusBadRequest, "git checkout failed", response.WithDetail(err.Error())))
		return
	}

	metadata := map[string]any{
		"checkout_path": checkoutPath,
	}
	metadata["resolved_commit"] = commit
	metadata["ref"] = refName
	metadata["url"] = repoURL

	aliasDefs, aliasErr := loadSourceAliases(checkoutPath)
	if aliasErr != nil {
		response.Write(w, response.New(http.StatusBadRequest, "invalid alias configuration",
			response.WithExtension("code", "alias.configuration.invalid"),
			response.WithDetail(aliasErr.Error())))
		return
	}

	src := sourcestore.Source{
		Name:           name,
		Type:           "git",
		Ref:            refName,
		ResolvedRef:    commit,
		ResolvedCommit: commit,
		URL:            repoURL,
		Trust:          cloneTrust(req.Trust),
		Metadata:       metadata,
		LocalPath:      checkoutPath,
		Aliases:        aliasDefs,
		Expose:         expose,
		Provenance: map[string]any{
			"type":            "git",
			"resolved_commit": commit,
			"ref":             refName,
			"url":             repoURL,
		},
	}

	created := cfg.Store.Upsert(src)
	writeSourceResponse(w, sanitizeSourceForResponse(src, true), created)
}

func handleOCISource(ctx context.Context, w http.ResponseWriter, req sourceRequest, cfg SourcesConfig) {
	if ctx == nil {
		ctx = context.Background()
	}

	imageRef := strings.TrimSpace(req.Ref)
	if imageRef == "" {
		imageRef = strings.TrimSpace(req.URL)
	}
	if imageRef == "" {
		response.Write(w, response.New(http.StatusBadRequest, "ref is required for oci sources"))
		return
	}
	if strings.Contains(imageRef, "://") {
		response.Write(w, response.New(http.StatusBadRequest, "invalid image reference", response.WithDetail("image reference must not include scheme")))
		return
	}

	if !req.Trusted {
		response.Write(w, response.New(http.StatusBadRequest, "trust confirmation required",
			response.WithExtension("code", "source.trust.required"),
			response.WithDetail("oci sources require trusted=true")))
		return
	}

	storedPolicy, internalPolicy, err := normalizePullPolicy(req.PullPolicy)
	if err != nil {
		response.Write(w, response.New(http.StatusBadRequest, "invalid pull policy",
			response.WithDetail(err.Error())))
		return
	}
	expose, err := normalizeExpose(req.Expose)
	if err != nil {
		response.Write(w, response.New(http.StatusBadRequest, "invalid expose",
			response.WithDetail(err.Error())))
		return
	}

	effProfile, err := resolveEffectiveProfile("", cfg.Profile)
	if err != nil {
		response.Write(w, response.New(http.StatusUnprocessableEntity, "policy error",
			response.WithExtension("code", "E_POLICY"),
			response.WithDetail(err.Error())))
		return
	}
	ctx = requestctx.WithEffectiveProfile(ctx, effProfile)

	policyCtx := cfg.Policy
	if policyCtx == nil {
		policyCtx, err = policy.NewContext(nil)
		if err != nil {
			response.Write(w, response.New(http.StatusUnprocessableEntity, "policy error",
				response.WithExtension("code", "E_POLICY"),
				response.WithDetail(err.Error())))
			return
		}
	}

	if prob := enforceRegistryAllowList(ctx, imageRef, policyCtx); prob != nil {
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

	outcome, prob := enforceImageVerification(ctx, imageRef, mode, cfg.Verifier)
	if req.VerifySignatures {
		if mode == policy.VerifyModeDisabled {
			response.Write(w, response.New(http.StatusUnprocessableEntity, "signature verification required",
				response.WithType(problemTypeSignatureInvalid),
				response.WithExtension("code", "source-signature-invalid"),
				response.WithDetail("signature verification is disabled for the current profile")))
			return
		}
		if !outcome.Verified {
			detail := outcome.Reason
			if detail == "" {
				detail = "signature verification failed"
			}
			response.Write(w, response.New(http.StatusUnprocessableEntity, "signature verification failed",
				response.WithType(problemTypeSignatureInvalid),
				response.WithExtension("code", "source-signature-invalid"),
				response.WithDetail(detail)))
			return
		}
	} else if prob != nil {
		response.Write(w, *prob)
		return
	}

	runtimeVal, runtimeStr, runtimeErr := resolveRuntimeForOCI(ctx, cfg)
	if runtimeErr != nil {
		response.Write(w, runtimeUnavailableProblem(runtimeErr))
		return
	}
	ctx = requestctx.WithRuntime(ctx, runtimeStr)

	if internalPolicy == "on-add" {
		start := time.Now()
		if err := pullOCIImage(ctx, runtimeVal, imageRef); err != nil {
			detail := err.Error()
			response.Write(w, response.New(http.StatusBadRequest, "oci pull failed",
				response.WithExtension("code", "E_OCI"),
				response.WithDetail(detail)))
			return
		}
		metrics.Default.RecordContainerPull(time.Since(start))
	}

	manifestBytes, err := extractAddonManifest(ctx, runtimeVal, imageRef, effProfile, internalPolicy)
	if err != nil {
		switch {
		case errors.Is(err, errManifestMissing):
			metrics.Default.RecordAddonManifestInvalid()
			response.Write(w, response.New(http.StatusBadRequest, "addon manifest missing",
				response.WithExtension("code", "E_ADDON_MANIFEST"),
				response.WithDetail(err.Error())))
		case errors.Is(err, errManifestInvalid):
			metrics.Default.RecordAddonManifestInvalid()
			response.Write(w, response.New(http.StatusBadRequest, "addon manifest invalid",
				response.WithExtension("code", "E_ADDON_MANIFEST"),
				response.WithDetail(err.Error())))
		default:
			response.Write(w, response.New(http.StatusBadRequest, "oci command failed",
				response.WithExtension("code", "E_OCI"),
				response.WithDetail(err.Error())))
		}
		return
	}

	manifest, validationErrs, parseErr := parseAndValidateAddonManifest(manifestBytes)
	if parseErr != nil {
		metrics.Default.RecordAddonManifestInvalid()
		response.Write(w, response.New(http.StatusBadRequest, "addon manifest parse failed",
			response.WithExtension("code", "E_ADDON_MANIFEST"),
			response.WithDetail(parseErr.Error())))
		return
	}
	if len(validationErrs) > 0 {
		metrics.Default.RecordAddonManifestInvalid()
		response.Write(w, response.New(http.StatusBadRequest, "addon manifest invalid",
			response.WithExtension("code", "E_ADDON_MANIFEST"),
			response.WithDetail(strings.Join(validationErrs, "; "))))
		return
	}

	imageMeta, inspectErr := inspectImageMetadata(ctx, runtimeVal, imageRef)
	if inspectErr != nil {
		if internalPolicy == "on-run" {
			imageMeta = ociImageMetadata{}
		} else {
			response.Write(w, response.New(http.StatusBadRequest, "image inspect failed",
				response.WithExtension("code", "E_OCI"),
				response.WithDetail(inspectErr.Error())))
			return
		}
	}
	digest := imageMeta.Digest

	name := req.Name
	if name == "" {
		name = deriveOCIName(imageRef)
	}

	cacheRoot := deriveOCICacheRoot(cfg.CheckoutDir)
	manifestPath, writeErr := writeAddonManifest(cacheRoot, name, manifestBytes)
	if writeErr != nil {
		response.Write(w, response.New(http.StatusInternalServerError, "cache manifest failed",
			response.WithExtension("code", "E_OCI"),
			response.WithDetail(writeErr.Error())))
		return
	}

	metadata := map[string]any{
		"trusted":       true,
		"pull_policy":   storedPolicy,
		"manifest_path": manifestPath,
		"manifest":      manifestSummary(manifest),
	}
	if digest != "" {
		metadata["digest"] = digest
	}
	if imageMeta.ImageID != "" {
		metadata["image_id"] = imageMeta.ImageID
	}
	if imageMeta.Created != "" {
		metadata["created"] = imageMeta.Created
	}
	if imageMeta.SizeBytes > 0 {
		metadata["size_bytes"] = imageMeta.SizeBytes
	}
	if len(imageMeta.Labels) > 0 {
		metadata["labels"] = imageMeta.Labels
	}
	if mode != policy.VerifyModeDisabled {
		trustMeta := map[string]any{
			"verify_mode":        string(mode),
			"signature_verified": outcome.Verified,
		}
		if outcome.Reason != "" {
			trustMeta["signature_reason"] = outcome.Reason
		}
		metadata["image_trust"] = trustMeta
	}

	src := sourcestore.Source{
		Name:             name,
		Type:             "oci",
		Ref:              imageRef,
		ResolvedRef:      digest,
		URL:              strings.TrimSpace(req.URL),
		Trust:            cloneTrust(req.Trust),
		Metadata:         metadata,
		PullPolicy:       storedPolicy,
		Digest:           digest,
		LocalPath:        filepath.Dir(manifestPath),
		VerifySignatures: req.VerifySignatures,
		Expose:           expose,
		Provenance: buildSourceProvenance(sourcestore.Source{
			Type:             "oci",
			Ref:              imageRef,
			Digest:           digest,
			PullPolicy:       storedPolicy,
			VerifySignatures: req.VerifySignatures,
		}),
	}

	created := cfg.Store.Upsert(src)
	if created {
		metrics.Default.RecordSourceAdded(src.Type)
	}
	writeSourceResponse(w, sanitizeSourceForResponse(src, true), created)
}

func writeSourceResponse(w http.ResponseWriter, src sourcestore.Source, created bool) {
	data, err := json.Marshal(src)
	if err != nil {
		response.Write(w, response.New(http.StatusInternalServerError, "encode source failed", response.WithDetail(err.Error())))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if created {
		w.WriteHeader(http.StatusCreated)
	} else {
		w.WriteHeader(http.StatusOK)
	}
	_, _ = w.Write(data)
}

// NewSourceGetHandler returns a handler for GET/DELETE /sources/{name}.
func NewSourceGetHandler(cfg SourcesConfig) http.Handler {
	store := cfg.Store
	if store == nil {
		store = sourcestore.New()
	}
	cfg.Store = store
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimPrefix(r.URL.Path, "/sources/")
		if name == "" || strings.ContainsAny(name, "/\\") {
			response.Write(w, response.New(http.StatusNotFound, "source not found"))
			return
		}
		switch r.Method {
		case http.MethodGet:
			src, ok := store.Get(name)
			if !ok {
				response.Write(w, response.New(http.StatusNotFound, "source not found", response.WithDetail(name)))
				return
			}
			includeAliases := shouldExposeAliases(r, cfg)
			src = sanitizeSourceForResponse(src, includeAliases)
			if src.Provenance == nil {
				src.Provenance = buildSourceProvenance(src)
			}
			writeSourceResponse(w, src, false)
		case http.MethodDelete:
			if deleted := store.Delete(name); !deleted {
				response.Write(w, response.New(http.StatusNotFound, "source not found", response.WithDetail(name)))
				return
			}
			w.WriteHeader(http.StatusNoContent)
		default:
			response.Write(w, response.New(http.StatusMethodNotAllowed, "method not allowed"))
		}
	})
}

func loadSourceAliases(root string) ([]types.CommandAlias, error) {
	aliases, err := configloader.LoadAliases(root)
	if err != nil {
		return nil, err
	}
	return aliases, nil
}

func deriveLocalName(ref string) string {
	ref = strings.TrimSuffix(ref, string(filepath.Separator))
	base := filepath.Base(ref)
	if base == "." || base == string(filepath.Separator) {
		return "local"
	}
	return strings.ReplaceAll(base, " ", "-")
}

func deriveGitName(u *url.URL) string {
	path := strings.TrimSuffix(u.Path, "/")
	if idx := strings.LastIndex(path, "/"); idx >= 0 {
		path = path[idx+1:]
	}
	path = strings.TrimSuffix(path, ".git")
	if path == "" {
		path = "git-" + strings.ReplaceAll(u.Hostname(), ".", "-")
	}
	return path
}

func hostAllowed(host string, allow []string) bool {
	if len(allow) == 0 {
		return false
	}
	for _, allowed := range allow {
		if strings.EqualFold(host, allowed) {
			return true
		}
	}
	return false
}

func isSubPath(path, root string) bool {
	path = filepath.Clean(path)
	root = filepath.Clean(root)
	if len(path) < len(root) {
		return false
	}
	if path == root {
		return true
	}
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	return !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && rel != ".."
}

func cloneTrust(in map[string]interface{}) map[string]any {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func deriveOCIName(ref string) string {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return "oci"
	}
	if idx := strings.Index(ref, "@"); idx >= 0 {
		ref = ref[:idx]
	}
	lastSlash := strings.LastIndex(ref, "/")
	segment := ref
	if lastSlash >= 0 && lastSlash < len(ref)-1 {
		segment = ref[lastSlash+1:]
	}
	if idx := strings.LastIndex(segment, ":"); idx > 0 {
		segment = segment[:idx]
	}
	segment = strings.Trim(segment, "/")
	if segment == "" {
		segment = "oci"
	}
	segment = strings.ToLower(segment)

	var b strings.Builder
	lastDash := false
	for _, r := range segment {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
			lastDash = false
		case r >= '0' && r <= '9':
			b.WriteRune(r)
			lastDash = false
		case r == '-', r == '_':
			if lastDash && r == '-' {
				continue
			}
			b.WriteRune(r)
			lastDash = r == '-'
		case r == '.':
			b.WriteRune(r)
			lastDash = false
		default:
			if !lastDash {
				b.WriteRune('-')
				lastDash = true
			}
		}
	}
	name := strings.Trim(b.String(), "-_.")
	if name == "" {
		name = "oci"
	}
	return name
}

func isLocalGitURL(u *url.URL) bool {
	if u == nil {
		return false
	}
	if u.Scheme == "file" {
		return true
	}
	return u.Scheme == "" && u.Host == "" && u.Path != ""
}

func materializeGitSource(ctx context.Context, baseDir, name, repoURL, ref string) (string, string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if repoURL == "" {
		return "", "", errors.New("missing repository url")
	}
	if name == "" {
		return "", "", errors.New("missing source name")
	}
	if err := os.MkdirAll(baseDir, 0o755); err != nil {
		return "", "", fmt.Errorf("create sources dir: %w", err)
	}
	dest := filepath.Join(baseDir, name)
	if !isSubPath(dest, baseDir) {
		return "", "", errors.New("invalid source name")
	}
	if _, err := os.Stat(dest); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			if _, cloneErr := runGit(ctx, "", "clone", repoURL, dest); cloneErr != nil {
				return "", "", cloneErr
			}
		} else {
			return "", "", fmt.Errorf("stat checkout dir: %w", err)
		}
	} else {
		if _, err := os.Stat(filepath.Join(dest, ".git")); err != nil {
			return "", "", fmt.Errorf("destination %s exists and is not a git repository", dest)
		}
		if _, err := runGit(ctx, dest, "remote", "set-url", "origin", repoURL); err != nil {
			return "", "", err
		}
	}

	if _, err := runGit(ctx, dest, "fetch", "--all", "--tags", "--prune"); err != nil {
		return "", "", err
	}

	commit, err := resolveGitCommit(ctx, dest, ref)
	if err != nil {
		return "", "", err
	}

	if _, err := runGit(ctx, dest, "checkout", "--force", commit); err != nil {
		return "", "", err
	}
	if _, err := runGit(ctx, dest, "reset", "--hard", commit); err != nil {
		return "", "", err
	}
	if _, err := runGit(ctx, dest, "clean", "-fdx"); err != nil {
		return "", "", err
	}

	return commit, dest, nil
}

func resolveGitCommit(ctx context.Context, dir, ref string) (string, error) {
	if ref == "" || ref == "HEAD" {
		if out, err := runGit(ctx, dir, "rev-parse", "HEAD"); err == nil {
			return out, nil
		}
	}
	candidates := []string{ref}
	if !strings.HasPrefix(ref, "origin/") {
		candidates = append(candidates, "origin/"+ref)
	}
	if !strings.HasPrefix(ref, "refs/") {
		candidates = append(candidates, "refs/tags/"+ref)
	}
	var lastErr error
	for _, candidate := range candidates {
		if candidate == "" {
			continue
		}
		if out, err := runGit(ctx, dir, "rev-parse", "--verify", candidate); err == nil {
			return out, nil
		} else {
			lastErr = err
		}
	}
	if lastErr != nil {
		return "", lastErr
	}
	return "", fmt.Errorf("unable to resolve ref %q", ref)
}

func runGit(ctx context.Context, dir string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return strings.TrimSpace(stdout.String()), nil
}
