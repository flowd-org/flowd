package handlers

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/flowd-org/flowd/internal/configloader"
	"github.com/flowd-org/flowd/internal/indexer"
	"github.com/flowd-org/flowd/internal/server/headers"
	"github.com/flowd-org/flowd/internal/server/requestctx"
	"github.com/flowd-org/flowd/internal/server/response"
	"github.com/flowd-org/flowd/internal/server/sourcestore"
)

const (
	defaultPage     = 1
	defaultPerPage  = 50
	defaultMaxLimit = 200
)

// JobsConfig configures the jobs handler.
type JobsConfig struct {
	Root          string
	MaxPerPage    int
	Discover      func(string) (indexer.Result, error)
	Sources       *sourcestore.Store
	AliasesPublic bool
	ExposeAliases func(*http.Request) bool
}

type jobView struct {
	ID          string        `json:"id"`
	Name        string        `json:"name"`
	Tenant      string        `json:"tenant"`
	Origin      jobOrigin     `json:"origin"`
	Description string        `json:"description,omitempty"`
	Args        []interface{} `json:"args,omitempty"`
	Extends     []string      `json:"extends,omitempty"`
	Source      *jobSource    `json:"source,omitempty"`
	AliasOf     string        `json:"alias_of,omitempty"`
	AliasDetail string        `json:"alias_detail,omitempty"`
}

type jobOrigin struct {
	SourceKind string `json:"source_kind"`
	SourceName string `json:"source_name"`
}

type jobSource struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

// NewJobsHandler returns an HTTP handler for GET /jobs.
func NewJobsHandler(cfg JobsConfig) http.Handler {
	if cfg.MaxPerPage <= 0 {
		cfg.MaxPerPage = defaultMaxLimit
	}
	if cfg.Root == "" {
		cfg.Root = "scripts"
	}
	discoverFn := cfg.Discover
	if discoverFn == nil {
		discoverFn = indexer.Discover
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			response.Write(w, response.New(http.StatusMethodNotAllowed, "method not allowed"))
			return
		}

		exposeAliases := cfg.AliasesPublic
		if cfg.ExposeAliases != nil {
			exposeAliases = cfg.ExposeAliases(r)
		}

		page, perPage, err := parsePagination(r, cfg.MaxPerPage)
		if err != nil {
			response.Write(w, response.New(http.StatusBadRequest, "invalid pagination", response.WithDetail(err.Error())))
			return
		}

		resolvedTenant, prob := resolveTenant(r.Context(), "")
		if prob != nil {
			response.Write(w, *prob)
			return
		}

		logger := requestctx.Logger(r.Context())

		targets, err := resolveJobTargets(cfg.Root, cfg.Sources)
		if err != nil {
			response.Write(w, response.New(http.StatusInternalServerError, "resolve sources failed", response.WithDetail(err.Error())))
			return
		}

		var (
			allViews []jobView
			allJobs  []indexer.JobInfo
			errorCnt int
		)
		var collisionCandidates []jobCollisionContender

		aliasSets := make([]indexer.AliasSet, 0)
		aliasSources := make(map[string]struct{})
		if aliases, err := configloader.LoadAliases(cfg.Root); err != nil {
			response.Write(w, response.New(http.StatusInternalServerError, "load aliases failed", response.WithDetail(err.Error())))
			return
		} else if len(aliases) > 0 {
			aliasSets = append(aliasSets, indexer.AliasSet{Source: "", Aliases: aliases})
		}

		sourceKindByName := make(map[string]string)
		for _, target := range targets {
			if target.source != nil {
				sourceKindByName[target.source.Name] = mapSourceKind(target.source.Type)
			}
		}

		for _, target := range targets {
			origin := defaultJobOrigin()
			if target.source != nil {
				origin = jobOrigin{
					SourceKind: mapSourceKind(target.source.Type),
					SourceName: target.source.Name,
				}
			}
			if target.source != nil && len(target.source.Aliases) > 0 {
				if _, ok := aliasSources[target.source.Name]; !ok {
					aliasSets = append(aliasSets, indexer.AliasSet{Source: target.source.Name, Aliases: target.source.Aliases})
					aliasSources[target.source.Name] = struct{}{}
				}
			}
			if target.source != nil && strings.EqualFold(target.source.Type, "oci") {
				ociViews, ociErrors := discoverOCIJobs(*target.source, resolvedTenant, origin)
				allViews = append(allViews, ociViews...)
				for _, view := range ociViews {
					allJobs = append(allJobs, indexer.JobInfo{ID: view.ID, Name: view.Name})
				}
				errorCnt += len(ociErrors)
				continue
			}

			discovered, dErr := discoverJobsWithMountPath(target, discoverFn)
			if dErr != nil {
				if prob, ok := discoveryProblem(dErr); ok {
					response.Write(w, *prob)
					return
				}
				response.Write(w, response.New(http.StatusInternalServerError, "job discovery failed", response.WithDetail(dErr.Error())))
				return
			}
			for _, job := range discovered.Jobs {
				view := jobView{
					ID:          job.ID,
					Name:        job.Name,
					Tenant:      resolvedTenant,
					Origin:      origin,
					Description: job.Summary,
				}
				if target.source != nil {
					view.Source = &jobSource{
						Name: target.source.Name,
						Type: target.source.Type,
					}
				}
				allViews = append(allViews, view)
				allJobs = append(allJobs, job)
				collisionCandidates = append(collisionCandidates, buildJobCollisionContender(job, target.root, target.source))
			}
			errorCnt += len(discovered.Errors)
		}

		logJobsDiscoverySummary(logger, resolvedTenant, len(allJobs), len(targets), errorCnt)

		if canonicalID, contenders, ok := findJobIDCollision(collisionCandidates); ok {
			response.Write(w, jobIDCollisionProblem(canonicalID, contenders))
			return
		}

		aliasIndex, aliasErrs := indexer.BuildAliasIndex(allJobs, aliasSets)
		if len(aliasErrs) > 0 {
			errorCnt += len(aliasErrs)
		}
		if len(aliasIndex.Collisions) > 0 {
			errorCnt += len(aliasIndex.Collisions)
		}
		seenAliases := make(map[string]struct{})
		if exposeAliases {
			for _, alias := range aliasIndex.Entries {
				key := strings.ToLower(alias.Name)
				if _, exists := seenAliases[key]; exists {
					continue
				}
				seenAliases[key] = struct{}{}
				origin := defaultJobOrigin()
				if alias.Source != "" {
					if kind, ok := sourceKindByName[alias.Source]; ok {
						origin = jobOrigin{SourceKind: kind, SourceName: alias.Source}
					}
				}
				aliasView := jobView{
					ID:      alias.Name,
					Name:    alias.Name,
					Tenant:  resolvedTenant,
					Origin:  origin,
					AliasOf: alias.TargetPath,
				}
				aliasView.Description = fmt.Sprintf("[alias] %s", alias.TargetPath)
				if alias.Description != "" {
					aliasView.Description = alias.Description
				}
				if alias.Source != "" {
					aliasView.Source = &jobSource{Name: alias.Source, Type: "alias"}
				}
				if list, ok := aliasIndex.Collisions[key]; ok && len(list) > 1 {
					aliasView.AliasDetail = "collision"
					aliasView.Description = fmt.Sprintf("[alias] %s (collision)", alias.TargetPath)
				} else {
					aliasView.AliasDetail = alias.TargetPath
				}
				allViews = append(allViews, aliasView)
			}
		}

		if errorCnt > 0 {
			w.Header().Set(headers.DiscoveryErrors, strconv.Itoa(errorCnt))
		} else {
			w.Header().Set(headers.DiscoveryErrors, "0")
		}

		sort.Slice(allViews, func(i, j int) bool {
			if allViews[i].ID == allViews[j].ID {
				var left, right string
				if allViews[i].Source != nil {
					left = allViews[i].Source.Name
				}
				if allViews[j].Source != nil {
					right = allViews[j].Source.Name
				}
				return left < right
			}
			return allViews[i].ID < allViews[j].ID
		})

		start := (page - 1) * perPage
		var views []jobView
		if start >= len(allViews) {
			views = []jobView{}
		} else {
			end := start + perPage
			if end > len(allViews) {
				end = len(allViews)
			}
			views = allViews[start:end]
		}

		logJobsListedSummary(logger, resolvedTenant, page, perPage, len(allViews), len(views), errorCnt)

		payload, err := json.Marshal(views)
		if err != nil {
			response.Write(w, response.New(http.StatusInternalServerError, "encode response failed", response.WithDetail(err.Error())))
			return
		}

		w.Header().Set("Content-Type", "application/json")
		if _, err := w.Write(payload); err != nil {
			response.Write(w, response.New(http.StatusInternalServerError, "write response failed", response.WithDetail(err.Error())))
			return
		}
	})
}

func parsePagination(r *http.Request, maxPerPage int) (page int, perPage int, err error) {
	page = defaultPage
	perPage = defaultPerPage

	q := r.URL.Query()

	if v := q.Get("page"); v != "" {
		page, err = strconv.Atoi(v)
		if err != nil || page <= 0 {
			return 0, 0, errInvalidPage(v)
		}
	}

	if v := q.Get("per_page"); v != "" {
		perPage, err = strconv.Atoi(v)
		if err != nil {
			return 0, 0, errInvalidPerPage(v, maxPerPage)
		}
		if perPage <= 0 || perPage > maxPerPage {
			return 0, 0, errInvalidPerPage(v, maxPerPage)
		}
	}

	return page, perPage, nil
}

func errInvalidPage(v string) error {
	return &paginationError{Msg: "page must be a positive integer", Value: v}
}

func errInvalidPerPage(v string, max int) error {
	return &paginationError{Msg: "per_page must be between 1 and " + strconv.Itoa(max), Value: v}
}

type paginationError struct {
	Msg   string
	Value string
}

func (e *paginationError) Error() string {
	return e.Msg + " (got " + e.Value + ")"
}

type jobTarget struct {
	root   string
	source *sourcestore.Source
}

func resolveJobTargets(defaultRoot string, store *sourcestore.Store) ([]jobTarget, error) {
	root := defaultRoot
	if root == "" {
		root = "scripts"
	}

	targets := []jobTarget{{root: root}}

	seen := make(map[string]struct{})
	if abs, err := filepath.Abs(root); err == nil {
		seen[abs] = struct{}{}
	}

	if store == nil {
		return targets, nil
	}

	for _, src := range store.List() {
		if src.LocalPath == "" {
			continue
		}
		abs := src.LocalPath
		if absPath, err := filepath.Abs(src.LocalPath); err == nil {
			abs = absPath
		}
		if _, ok := seen[abs]; ok {
			continue
		}
		srcCopy := src
		targets = append(targets, jobTarget{
			root:   srcCopy.LocalPath,
			source: &srcCopy,
		})
		seen[abs] = struct{}{}
	}

	return targets, nil
}

func discoverJobsWithMountPath(target jobTarget, discoverFn func(string) (indexer.Result, error)) (indexer.Result, error) {
	if target.source == nil {
		return discoverFn(target.root)
	}
	mountPath := sourceNameMountPath(*target.source)
	if strings.TrimSpace(mountPath) == "" || mountPath == "." {
		return discoverFn(target.root)
	}
	return indexer.DiscoverWithMountPath(target.root, mountPath)
}

func sourceNameMountPath(src sourcestore.Source) string {
	return sourceLogicalMountPath(src)
}

func sourceLogicalMountPath(src sourcestore.Source) string {
	name := strings.TrimSpace(src.Name)
	if name == "" {
		return "."
	}
	return name
}

func discoverOCIJobs(src sourcestore.Source, tenant string, origin jobOrigin) ([]jobView, []indexer.DiscoveryError) {
	manifest, err := loadAddonManifestFromSource(src)
	if err != nil {
		return nil, []indexer.DiscoveryError{{
			Path: fmt.Sprintf("oci://%s", src.Name),
			Err:  err.Error(),
		}}
	}
	if manifest == nil {
		return nil, nil
	}
	var views []jobView
	for _, job := range manifest.Jobs {
		if strings.TrimSpace(job.ID) == "" {
			continue
		}
		id := composeOCIJobID(src.Name, job.ID)
		name := job.Name
		if strings.TrimSpace(name) == "" {
			name = id
		}
		view := jobView{
			ID:          id,
			Name:        name,
			Tenant:      tenant,
			Origin:      origin,
			Description: job.Summary,
			Source: &jobSource{
				Name: src.Name,
				Type: src.Type,
			},
		}
		views = append(views, view)
	}
	return views, nil
}

func defaultJobOrigin() jobOrigin {
	return jobOrigin{SourceKind: "fs", SourceName: "local"}
}

func mapSourceKind(sourceType string) string {
	if strings.EqualFold(sourceType, "local") {
		return "fs"
	}
	return strings.ToLower(strings.TrimSpace(sourceType))
}

func logJobsDiscoverySummary(logger *slog.Logger, tenant string, discovered, targets, discoveryErrors int) {
	if logger == nil {
		return
	}
	logger.Info("jobs.discovered",
		slog.String("tenant", tenant),
		slog.Int("discovered", discovered),
		slog.Int("targets", targets),
		slog.Int("discovery_errors", discoveryErrors),
	)
}

func logJobsListedSummary(logger *slog.Logger, tenant string, page, perPage, total, returned, discoveryErrors int) {
	if logger == nil {
		return
	}
	logger.Info("jobs.listed",
		slog.String("tenant", tenant),
		slog.Int("page", page),
		slog.Int("per_page", perPage),
		slog.Int("total", total),
		slog.Int("returned", returned),
		slog.Int("discovery_errors", discoveryErrors),
	)
}
