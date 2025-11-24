package handlers

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/flowd-org/flowd/internal/coredb"
)

func TestKVHandlerPutGetDelete(t *testing.T) {
	ctx := context.Background()
	store := coredb.NewRuleYStore(openTestDB(t))
	h := NewKVHandler(KVConfig{
		Store: store,
		Allowlist: map[string]KVNamespaceConfig{
			"core_triggers": {},
		},
	})

	putBody := map[string]string{"value": base64.StdEncoding.EncodeToString([]byte("bar"))}
	buf, _ := json.Marshal(putBody)
	putReq := httptest.NewRequest(http.MethodPut, "/kv/core_triggers/foo", bytes.NewReader(buf)).WithContext(ctx)
	putReq.Header.Set("Content-Type", "application/json")
	putResp := httptest.NewRecorder()
	h.ServeHTTP(putResp, putReq)
	if putResp.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", putResp.Code)
	}

	getResp := httptest.NewRecorder()
	h.ServeHTTP(getResp, httptest.NewRequest(http.MethodGet, "/kv/core_triggers/foo", nil).WithContext(ctx))
	if getResp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", getResp.Code)
	}
	var payload map[string]any
	if err := json.NewDecoder(getResp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode get body: %v", err)
	}
	if payload["key"] != "foo" {
		t.Fatalf("expected key foo, got %v", payload["key"])
	}
	if payload["value"] != base64.StdEncoding.EncodeToString([]byte("bar")) {
		t.Fatalf("unexpected value %v", payload["value"])
	}

	delResp := httptest.NewRecorder()
	h.ServeHTTP(delResp, httptest.NewRequest(http.MethodDelete, "/kv/core_triggers/foo", nil).WithContext(ctx))
	if delResp.Code != http.StatusNoContent {
		t.Fatalf("expected 204 delete, got %d", delResp.Code)
	}
}

func TestKVHandlerScan(t *testing.T) {
	ctx := context.Background()
	store := coredb.NewRuleYStore(openTestDB(t))
	h := NewKVHandler(KVConfig{
		Store: store,
		Allowlist: map[string]KVNamespaceConfig{
			"core_triggers": {},
		},
	})

	keys := []string{"app:one", "app:two", "app:three", "bee:one"}
	for _, k := range keys {
		putBody := map[string]string{"value": base64.StdEncoding.EncodeToString([]byte("value:" + k))}
		buf, _ := json.Marshal(putBody)
		req := httptest.NewRequest(http.MethodPut, "/kv/core_triggers/"+k, bytes.NewReader(buf)).WithContext(ctx)
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusNoContent {
			t.Fatalf("put %s failed: %d", k, rec.Code)
		}
	}

	scanReq := httptest.NewRequest(http.MethodGet, "/kv/core_triggers?prefix=app:", nil).WithContext(ctx)
	resp := httptest.NewRecorder()
	h.ServeHTTP(resp, scanReq)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.Code)
	}
	var body struct {
		Items      []map[string]any `json:"items"`
		NextCursor string           `json:"nextCursor"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body.Items) != 3 {
		t.Fatalf("expected 3 app items, got %d", len(body.Items))
	}
	if body.NextCursor != "" {
		if _, err := base64.StdEncoding.DecodeString(body.NextCursor); err != nil {
			t.Fatalf("invalid cursor encoding: %v", err)
		}
	}
}

func TestKVHandlerNamespaceForbidden(t *testing.T) {
	ctx := context.Background()
	h := NewKVHandler(KVConfig{
		Store: coredb.NewRuleYStore(openTestDB(t)),
		Allowlist: map[string]KVNamespaceConfig{
			"core_triggers": {},
		},
	})

	req := httptest.NewRequest(http.MethodPut, "/kv/forbidden/key", bytes.NewReader([]byte(`{"value":""}`))).WithContext(ctx)
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()
	h.ServeHTTP(resp, req)
	if resp.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", resp.Code)
	}
	var prob map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&prob)
	if prob["type"] != namespaceForbiddenProblemType {
		t.Fatalf("expected problem type %s, got %v", namespaceForbiddenProblemType, prob["type"])
	}
}

func TestKVHandlerQuotaExceeded(t *testing.T) {
	ctx := context.Background()
	store := coredb.NewRuleYStore(openTestDB(t))
	h := NewKVHandler(KVConfig{
		Store: store,
		Allowlist: map[string]KVNamespaceConfig{
			"core_triggers": {LimitBytes: 9},
		},
	})

	value := base64.StdEncoding.EncodeToString([]byte("1234"))
	body := map[string]string{"value": value}
	buf, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPut, "/kv/core_triggers/a", bytes.NewReader(buf)).WithContext(ctx)
	req.Header.Set("Content-Type", "application/json")
	resp := httptest.NewRecorder()
	h.ServeHTTP(resp, req)
	if resp.Code != http.StatusNoContent {
		t.Fatalf("initial put failed: %d", resp.Code)
	}

	req2 := httptest.NewRequest(http.MethodPut, "/kv/core_triggers/b", bytes.NewReader(buf)).WithContext(ctx)
	req2.Header.Set("Content-Type", "application/json")
	resp2 := httptest.NewRecorder()
	h.ServeHTTP(resp2, req2)
	if resp2.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429, got %d", resp2.Code)
	}
}

func openTestDB(t *testing.T) *coredb.DB {
	t.Helper()
	dir := t.TempDir()
	db, err := coredb.Open(context.Background(), coredb.Options{DataDir: dir})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}
