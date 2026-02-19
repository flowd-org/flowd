package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/flowd-org/flowd/internal/server/buildinfo"
)

func TestCapabilitiesHandlerMethodNotAllowed(t *testing.T) {
	handler := NewCapabilitiesHandler(map[string]bool{"export": true})
	req := httptest.NewRequest(http.MethodPost, "/capabilities", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rec.Code)
	}
}

func TestCapabilitiesHandlerResponse(t *testing.T) {
	handler := NewCapabilitiesHandler(map[string]bool{"export": true})
	req := httptest.NewRequest(http.MethodGet, "/capabilities", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Fatalf("expected application/json, got %q", ct)
	}
	if cc := rec.Header().Get("Cache-Control"); cc != "no-store" {
		t.Fatalf("expected no-store cache header, got %q", cc)
	}

	var body struct {
		Core struct {
			Version     string `json:"version"`
			SpecVersion string `json:"spec_version"`
			AppID       string `json:"app_id"`
		} `json:"core"`
		Extensions []struct {
			Name     string `json:"name"`
			Version  string `json:"version"`
			Compiled bool   `json:"compiled"`
			Enabled  bool   `json:"enabled"`
		} `json:"extensions"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	var raw map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &raw); err != nil {
		t.Fatalf("decode raw body: %v", err)
	}
	if _, ok := raw["kv"]; ok {
		t.Fatal("capabilities must not advertise /kv")
	}

	if body.Core.SpecVersion != buildinfo.CoreSpecVersion {
		t.Fatalf("expected core.spec_version=%q, got %q", buildinfo.CoreSpecVersion, body.Core.SpecVersion)
	}
	if body.Core.AppID != buildinfo.CoreAppID {
		t.Fatalf("expected core.app_id=%q, got %q", buildinfo.CoreAppID, body.Core.AppID)
	}
	if len(body.Extensions) != 1 {
		t.Fatalf("expected one compiled extension, got %d", len(body.Extensions))
	}
	ext := body.Extensions[0]
	if ext.Name != "export" {
		t.Fatalf("expected extension name export, got %q", ext.Name)
	}
	if !ext.Compiled {
		t.Fatal("expected export extension to be compiled")
	}
	if !ext.Enabled {
		t.Fatal("expected export extension to be enabled")
	}
	if ext.Version != body.Core.Version {
		t.Fatalf("expected extension version to match core.version (%q), got %q", body.Core.Version, ext.Version)
	}
}
