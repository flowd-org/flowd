package handlers

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/flowd-org/flowd/internal/coredb"
	"github.com/flowd-org/flowd/internal/server/response"
)

const namespaceForbiddenProblemType = "https://flowd.dev/problems/namespace-forbidden"

// KVNamespaceConfig controls namespace-specific Rule-Y behaviour.
type KVNamespaceConfig struct {
	LimitBytes int64
}

// KVConfig configures the KV handler.
type KVConfig struct {
	Store     *coredb.RuleYStore
	Allowlist map[string]KVNamespaceConfig
}

// NewKVHandler returns an HTTP handler that exposes the Rule-Y key/value surface.
func NewKVHandler(cfg KVConfig) http.Handler {
	allow := make(map[string]KVNamespaceConfig)
	for ns, c := range cfg.Allowlist {
		allow[strings.ToLower(ns)] = c
	}
	return &kvHandler{
		store:     cfg.Store,
		allowlist: allow,
	}
}

type kvHandler struct {
	store     *coredb.RuleYStore
	allowlist map[string]KVNamespaceConfig
}

func (h *kvHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodPut && r.Method != http.MethodDelete {
		response.Write(w, response.New(http.StatusMethodNotAllowed, "method not allowed"))
		return
	}

	namespace, keyPath, err := parseKVPath(r.URL.Path)
	if err != nil {
		response.Write(w, response.New(http.StatusBadRequest, "invalid path", response.WithDetail(err.Error())))
		return
	}

	cfg, ok := h.allowlist[strings.ToLower(namespace)]
	if !ok {
		response.Write(w, namespaceForbiddenProblem())
		return
	}

	switch r.Method {
	case http.MethodPut:
		h.handlePut(w, r, namespace, keyPath, cfg.LimitBytes)
	case http.MethodDelete:
		h.handleDelete(w, r, namespace, keyPath)
	case http.MethodGet:
		if keyPath == "" {
			h.handleScan(w, r, namespace, cfg.LimitBytes)
			return
		}
		h.handleGet(w, r, namespace, keyPath)
	}
}

func (h *kvHandler) handlePut(w http.ResponseWriter, r *http.Request, namespace, key string, limit int64) {
	if key == "" {
		response.Write(w, response.New(http.StatusBadRequest, "key required"))
		return
	}
	var payload struct {
		Value string `json:"value"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		response.Write(w, response.New(http.StatusBadRequest, "invalid JSON body", response.WithDetail(err.Error())))
		return
	}
	value, err := base64.StdEncoding.DecodeString(payload.Value)
	if err != nil {
		response.Write(w, response.New(http.StatusBadRequest, "value must be base64", response.WithDetail(err.Error())))
		return
	}

	if err := h.store.Put(r.Context(), namespace, []byte(key), value, limit); err != nil {
		h.writeStoreError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *kvHandler) handleGet(w http.ResponseWriter, r *http.Request, namespace, key string) {
	if key == "" {
		response.Write(w, response.New(http.StatusBadRequest, "key required"))
		return
	}
	value, ts, found, err := h.store.Get(r.Context(), namespace, []byte(key))
	if err != nil {
		h.writeStoreError(w, err)
		return
	}
	if !found {
		response.Write(w, response.New(http.StatusNotFound, "key not found"))
		return
	}

	resp := map[string]any{
		"key":       key,
		"value":     base64.StdEncoding.EncodeToString(value),
		"timestamp": ts.Format(time.RFC3339Nano),
	}
	writeJSON(w, resp, http.StatusOK)
}

func (h *kvHandler) handleDelete(w http.ResponseWriter, r *http.Request, namespace, key string) {
	if key == "" {
		response.Write(w, response.New(http.StatusBadRequest, "key required"))
		return
	}
	deleted, err := h.store.Delete(r.Context(), namespace, []byte(key))
	if err != nil {
		h.writeStoreError(w, err)
		return
	}
	if !deleted {
		response.Write(w, response.New(http.StatusNotFound, "key not found"))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *kvHandler) handleScan(w http.ResponseWriter, r *http.Request, namespace string, _ int64) {
	q := r.URL.Query()
	prefix := []byte(q.Get("prefix"))

	var cursor []byte
	if c := q.Get("cursor"); c != "" {
		decoded, err := base64.StdEncoding.DecodeString(c)
		if err != nil {
			response.Write(w, response.New(http.StatusBadRequest, "cursor must be base64", response.WithDetail(err.Error())))
			return
		}
		cursor = decoded
	}

	limit := 256
	if v := q.Get("limit"); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil && parsed > 0 {
			limit = parsed
		}
	}

	items, nextCursor, err := h.store.Scan(r.Context(), namespace, prefix, cursor, limit)
	if err != nil {
		h.writeStoreError(w, err)
		return
	}

	resp := map[string]any{}
	encoded := make([]map[string]any, len(items))
	for i, item := range items {
		encoded[i] = map[string]any{
			"key":       string(item.Key),
			"value":     base64.StdEncoding.EncodeToString(item.Value),
			"timestamp": item.Timestamp.Format(time.RFC3339Nano),
		}
	}
	resp["items"] = encoded
	if len(nextCursor) > 0 {
		resp["nextCursor"] = base64.StdEncoding.EncodeToString(nextCursor)
	}
	writeJSON(w, resp, http.StatusOK)
}

func (h *kvHandler) writeStoreError(w http.ResponseWriter, err error) {
	if err == nil {
		return
	}
	switch {
	case errors.Is(err, coredb.ErrRuleYUnavailable):
		response.Write(w, response.New(http.StatusServiceUnavailable, "storage unavailable"))
	case errors.Is(err, coredb.ErrRuleYNamespaceQuota) || coredb.IsQuotaExceeded(err):
		response.Write(w, storageQuotaExceededProblem())
	case errors.Is(err, coredb.ErrRuleYKeyTooLarge):
		response.Write(w, response.New(http.StatusBadRequest, "key exceeds maximum length"))
	case errors.Is(err, coredb.ErrRuleYValueTooLarge):
		response.Write(w, response.New(http.StatusBadRequest, "value exceeds maximum length"))
	default:
		response.Write(w, response.New(http.StatusInternalServerError, "kv operation failed", response.WithDetail(err.Error())))
	}
}

func namespaceForbiddenProblem() response.Problem {
	return response.New(http.StatusForbidden, "namespace forbidden",
		response.WithType(namespaceForbiddenProblemType),
	)
}

func writeJSON(w http.ResponseWriter, payload any, status int) {
	data, err := json.Marshal(payload)
	if err != nil {
		response.Write(w, response.New(http.StatusInternalServerError, "encode response failed", response.WithDetail(err.Error())))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(data)
}

func parseKVPath(path string) (namespace string, key string, err error) {
	if !strings.HasPrefix(path, "/kv/") {
		return "", "", fmt.Errorf("invalid path %q", path)
	}
	rest := strings.TrimPrefix(path, "/kv/")
	if rest == "" {
		return "", "", fmt.Errorf("namespace required")
	}
	namespace, remainder, hasKey := strings.Cut(rest, "/")
	if !hasKey {
		return namespace, "", nil
	}
	return namespace, remainder, nil
}
