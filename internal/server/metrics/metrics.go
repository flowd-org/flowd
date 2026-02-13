// SPDX-License-Identifier: AGPL-3.0-or-later
package metrics

import (
	"bufio"
	"fmt"
	"math"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	coremetrics "github.com/flowd-org/flowd/internal/metrics"
	"github.com/flowd-org/flowd/internal/server/buildinfo"
)

// Registry collects counters, gauges, and histograms for Prometheus exposition.
type Registry struct {
	mu sync.Mutex

	httpRequests          *httpHistogram
	securityProfileGauge  string
	buildInfoLabels       map[string]string
	policyDenials         map[string]uint64
	containerRuns         *simpleHistogram
	containerPulls        *simpleHistogram
	containerRunsTotal    uint64
	containerPullsTotal   uint64
	sourcesAdded          map[string]uint64
	addonManifestInvalid  uint64
	persistenceLatency    map[[2]string]*valueHistogram
	persistenceEvictions  map[string]uint64
	persistenceBytes      map[string]uint64
	kvQuotaExceeded       map[string]uint64
	artifactWriteFailed   map[string]uint64
	idempotencyLookups    map[string]uint64
	idempotencyReplays    uint64
	idempotencyConflicts  uint64
	idempotencyInFlight   uint64
	sseActive             map[string]int64
	sseStreamStarts       map[string]uint64
	sseStreamEnds         map[string]uint64
	sseResumeTotal        uint64
	sseCursorExpiredTotal uint64
	runStartedTotal       uint64
	runFinishedTotals     map[string]uint64
	secretHandlesCreated  uint64
	secretHandlesCleaned  uint64
	secretHandleErrors    uint64
	secretContainerReject uint64
}

// NewRegistry constructs a metrics registry with default buckets.
func NewRegistry() *Registry {
	r := &Registry{
		httpRequests:  newHTTPHistogram(),
		policyDenials: make(map[string]uint64),
		sourcesAdded:  make(map[string]uint64),
		buildInfoLabels: map[string]string{
			"version":      "dev",
			"spec_version": buildinfo.CoreSpecVersion,
		},
		containerRuns:        newSimpleHistogram([]float64{0.5, 1, 2, 5, 10, 20, 60, 120, 300}),
		containerPulls:       newSimpleHistogram([]float64{0.5, 1, 2, 5, 10, 20, 60, 120, 300}),
		persistenceLatency:   make(map[[2]string]*valueHistogram),
		persistenceEvictions: make(map[string]uint64),
		persistenceBytes:     make(map[string]uint64),
		kvQuotaExceeded:      make(map[string]uint64),
		artifactWriteFailed:  make(map[string]uint64),
		idempotencyLookups:   make(map[string]uint64),
		sseActive:            make(map[string]int64),
		sseStreamStarts:      make(map[string]uint64),
		sseStreamEnds:        make(map[string]uint64),
		runFinishedTotals:    make(map[string]uint64),
	}
	for op, outcomes := range persistenceLatencyDefaults {
		op = normalizeLabel(op)
		for _, outcome := range outcomes {
			key := [2]string{op, normalizeLabel(outcome)}
			r.persistenceLatency[key] = newValueHistogram(persistenceLatencyBuckets)
		}
	}
	for _, kind := range persistenceDefaultKinds {
		r.persistenceEvictions[normalizeLabel(kind)] = 0
		r.persistenceBytes[normalizeLabel(kind)] = 0
	}
	for _, outcome := range idempotencyLookupOutcomes {
		r.idempotencyLookups[normalizeLabel(outcome)] = 0
	}
	return r
}

// Default global registry used by the server.
var Default = NewRegistry()

func init() {
	coremetrics.SetSink(Default)
}

// SetBuildInfo configures the build info labels exposed by flwd_build_info.
func (r *Registry) SetBuildInfo(labels map[string]string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for k, v := range labels {
		r.buildInfoLabels[k] = v
	}
}

// RecordHTTP records an HTTP request metric.
func (r *Registry) RecordHTTP(route, method string, status int, duration time.Duration) {
	if r == nil || route == "" || method == "" {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.httpRequests.observe(route, method, status, duration)
}

// RecordSecurityProfileGauge sets the active security profile gauge (1 for active).
func (r *Registry) RecordSecurityProfileGauge(profile string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.securityProfileGauge = profile
}

// RecordPolicyDenial increments policy denial counter for a reason.
func (r *Registry) RecordPolicyDenial(reason string) {
	if reason == "" {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.policyDenials[reason]++
}

// RecordContainerRun records container run duration and increments counters.
func (r *Registry) RecordContainerRun(duration time.Duration) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.containerRuns.observe(duration)
	r.containerRunsTotal++
}

// RecordContainerPull records container pull duration and increments counters.
func (r *Registry) RecordContainerPull(duration time.Duration) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.containerPulls.observe(duration)
	r.containerPullsTotal++
}

// RecordSourceAdded increments the counter for sources added by type.
func (r *Registry) RecordSourceAdded(sourceType string) {
	if r == nil || strings.TrimSpace(sourceType) == "" {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.sourcesAdded[strings.ToLower(strings.TrimSpace(sourceType))]++
}

// RecordAddonManifestInvalid increments the counter for invalid addon manifests.
func (r *Registry) RecordAddonManifestInvalid() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.addonManifestInvalid++
}

// SourceAddedTotals returns a copy of the added sources counter for testing.
func (r *Registry) SourceAddedTotals() map[string]uint64 {
	r.mu.Lock()
	defer r.mu.Unlock()
	copy := make(map[string]uint64, len(r.sourcesAdded))
	for k, v := range r.sourcesAdded {
		copy[k] = v
	}
	return copy
}

// AddonManifestInvalidTotal returns the count of invalid manifests recorded.
func (r *Registry) AddonManifestInvalidTotal() uint64 {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.addonManifestInvalid
}

// ContainerPullsTotal exposes the pull counter for testing.
func (r *Registry) ContainerPullsTotal() uint64 {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.containerPullsTotal
}

// Handler returns an http.Handler that writes Prometheus text exposition.
func (r *Registry) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		r.writeAll(w)
	})
}

func (r *Registry) writeAll(w http.ResponseWriter) {
	r.mu.Lock()
	defer r.mu.Unlock()
	buf := bufio.NewWriter(w)
	defer buf.Flush()

	writeMetricHeader(buf, "http_requests_total", "Total HTTP requests", "counter")
	keys := r.httpRequests.sortedKeys()
	for _, key := range keys {
		route, method, code := key[0], key[1], key[2]
		fmt.Fprintf(buf, "http_requests_total{method=%q,route=%q,code=%q} %.0f\n", method, route, code, r.httpRequests.total(route, method, code))
	}
	buf.WriteByte('\n')

	writeMetricHeader(buf, "http_request_duration_seconds", "HTTP request latency in seconds", "histogram")
	r.httpRequests.writeHistograms(buf)
	buf.WriteByte('\n')

	writeMetricHeader(buf, "flwd_build_info", "Runner build info", "gauge")
	buf.WriteString("flwd_build_info")
	buf.WriteByte('{')
	first := true
	for _, key := range sortedKeys(r.buildInfoLabels) {
		if !first {
			buf.WriteByte(',')
		}
		first = false
		fmt.Fprintf(buf, "%s=%q", key, r.buildInfoLabels[key])
	}
	buf.WriteString("} 1\n\n")

	writeMetricHeader(buf, "flwd_security_profile", "Active security profile", "gauge")
	if r.securityProfileGauge != "" {
		fmt.Fprintf(buf, "flwd_security_profile{profile=%q} 1\n\n", r.securityProfileGauge)
	} else {
		buf.WriteString("flwd_security_profile{profile=\"\"} 0\n\n")
	}

	writeMetricHeader(buf, "flwd_policy_denials_total", "Policy denials by reason", "counter")
	for _, reason := range sortedKeysUint(r.policyDenials) {
		fmt.Fprintf(buf, "flwd_policy_denials_total{reason=%q} %d\n", reason, r.policyDenials[reason])
	}
	buf.WriteByte('\n')

	writeMetricHeader(buf, "flowd_persistence_latency_ms", "Persistence operation latency in milliseconds", "histogram")
	persistenceKeys := make([][2]string, 0, len(r.persistenceLatency))
	for key := range r.persistenceLatency {
		persistenceKeys = append(persistenceKeys, key)
	}
	sort.Slice(persistenceKeys, func(i, j int) bool {
		a, b := persistenceKeys[i], persistenceKeys[j]
		if a[0] != b[0] {
			return a[0] < b[0]
		}
		return a[1] < b[1]
	})
	for _, key := range persistenceKeys {
		op, outcome := key[0], key[1]
		r.persistenceLatency[key].writeWithLabels(buf, "flowd_persistence_latency_ms", map[string]string{
			"operation": op,
			"outcome":   outcome,
		})
	}
	buf.WriteByte('\n')

	writeMetricHeader(buf, "flowd_persistence_evictions_total", "Persistence evictions by kind", "counter")
	for _, kind := range sortedKeysUint(r.persistenceEvictions) {
		fmt.Fprintf(buf, "flowd_persistence_evictions_total{kind=%q} %d\n", kind, r.persistenceEvictions[kind])
	}
	buf.WriteByte('\n')

	writeMetricHeader(buf, "flowd_persistence_eviction_bytes_total", "Bytes reclaimed by persistence evictions", "counter")
	for _, kind := range sortedKeysUint(r.persistenceBytes) {
		fmt.Fprintf(buf, "flowd_persistence_eviction_bytes_total{kind=%q} %d\n", kind, r.persistenceBytes[kind])
	}
	buf.WriteByte('\n')

	writeMetricHeader(buf, "kv_quota_exceeded_total", "KV quota exceeded responses by namespace", "counter")
	for _, namespace := range sortedKeysUint(r.kvQuotaExceeded) {
		fmt.Fprintf(buf, "kv_quota_exceeded_total{namespace=%q} %d\n", namespace, r.kvQuotaExceeded[namespace])
	}
	buf.WriteByte('\n')

	writeMetricHeader(buf, "artifacts_write_failed_total", "Artifact write failures by reason", "counter")
	for _, reason := range sortedKeysUint(r.artifactWriteFailed) {
		fmt.Fprintf(buf, "artifacts_write_failed_total{reason=%q} %d\n", reason, r.artifactWriteFailed[reason])
	}
	buf.WriteByte('\n')

	writeMetricHeader(buf, "flowd_idempotency_lookup_total", "Idempotency lookup outcomes", "counter")
	for _, outcome := range sortedKeysUint(r.idempotencyLookups) {
		fmt.Fprintf(buf, "flowd_idempotency_lookup_total{outcome=%q} %d\n", outcome, r.idempotencyLookups[outcome])
	}
	buf.WriteByte('\n')

	writeMetricHeader(buf, "flowd_idempotency_replay_total", "Idempotency replay responses", "counter")
	fmt.Fprintf(buf, "flowd_idempotency_replay_total %d\n", r.idempotencyReplays)
	buf.WriteByte('\n')

	writeMetricHeader(buf, "flowd_idempotency_conflict_total", "Idempotency conflict responses", "counter")
	fmt.Fprintf(buf, "flowd_idempotency_conflict_total %d\n", r.idempotencyConflicts)
	buf.WriteByte('\n')

	writeMetricHeader(buf, "flowd_idempotency_inflight_total", "Idempotency in-flight responses", "counter")
	fmt.Fprintf(buf, "flowd_idempotency_inflight_total %d\n", r.idempotencyInFlight)
	buf.WriteByte('\n')

	writeMetricHeader(buf, "flowd_sse_active_streams", "Active SSE streams by transport", "gauge")
	for _, transport := range sortedKeysInt64(r.sseActive) {
		fmt.Fprintf(buf, "flowd_sse_active_streams{transport=%q} %d\n", transport, r.sseActive[transport])
	}
	buf.WriteByte('\n')

	writeMetricHeader(buf, "flowd_sse_stream_start_total", "SSE stream starts by transport", "counter")
	for _, transport := range sortedKeysUint(r.sseStreamStarts) {
		fmt.Fprintf(buf, "flowd_sse_stream_start_total{transport=%q} %d\n", transport, r.sseStreamStarts[transport])
	}
	buf.WriteByte('\n')

	writeMetricHeader(buf, "flowd_sse_stream_end_total", "SSE stream ends by transport", "counter")
	for _, transport := range sortedKeysUint(r.sseStreamEnds) {
		fmt.Fprintf(buf, "flowd_sse_stream_end_total{transport=%q} %d\n", transport, r.sseStreamEnds[transport])
	}
	buf.WriteByte('\n')

	writeMetricHeader(buf, "flowd_sse_resume_total", "SSE resume attempts", "counter")
	fmt.Fprintf(buf, "flowd_sse_resume_total %d\n", r.sseResumeTotal)
	buf.WriteByte('\n')

	writeMetricHeader(buf, "flowd_sse_cursor_expired_total", "SSE cursor expired responses (HTTP 410)", "counter")
	fmt.Fprintf(buf, "flowd_sse_cursor_expired_total %d\n", r.sseCursorExpiredTotal)
	buf.WriteByte('\n')

	r.writeHistogram(buf, "flwd_container_runs_total", "counter", func() (float64, bool) {
		return float64(r.containerRunsTotal), true
	})
	r.containerRuns.write(buf, "flwd_container_run_seconds")

	r.writeHistogram(buf, "flwd_container_pulls_total", "counter", func() (float64, bool) {
		return float64(r.containerPullsTotal), true
	})
	r.containerPulls.write(buf, "flwd_container_pull_seconds")

	writeMetricHeader(buf, "flwd_sources_added_total", "Sources added by type", "counter")
	for _, sourceType := range sortedKeysUint(r.sourcesAdded) {
		fmt.Fprintf(buf, "flwd_sources_added_total{type=%q} %d\n", sourceType, r.sourcesAdded[sourceType])
	}
	buf.WriteByte('\n')

	writeMetricHeader(buf, "flwd_addon_manifest_invalid_total", "Invalid add-on manifests", "counter")
	fmt.Fprintf(buf, "flwd_addon_manifest_invalid_total %d\n\n", r.addonManifestInvalid)

	writeMetricHeader(buf, "flowd_runs_started_total", "Run started events", "counter")
	fmt.Fprintf(buf, "flowd_runs_started_total %d\n", r.runStartedTotal)
	buf.WriteByte('\n')

	writeMetricHeader(buf, "flowd_runs_finished_total", "Run finished events by status", "counter")
	for _, status := range sortedKeysUint(r.runFinishedTotals) {
		fmt.Fprintf(buf, "flowd_runs_finished_total{status=%q} %d\n", status, r.runFinishedTotals[status])
	}
	buf.WriteByte('\n')

	writeMetricHeader(buf, "flowd_secret_handles_created_total", "Secret handles created", "counter")
	fmt.Fprintf(buf, "flowd_secret_handles_created_total %d\n", r.secretHandlesCreated)
	buf.WriteByte('\n')

	writeMetricHeader(buf, "flowd_secret_handles_cleaned_total", "Secret handles cleaned", "counter")
	fmt.Fprintf(buf, "flowd_secret_handles_cleaned_total %d\n", r.secretHandlesCleaned)
	buf.WriteByte('\n')

	writeMetricHeader(buf, "flowd_secret_handle_errors_total", "Secret handle cleanup errors", "counter")
	fmt.Fprintf(buf, "flowd_secret_handle_errors_total %d\n", r.secretHandleErrors)
	buf.WriteByte('\n')

	writeMetricHeader(buf, "flowd_secret_container_rejected_total", "Container runs rejected due to secret args", "counter")
	fmt.Fprintf(buf, "flowd_secret_container_rejected_total %d\n", r.secretContainerReject)
	buf.WriteByte('\n')
}

func (r *Registry) writeHistogram(buf *bufio.Writer, name, metricType string, getter func() (float64, bool)) {
	writeMetricHeader(buf, name, "", metricType)
	if val, ok := getter(); ok {
		fmt.Fprintf(buf, "%s %.0f\n\n", name, val)
	} else {
		fmt.Fprintf(buf, "%s 0\n\n", name)
	}
}

func writeMetricHeader(buf *bufio.Writer, name, help, metricType string) {
	if help != "" {
		fmt.Fprintf(buf, "# HELP %s %s\n", name, escapeHelp(help))
	}
	if metricType != "" {
		fmt.Fprintf(buf, "# TYPE %s %s\n", name, metricType)
	}
}

func escapeHelp(help string) string {
	return strings.ReplaceAll(help, "\\", "\\\\")
}

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func sortedKeysUint(m map[string]uint64) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func sortedKeysInt64(m map[string]int64) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

type httpHistogram struct {
	// key: route|method|code
	counts map[[3]string]uint64
	hist   map[string]*simpleHistogram
}

func newHTTPHistogram() *httpHistogram {
	return &httpHistogram{
		counts: make(map[[3]string]uint64),
		hist:   make(map[string]*simpleHistogram),
	}
}

func (h *httpHistogram) observe(route, method string, status int, duration time.Duration) {
	key := [3]string{route, method, strconv.Itoa(status)}
	h.counts[key]++
	label := route + "|" + method + "|" + strconv.Itoa(status)
	b, ok := h.hist[label]
	if !ok {
		b = newSimpleHistogram([]float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5, 10})
		h.hist[label] = b
	}
	b.observe(duration)
}

func (h *httpHistogram) total(route, method, code string) float64 {
	key := [3]string{route, method, code}
	return float64(h.counts[key])
}

func (h *httpHistogram) sortedKeys() [][3]string {
	keys := make([][3]string, 0, len(h.counts))
	for k := range h.counts {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		a, b := keys[i], keys[j]
		if a[0] != b[0] {
			return a[0] < b[0]
		}
		if a[1] != b[1] {
			return a[1] < b[1]
		}
		return a[2] < b[2]
	})
	return keys
}

func (h *httpHistogram) writeHistograms(buf *bufio.Writer) {
	keys := make([]string, 0, len(h.hist))
	for k := range h.hist {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, label := range keys {
		parts := strings.Split(label, "|")
		if len(parts) != 3 {
			continue
		}
		route, method, code := parts[0], parts[1], parts[2]
		h.hist[label].writeWithLabels(buf, "http_request_duration_seconds", map[string]string{
			"method": method,
			"route":  route,
			"code":   code,
		})
	}
}

type simpleHistogram struct {
	buckets []float64
	counts  []uint64
	sum     float64
	count   uint64
}

func newSimpleHistogram(buckets []float64) *simpleHistogram {
	return &simpleHistogram{
		buckets: append([]float64(nil), buckets...),
		counts:  make([]uint64, len(buckets)),
	}
}

func (h *simpleHistogram) observe(duration time.Duration) {
	if h == nil {
		return
	}
	sec := duration.Seconds()
	for i, upper := range h.buckets {
		if sec <= upper {
			h.counts[i]++
		}
	}
	// +Inf bucket
	h.count++
	h.sum += sec
}

func (h *simpleHistogram) write(buf *bufio.Writer, name string) {
	if h == nil {
		return
	}
	writeMetricHeader(buf, name, "", "histogram")
	labels := map[string]string{}
	h.writeWithLabels(buf, name, labels)
}

func (h *simpleHistogram) writeWithLabels(buf *bufio.Writer, name string, labels map[string]string) {
	if h == nil {
		return
	}
	for i, upper := range h.buckets {
		labelStr := labelsWithLE(labels, upper)
		fmt.Fprintf(buf, "%s_bucket%s %d\n", name, labelStr, h.counts[i])
	}
	labelStr := labelsWithLE(labels, math.Inf(1))
	fmt.Fprintf(buf, "%s_bucket%s %d\n", name, labelStr, h.count)
	fmt.Fprintf(buf, "%s_sum%s %g\n", name, labelsToString(labels), h.sum)
	fmt.Fprintf(buf, "%s_count%s %d\n", name, labelsToString(labels), h.count)
	buf.WriteByte('\n')
}

func labelsWithLE(labels map[string]string, le float64) string {
	labelCopy := make(map[string]string, len(labels)+1)
	for k, v := range labels {
		labelCopy[k] = v
	}
	if math.IsInf(le, 1) {
		labelCopy["le"] = "+Inf"
	} else {
		labelCopy["le"] = strconv.FormatFloat(le, 'f', -1, 64)
	}
	return labelsToString(labelCopy)
}

func labelsToString(labels map[string]string) string {
	if len(labels) == 0 {
		return ""
	}
	keys := make([]string, 0, len(labels))
	for k := range labels {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(labels))
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("%s=%q", k, labels[k]))
	}
	return "{" + strings.Join(parts, ",") + "}"
}

// EnsurePersistenceLatency pre-creates a histogram for the supplied labels.
func (r *Registry) EnsurePersistenceLatency(operation, outcome string) {
	operation = normalizeLabel(operation)
	outcome = normalizeLabel(outcome)
	if operation == "" || outcome == "" {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	key := [2]string{operation, outcome}
	if _, ok := r.persistenceLatency[key]; !ok {
		r.persistenceLatency[key] = newValueHistogram(persistenceLatencyBuckets)
	}
}

// RecordPersistenceLatency records the latency of a persistence operation.
func (r *Registry) RecordPersistenceLatency(operation, outcome string, duration time.Duration) {
	operation = normalizeLabel(operation)
	outcome = normalizeLabel(outcome)
	if operation == "" || duration < 0 {
		return
	}
	if outcome == "" {
		outcome = "ok"
	}
	ms := duration.Seconds() * 1000
	if ms < 0 {
		ms = 0
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	key := [2]string{operation, outcome}
	hist, ok := r.persistenceLatency[key]
	if !ok {
		hist = newValueHistogram(persistenceLatencyBuckets)
		r.persistenceLatency[key] = hist
	}
	hist.observe(ms)
}

// RecordPersistenceEviction increments counters for persistence evictions.
func (r *Registry) RecordPersistenceEviction(kind string, bytes int64) {
	kind = normalizeLabel(kind)
	if kind == "" {
		kind = "unknown"
	}
	if bytes < 0 {
		bytes = 0
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.persistenceEvictions[kind]++
	r.persistenceBytes[kind] += uint64(bytes)
}

// RecordKVQuotaExceeded increments KV quota exceeded counter for a namespace.
func (r *Registry) RecordKVQuotaExceeded(namespace string) {
	namespace = normalizeLabel(namespace)
	if namespace == "" {
		namespace = "unknown"
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.kvQuotaExceeded[namespace]++
}

// RecordArtifactWriteFailed increments artifact write failure counter by reason.
func (r *Registry) RecordArtifactWriteFailed(reason string) {
	reason = normalizeLabel(reason)
	if reason == "" {
		reason = "unknown"
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.artifactWriteFailed[reason]++
}

// RecordIdempotencyLookup increments idempotency lookup counters by outcome.
func (r *Registry) RecordIdempotencyLookup(outcome string) {
	outcome = normalizeLabel(outcome)
	if outcome == "" {
		outcome = "error"
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.idempotencyLookups[outcome]++
}

// RecordIdempotencyReplay increments idempotency replay counter.
func (r *Registry) RecordIdempotencyReplay() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.idempotencyReplays++
}

// RecordIdempotencyConflict increments idempotency conflict counter.
func (r *Registry) RecordIdempotencyConflict() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.idempotencyConflicts++
}

// RecordIdempotencyInFlight increments idempotency in-flight counter.
func (r *Registry) RecordIdempotencyInFlight() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.idempotencyInFlight++
}

// RecordSSEActiveDelta adjusts the active SSE stream gauge for the provided transport.
func (r *Registry) RecordSSEActiveDelta(transport string, delta int64) {
	transport = normalizeLabel(transport)
	if transport == "" {
		transport = "unknown"
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.sseActive[transport] += delta
	if r.sseActive[transport] < 0 {
		r.sseActive[transport] = 0
	}
}

// RecordSSEStreamStart increments SSE stream start counter for a transport.
func (r *Registry) RecordSSEStreamStart(transport string) {
	transport = normalizeLabel(transport)
	if transport == "" {
		transport = "unknown"
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.sseStreamStarts[transport]++
}

// RecordSSEStreamEnd increments SSE stream end counter for a transport.
func (r *Registry) RecordSSEStreamEnd(transport string) {
	transport = normalizeLabel(transport)
	if transport == "" {
		transport = "unknown"
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.sseStreamEnds[transport]++
}

// RecordSSEResumeAttempt increments the SSE resume counter.
func (r *Registry) RecordSSEResumeAttempt() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.sseResumeTotal++
}

// RecordSSECursorExpired increments the SSE cursor expired counter.
func (r *Registry) RecordSSECursorExpired() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.sseCursorExpiredTotal++
}

// RecordRunStarted increments run started counter.
func (r *Registry) RecordRunStarted() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.runStartedTotal++
}

// RecordRunFinished increments run finished counter by status.
func (r *Registry) RecordRunFinished(status string) {
	status = normalizeLabel(status)
	if status == "" {
		status = "unknown"
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.runFinishedTotals[status]++
}

// RecordSecretHandleCreated increments secret handle creation counters.
func (r *Registry) RecordSecretHandleCreated(count int) {
	if count <= 0 {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.secretHandlesCreated += uint64(count)
}

// RecordSecretHandleCleanup increments secret handle cleanup counters.
func (r *Registry) RecordSecretHandleCleanup(count int, success bool) {
	if count <= 0 {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.secretHandlesCleaned += uint64(count)
	if !success {
		r.secretHandleErrors++
	}
}

// RecordSecretContainerRejected increments container secret rejection counter.
func (r *Registry) RecordSecretContainerRejected() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.secretContainerReject++
}

func normalizeLabel(v string) string {
	v = strings.TrimSpace(strings.ToLower(v))
	return v
}

var persistenceLatencyBuckets = []float64{1, 2, 5, 10, 25, 50, 100, 250, 500, 1000, 2500, 5000}

var persistenceLatencyDefaults = map[string][]string{
	"idempotency_lookup": {"hit", "miss", "expired", "error"},
	"idempotency_store":  {"ok", "error"},
	"journal_append":     {"ok", "quota_exceeded", "error"},
	"journal_read":       {"ok", "error"},
}

var persistenceDefaultKinds = []string{"journal", "idempotency"}

var idempotencyLookupOutcomes = []string{"hit", "miss", "expired", "error"}

type valueHistogram struct {
	buckets []float64
	counts  []uint64
	sum     float64
	count   uint64
}

func newValueHistogram(buckets []float64) *valueHistogram {
	return &valueHistogram{
		buckets: append([]float64(nil), buckets...),
		counts:  make([]uint64, len(buckets)),
	}
}

func (h *valueHistogram) observe(value float64) {
	if h == nil {
		return
	}
	if value < 0 {
		value = 0
	}
	for i, upper := range h.buckets {
		if value <= upper {
			h.counts[i]++
		}
	}
	h.count++
	h.sum += value
}

func (h *valueHistogram) writeWithLabels(buf *bufio.Writer, name string, labels map[string]string) {
	if h == nil {
		return
	}
	for i, upper := range h.buckets {
		fmt.Fprintf(buf, "%s_bucket%s %d\n", name, labelsWithLE(labels, upper), h.counts[i])
	}
	fmt.Fprintf(buf, "%s_bucket%s %d\n", name, labelsWithLE(labels, math.Inf(1)), h.count)
	fmt.Fprintf(buf, "%s_sum%s %g\n", name, labelsToString(labels), h.sum)
	fmt.Fprintf(buf, "%s_count%s %d\n\n", name, labelsToString(labels), h.count)
}
