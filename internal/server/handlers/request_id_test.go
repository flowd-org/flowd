package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/flowd-org/flowd/internal/coredb"
	"github.com/flowd-org/flowd/internal/server/requestctx"
	"github.com/flowd-org/flowd/internal/server/runstore"
)

func TestRunsHandlerPersistsRequestID(t *testing.T) {
	root := t.TempDir()
	writeJobConfig(t, root, "demo", `
version: v1
job:
  id: demo
  name: Demo Job
`)

	db, err := coredb.Open(context.Background(), coredb.Options{DataDir: t.TempDir()})
	if err != nil {
		t.Fatalf("open coredb: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	store := runstore.New()
	h := NewRunsHandler(RunsConfig{
		Root:  root,
		Now:   func() time.Time { return time.Unix(0, 0).UTC() },
		Store: store,
		DB:    db,
	})

	requestID := "req-test-123"
	payload := `{"job_id":"demo"}`
	req := httptest.NewRequest(http.MethodPost, "/runs", strings.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", "aaaaaaaaaaaaaaaaaaaa")
	req = req.WithContext(requestctx.WithRequestID(req.Context(), requestID))

	resp := httptest.NewRecorder()
	h.ServeHTTP(resp, req)
	if resp.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", resp.Code)
	}

	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got := body["request_id"]; got != requestID {
		t.Fatalf("expected request_id %q, got %v", requestID, got)
	}

	runID, _ := body["id"].(string)
	if runID == "" {
		t.Fatalf("expected run id")
	}

	get := httptest.NewRecorder()
	NewRunGetHandler(RunGetConfig{Store: store, DB: db}).ServeHTTP(get, httptest.NewRequest(http.MethodGet, "/runs/"+runID, nil))
	if get.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", get.Code)
	}
	var getBody map[string]any
	if err := json.NewDecoder(get.Body).Decode(&getBody); err != nil {
		t.Fatalf("decode get response: %v", err)
	}
	if got := getBody["request_id"]; got != requestID {
		t.Fatalf("expected persisted request_id %q, got %v", requestID, got)
	}
}

func TestPlansHandlerEchoesRequestID(t *testing.T) {
	root := t.TempDir()
	writeJobConfig(t, root, "demo", `
version: v1
job:
  id: demo
  name: Demo Job
`)

	planHandler := NewPlansHandler(PlansConfig{Root: root})
	requestID := "req-plan-456"
	payload := `{"job_id":"demo"}`
	req := httptest.NewRequest(http.MethodPost, "/plans", strings.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(requestctx.WithRequestID(req.Context(), requestID))

	resp := httptest.NewRecorder()
	planHandler.ServeHTTP(resp, req)
	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.Code)
	}

	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got := body["request_id"]; got != requestID {
		t.Fatalf("expected request_id %q, got %v", requestID, got)
	}
}
