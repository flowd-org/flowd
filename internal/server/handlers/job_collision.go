package handlers

import (
	"net/http"
	"path/filepath"
	"sort"
	"strings"

	"github.com/flowd-org/flowd/internal/indexer"
	"github.com/flowd-org/flowd/internal/server/response"
	"github.com/flowd-org/flowd/internal/server/sourcestore"
)

type jobCollisionOrigin struct {
	SourceKind string `json:"source_kind"`
	SourceName string `json:"source_name"`
}

type jobCollisionContender struct {
	CanonicalJobID string             `json:"canonical_job_id"`
	Origin         jobCollisionOrigin `json:"origin"`
	MountPath      string             `json:"mountPath"`
	JobDir         string             `json:"job_dir"`
}

func buildCollisionCandidates(jobs []indexer.JobInfo, root string, src *sourcestore.Source) []jobCollisionContender {
	if len(jobs) == 0 {
		return nil
	}
	candidates := make([]jobCollisionContender, 0, len(jobs))
	for _, job := range jobs {
		candidates = append(candidates, buildJobCollisionContender(job, root, src))
	}
	return candidates
}

func buildJobCollisionContender(job indexer.JobInfo, root string, src *sourcestore.Source) jobCollisionContender {
	jobDir := job.Path
	if root != "" && job.Path != "" {
		if rel, err := filepath.Rel(root, job.Path); err == nil {
			jobDir = rel
		}
	}
	jobDir = filepath.ToSlash(strings.TrimSpace(jobDir))
	if jobDir == "" {
		jobDir = "."
	}
	origin := jobCollisionOrigin{
		SourceKind: normalizeJobCollisionSourceKind(""),
		SourceName: "local",
	}
	if src != nil {
		origin.SourceKind = normalizeJobCollisionSourceKind(src.Type)
		origin.SourceName = src.Name
	}
	return jobCollisionContender{
		CanonicalJobID: job.ID,
		Origin:         origin,
		MountPath:      ".",
		JobDir:         jobDir,
	}
}

func normalizeJobCollisionSourceKind(kind string) string {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "", "fs", "local":
		return "fs"
	case "git":
		return "git"
	case "oci":
		return "oci"
	default:
		return strings.ToLower(strings.TrimSpace(kind))
	}
}

func findJobIDCollision(candidates []jobCollisionContender) (string, []jobCollisionContender, bool) {
	byID := make(map[string][]jobCollisionContender, len(candidates))
	for _, candidate := range candidates {
		key := strings.ToLower(candidate.CanonicalJobID)
		byID[key] = append(byID[key], candidate)
	}
	keys := make([]string, 0, len(byID))
	for key, list := range byID {
		if len(list) > 1 {
			keys = append(keys, key)
		}
	}
	if len(keys) == 0 {
		return "", nil, false
	}
	sort.Strings(keys)
	key := keys[0]
	contenders := byID[key]
	sort.Slice(contenders, func(i, j int) bool {
		left := contenders[i]
		right := contenders[j]
		if left.CanonicalJobID != right.CanonicalJobID {
			return left.CanonicalJobID < right.CanonicalJobID
		}
		if left.Origin.SourceKind != right.Origin.SourceKind {
			return left.Origin.SourceKind < right.Origin.SourceKind
		}
		if left.Origin.SourceName != right.Origin.SourceName {
			return left.Origin.SourceName < right.Origin.SourceName
		}
		if left.MountPath != right.MountPath {
			return left.MountPath < right.MountPath
		}
		return left.JobDir < right.JobDir
	})
	canonicalID := contenders[0].CanonicalJobID
	return canonicalID, contenders, true
}

func jobIDCollisionProblem(canonicalID string, contenders []jobCollisionContender) response.Problem {
	detail := "multiple job definitions resolve to the same canonical id"
	if canonicalID != "" {
		detail = "multiple job definitions resolve to canonical id \"" + canonicalID + "\""
	}
	return response.New(http.StatusConflict, "job id collision",
		response.WithExtension("code", "job_id.collision"),
		response.WithDetail(detail),
		response.WithExtension("canonical_job_id", canonicalID),
		response.WithExtension("contenders", contenders),
	)
}
