// SPDX-License-Identifier: AGPL-3.0-or-later
package server

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"

	"github.com/flowd-org/flowd/internal/coredb"
	"github.com/flowd-org/flowd/internal/executor/container"
	"github.com/flowd-org/flowd/internal/paths"
	"github.com/flowd-org/flowd/internal/policy"
	policyverify "github.com/flowd-org/flowd/internal/policy/verify"
	"github.com/flowd-org/flowd/internal/server/handlers"
	"github.com/flowd-org/flowd/internal/server/metrics"
	"github.com/flowd-org/flowd/internal/server/runstore"
	"github.com/flowd-org/flowd/internal/server/sourcestore"
	"github.com/flowd-org/flowd/internal/server/sse"
)

// Run boots the HTTP server until the context is canceled or an unrecoverable error occurs.
func Run(ctx context.Context, cfg Config) error {
	if cfg.DataDir != "" {
		paths.SetDataDirOverride(cfg.DataDir)
	}
	norm := cfg.normalize()
	paths.SetDataDirOverride(norm.DataDir)

	db, err := coredb.Open(ctx, norm.CoreDBOptions)
	if err != nil {
		return fmt.Errorf("open core db: %w", err)
	}
	defer db.Close()
	norm.CoreDB = db

	logger := newLogger(norm)
	runtimeDetector := norm.RuntimeDetector
	if runtimeDetector == nil {
		runtimeDetector = func() (container.Runtime, error) {
			return container.DetectRuntime(nil)
		}
	}
	if norm.MetricsEnabled {
		version := os.Getenv("FLWD_VERSION")
		if version == "" {
			version = "dev"
		}
		metrics.Default.SetBuildInfo(map[string]string{"version": version})
		metrics.Default.RecordSecurityProfileGauge(norm.Profile)
	}
	runtime, err := runtimeDetector()
	if err != nil {
		logger.Error("container runtime preflight failed", slog.String("error", err.Error()))
		return fmt.Errorf("container runtime preflight: %w", err)
	}
	logger.Info("container runtime ready", slog.String("runtime.selected", string(runtime)))
	norm.ContainerRuntime = runtime

	policyCtx, err := loadPolicyContext(ctx, norm.Profile, norm.PolicyVerifier)
	if err != nil {
		return err
	}
	verifier := norm.Verifier
	if verifier == nil {
		verifier = policyverify.NewCosignVerifier()
	}

	server := &http.Server{
		Addr:    norm.Bind,
		Handler: buildHandler(norm, policyCtx, verifier),
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- server.ListenAndServe()
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), norm.ShutdownTimeout)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil && !errors.Is(err, http.ErrServerClosed) {
			return err
		}
		if err := <-errCh; err != nil && !errors.Is(err, http.ErrServerClosed) {
			return err
		}
		return ctx.Err()
	case err := <-errCh:
		if err == nil || errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}

func loadPolicyContext(ctx context.Context, profile string, bundleVerifier policyverify.BundleVerifier) (*policy.Context, error) {
	bundle, bundlePath, err := policy.LoadFromEnvOrDefault()
	if err != nil {
		return nil, fmt.Errorf("load policy bundle: %w", err)
	}
	if bundle != nil && bundlePath != "" && strings.EqualFold(profile, "secure") {
		verifier := bundleVerifier
		if verifier == nil {
			verifier = policyverify.NewCosignBundleVerifier()
		}
		if err := verifier.Verify(ctx, bundlePath); err != nil {
			return nil, fmt.Errorf("verify policy bundle: %w", err)
		}
	}
	policyCtx, err := policy.NewContext(bundle)
	if err != nil {
		return nil, fmt.Errorf("policy context: %w", err)
	}
	return policyCtx, nil
}

func buildHandler(cfg Config, policyCtx *policy.Context, verifier policyverify.ImageVerifier) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(http.StatusNoContent)
	})
	if cfg.MetricsEnabled {
		mux.Handle("/metrics", metrics.Default.Handler())
	}

	sourceStore := sourcestore.New()
	exposeAliases := func(r *http.Request) bool {
		if cfg.AliasesPublic {
			return true
		}
		scopes, _, ok := AuthInfoFromContext(r.Context())
		if !ok {
			return false
		}
		for _, scope := range scopes {
			switch scope {
			case "sources:write", "jobs:write":
				return true
			}
		}
		return false
	}
	sourcesCfg := handlers.SourcesConfig{
		Store:           sourceStore,
		AllowLocalRoots: cfg.Sources.AllowLocalRoots,
		AllowGitHosts:   cfg.Sources.AllowGitHosts,
		CheckoutDir:     cfg.Sources.CheckoutDir,
		Profile:         cfg.Profile,
		Policy:          policyCtx,
		Verifier:        verifier,
		Runtime:         cfg.ContainerRuntime,
		RuntimeDetector: cfg.RuntimeDetector,
		AliasesPublic:   cfg.AliasesPublic,
		ExposeAliases:   exposeAliases,
	}
	mux.Handle("/sources", handlers.NewSourcesHandler(sourcesCfg))
	mux.Handle("/sources/", handlers.NewSourceGetHandler(sourcesCfg))

	kvStore := coredb.NewRuleYStore(cfg.CoreDB)
	kvAllow := make(map[string]handlers.KVNamespaceConfig, len(cfg.RuleY.Allowlist))
	for ns, entry := range cfg.RuleY.Allowlist {
		kvAllow[ns] = handlers.KVNamespaceConfig{LimitBytes: entry.LimitBytes}
	}
	mux.Handle("/kv/", handlers.NewKVHandler(handlers.KVConfig{
		Store:     kvStore,
		Allowlist: kvAllow,
	}))

	runStore := runstore.New()
	hub := sse.New(sse.Config{})
	globalHub := sse.New(sse.Config{})
	journal := coredb.NewJournal(cfg.CoreDB, cfg.CoreDBOptions.JournalMaxBytes)
	baseSink := handlers.EventSinkFunc(func(runID string, ev sse.Event) {
		hub.Publish(runID, ev)
		globalHub.Publish("global", handlers.WrapGlobalEvent(runID, ev))
	})
	eventSink := handlers.NewJournalEventSink(journal, baseSink)
	resolveSource := func(jobID string, ref *handlers.RunSourceRef) (map[string]any, bool) {
		var name string
		if ref != nil && ref.Name != "" {
			name = ref.Name
		} else {
			name = jobID
		}
		src, ok := sourceStore.Get(name)
		if !ok {
			return nil, false
		}
		return sourcetoProvenance(src), true
	}
	runGet := handlers.NewRunGetHandler(runStore)
	runEvents := handlers.NewRunEventsHandler(runStore, hub, journal)
	runEventsExport := handlers.NewRunEventsExportHandler(runStore, journal, cfg.ExtensionEnabled("export"))
	storageHealth := handlers.NewStorageHealthHandler(cfg.CoreDB)
	runHandler := handlers.NewRunsHandler(handlers.RunsConfig{
		Root:          cfg.ScriptsRoot,
		Store:         runStore,
		Events:        eventSink,
		ResolveSource: resolveSource,
		Sources:       sourceStore,
		Profile:       cfg.Profile,
		Policy:        policyCtx,
		Verifier:      verifier,
		Runtime:       cfg.ContainerRuntime,
		DB:            cfg.CoreDB,
	})
	mux.Handle("/jobs", handlers.NewJobsHandler(handlers.JobsConfig{
		Root:          cfg.ScriptsRoot,
		Sources:       sourceStore,
		AliasesPublic: cfg.AliasesPublic,
		ExposeAliases: exposeAliases,
	}))
	mux.Handle("/plans", handlers.NewPlansHandler(handlers.PlansConfig{
		Root:     cfg.ScriptsRoot,
		Sources:  sourceStore,
		Profile:  cfg.Profile,
		Policy:   policyCtx,
		Verifier: verifier,
		Runtime:  cfg.ContainerRuntime,
	}))
	mux.Handle("/runs", runHandler)
	mux.Handle("/runs/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, ":cancel") {
			id := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/runs/"), ":cancel")
			runHandler.HandleCancel(w, r, strings.Trim(id, "/"))
			return
		}
		if strings.HasSuffix(r.URL.Path, "/events.ndjson") {
			runEventsExport.ServeHTTP(w, r)
			return
		}
		if strings.HasSuffix(r.URL.Path, "/events") {
			runEvents.ServeHTTP(w, r)
			return
		}
		runGet.ServeHTTP(w, r)
	}))
	mux.Handle("/health/storage", storageHealth)
	mux.Handle("/events", handlers.NewEventsHandler(handlers.EventsConfig{
		RunStore:  runStore,
		RunHub:    hub,
		GlobalHub: globalHub,
	}))

	return chainMiddleware(mux,
		metricsMiddleware(cfg),
		loggingMiddleware(cfg),
		corsMiddleware(cfg),
		authMiddleware(cfg),
	)
}

func sourcetoProvenance(src sourcestore.Source) map[string]any {
	out := map[string]any{
		"name": src.Name,
		"type": src.Type,
	}
	if src.Ref != "" {
		out["ref"] = src.Ref
	}
	if src.URL != "" {
		out["url"] = src.URL
	}
	if src.ResolvedRef != "" {
		out["resolved_ref"] = src.ResolvedRef
	}
	if src.ResolvedCommit != "" {
		out["resolved_commit"] = src.ResolvedCommit
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
	if src.Provenance != nil {
		for k, v := range src.Provenance {
			if _, exists := out[k]; !exists {
				out[k] = v
			}
		}
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
