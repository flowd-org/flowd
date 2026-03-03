package response

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNew_AppliesOptions(t *testing.T) {
	p := New(400, "bad", WithType("t"), WithDetail("d"), WithInstance("i"), WithExtension("code", "x"))

	if p.Status != 400 {
		t.Errorf("expected Status=400, got %d", p.Status)
	}
	if p.Title != "bad" {
		t.Errorf("expected Title='bad', got %q", p.Title)
	}
	if p.Type != "t" {
		t.Errorf("expected Type='t', got %q", p.Type)
	}
	if p.Detail != "d" {
		t.Errorf("expected Detail='d', got %q", p.Detail)
	}
	if p.Instance != "i" {
		t.Errorf("expected Instance='i', got %q", p.Instance)
	}
	if p.Ext == nil || p.Ext["code"] != "x" {
		t.Errorf("expected Ext['code']=='x', got %+v", p.Ext)
	}
}

func TestWrite_JSONShape_OmitsEmptyOptionalFields(t *testing.T) {
	p := Problem{Title: "oops", Status: 418}
	rec := httptest.NewRecorder()
	Write(rec, p)

	var body map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode JSON: %v", err)
	}

	if _, ok := body["title"]; !ok {
		t.Error("expected 'title' key in response")
	}
	if _, ok := body["status"]; !ok {
		t.Error("expected 'status' key in response")
	}
	if _, ok := body["type"]; ok {
		t.Error("unexpected 'type' key when empty")
	}
	if _, ok := body["detail"]; ok {
		t.Error("unexpected 'detail' key when empty")
	}
	if _, ok := body["instance"]; ok {
		t.Error("unexpected 'instance' key when empty")
	}
}

func TestWrite_IncludesTypeDetailInstanceAndExtensions(t *testing.T) {
	p := New(400, "bad", WithType("t"), WithDetail("d"), WithInstance("i"), WithExtension("code", "x"))
	rec := httptest.NewRecorder()
	Write(rec, p)

	var body map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode JSON: %v", err)
	}

	if body["title"] != "bad" {
		t.Errorf("expected title='bad', got %v", body["title"])
	}
	if body["status"].(float64) != 400 {
		t.Errorf("expected status=400, got %v", body["status"])
	}
	if body["type"] != "t" {
		t.Errorf("expected type='t', got %v", body["type"])
	}
	if body["detail"] != "d" {
		t.Errorf("expected detail='d', got %v", body["detail"])
	}
	if body["instance"] != "i" {
		t.Errorf("expected instance='i', got %v", body["instance"])
	}
	if ext, ok := body["code"]; !ok || ext != "x" {
		t.Errorf("expected code='x' in extensions, got %+v", body)
	}
}

func TestWrite_ExtensionCollisionPanics(t *testing.T) {
	p := New(400, "bad", WithExtension("title", "x"))
	rec := httptest.NewRecorder()

	defer func() {
		if recover() == nil {
			t.Fatal("expected panic on extension collision")
		}
	}()
	Write(rec, p)
}

func TestWrite_DefaultStatus500WhenZero(t *testing.T) {
	p := Problem{Title: "x"}
	rec := httptest.NewRecorder()
	Write(rec, p)

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("expected status 500 when zero, got %d", rec.Code)
	}
}
