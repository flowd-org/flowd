package handlers

import (
	"strings"

	"github.com/flowd-org/flowd/internal/indexer"
	"github.com/flowd-org/flowd/internal/server/sourcestore"
)

// Run tenant/origin are persisted in run provenance to keep a single source of
// truth across API responses, SSE payloads, and journal entries.

const (
	runProvenanceTenantKey = "tenant"
	runProvenanceOriginKey = "origin"
	runOriginKindKey       = "source_kind"
	runOriginNameKey       = "source_name"
)

func applyRunIdentityFromProvenance(payload *RunPayload) {
	if payload == nil {
		return
	}
	tenant, origin, ok := runIdentityFromProvenance(payload.Provenance)
	if !ok {
		return
	}
	if tenant != "" {
		payload.Tenant = tenant
	}
	if origin.SourceKind != "" || origin.SourceName != "" {
		payload.Origin = origin
	}
}

func runIdentityFromProvenance(provenance map[string]any) (string, jobOrigin, bool) {
	if len(provenance) == 0 {
		return "", jobOrigin{}, false
	}
	var (
		tenant string
		origin jobOrigin
		seen   bool
	)
	if value, ok := provenance[runProvenanceTenantKey]; ok {
		if str, ok := value.(string); ok {
			tenant = str
			seen = true
		}
	}
	if value, ok := provenance[runProvenanceOriginKey]; ok {
		origin = originFromAny(value)
		if origin.SourceKind != "" || origin.SourceName != "" {
			seen = true
		}
	}
	if !seen {
		return "", jobOrigin{}, false
	}
	return tenant, origin, true
}

func setRunIdentityInProvenance(provenance map[string]any, tenant string, origin jobOrigin) map[string]any {
	if provenance == nil {
		provenance = map[string]any{}
	}
	if tenant != "" {
		provenance[runProvenanceTenantKey] = tenant
	}
	if origin.SourceKind != "" || origin.SourceName != "" {
		provenance[runProvenanceOriginKey] = map[string]any{
			runOriginKindKey: origin.SourceKind,
			runOriginNameKey: origin.SourceName,
		}
	}
	return provenance
}

func originFromAny(value any) jobOrigin {
	switch t := value.(type) {
	case map[string]any:
		return originFromMap(t)
	case map[string]string:
		out := jobOrigin{}
		if kind, ok := t[runOriginKindKey]; ok {
			out.SourceKind = kind
		}
		if name, ok := t[runOriginNameKey]; ok {
			out.SourceName = name
		}
		return out
	default:
		return jobOrigin{}
	}
}

func originFromMap(values map[string]any) jobOrigin {
	var origin jobOrigin
	if kind, ok := values[runOriginKindKey]; ok {
		if str, ok := kind.(string); ok {
			origin.SourceKind = str
		}
	}
	if name, ok := values[runOriginNameKey]; ok {
		if str, ok := name.(string); ok {
			origin.SourceName = str
		}
	}
	return origin
}

func mergeJobOrigins(dest map[string]jobOrigin, jobs []indexer.JobInfo, src *sourcestore.Source) {
	if len(jobs) == 0 {
		return
	}
	origin := defaultJobOrigin()
	if src != nil {
		origin = jobOrigin{SourceKind: mapSourceKind(src.Type), SourceName: src.Name}
	}
	for _, job := range jobs {
		key := strings.ToLower(job.ID)
		if _, ok := dest[key]; ok {
			continue
		}
		dest[key] = origin
	}
}
