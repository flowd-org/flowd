package scenarios

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/flowd-org/flowd/conformance/internal/harness"
)

func TestValidateCollision_ReusesIdempotencyKey(t *testing.T) {
	var mu sync.Mutex
	var idemKeys []string
	var idemSHAs []string
	callCount := 0

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/runs" {
			w.WriteHeader(http.StatusNotFound)
			return
		}

		mu.Lock()
		callCount++
		idemKeys = append(idemKeys, r.Header.Get("Idempotency-Key"))
		idemSHAs = append(idemSHAs, r.Header.Get("Idempotency-SHA256"))
		curr := callCount
		mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		if curr == 1 {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"id":"run-1"}`))
			return
		}

		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(`{"error":"Conflict"}`))
	}))
	defer server.Close()

	env := Env{
		BaseURL: server.URL,
		Token:   "test-token",
		HTTPClient: &harness.Client{
			BaseURL: server.URL,
			Token:   "test-token",
			HTTP:    &http.Client{},
		},
	}

	result := validateCollision(context.Background(), env, "ulc.shell.bash")
	if !result.Passed {
		t.Fatalf("validateCollision() expected pass, got failure: %+v", result.Failure)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(idemKeys) != 2 {
		t.Fatalf("expected 2 /runs calls, got %d", len(idemKeys))
	}
	if idemKeys[0] == "" || idemSHAs[0] == "" {
		t.Fatalf("expected idempotency headers to be populated")
	}
	if idemKeys[0] != idemKeys[1] {
		t.Fatalf("expected Idempotency-Key to be reused, got %q and %q", idemKeys[0], idemKeys[1])
	}
	if idemSHAs[0] != idemSHAs[1] {
		t.Fatalf("expected Idempotency-SHA256 to be reused, got %q and %q", idemSHAs[0], idemSHAs[1])
	}
}

func TestValidateCollision_PassesOnSameRunID(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/runs" {
			w.WriteHeader(http.StatusNotFound)
			return
		}

		callCount++
		w.Header().Set("Content-Type", "application/json")
		if callCount == 1 {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"id":"run-1"}`))
			return
		}

		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"run-1"}`))
	}))
	defer server.Close()

	env := Env{
		BaseURL: server.URL,
		Token:   "test-token",
		HTTPClient: &harness.Client{
			BaseURL: server.URL,
			Token:   "test-token",
			HTTP:    &http.Client{},
		},
	}

	result := validateCollision(context.Background(), env, "ulc.shell.bash")
	if !result.Passed {
		t.Fatalf("validateCollision() expected pass when run IDs match, got failure: %+v", result.Failure)
	}
}

func TestValidateCollision_PassesOnConflict409(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/runs" {
			w.WriteHeader(http.StatusNotFound)
			return
		}

		callCount++
		w.Header().Set("Content-Type", "application/json")
		if callCount == 1 {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"id":"run-1"}`))
			return
		}

		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(`{"error":"Conflict"}`))
	}))
	defer server.Close()

	env := Env{
		BaseURL: server.URL,
		Token:   "test-token",
		HTTPClient: &harness.Client{
			BaseURL: server.URL,
			Token:   "test-token",
			HTTP:    &http.Client{},
		},
	}

	result := validateCollision(context.Background(), env, "ulc.shell.bash")
	if !result.Passed {
		t.Fatalf("validateCollision() expected pass on 409 conflict, got failure: %+v", result.Failure)
	}
}
