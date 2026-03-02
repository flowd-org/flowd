package harness

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClientDo_ResponseBodyReadableAfterReturn(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("test response body"))
	}))
	defer server.Close()

	c := &Client{
		BaseURL: server.URL,
		Token:   "test-token",
		HTTP:    &http.Client{},
	}

	req, err := c.NewRequest(context.Background(), http.MethodGet, "/test", nil)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}

	resp, bodyBytes, err := c.Do(req)
	if err != nil {
		t.Fatalf("Do failed: %v", err)
	}
	defer resp.Body.Close()

	// Assert that we can read the response body after Do returns
	readBody, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("failed to read resp.Body after Do returned: %v", err)
	}

	// The body should match what was returned in bodyBytes
	if !bytes.Equal(readBody, bodyBytes) {
		t.Errorf("resp.Body after Do returns different content than bodyBytes\n"+
			"bodyBytes: %q\nreadBody:  %q", bodyBytes, readBody)
	}

	// Verify the expected content
	expected := "test response body"
	if string(bodyBytes) != expected {
		t.Errorf("expected bodyBytes to be %q, got %q", expected, string(bodyBytes))
	}
}

func TestClientDo_TruncatesAndPreservesReadableBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		// Write more than maxBodyRead bytes
		extraBytes := make([]byte, 50)
		w.Write(append(make([]byte, maxBodyRead+25), extraBytes...))
	}))
	defer server.Close()

	c := &Client{
		BaseURL: server.URL,
		Token:   "test-token",
		HTTP:    &http.Client{},
	}

	req, err := c.NewRequest(context.Background(), http.MethodGet, "/test", nil)
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}

	resp, bodyBytes, err := c.Do(req)
	if err != nil {
		t.Fatalf("Do failed: %v", err)
	}
	defer resp.Body.Close()

	// Assert that bodyBytes is truncated to maxBodyRead
	if len(bodyBytes) != maxBodyRead {
		t.Errorf("expected bodyBytes length %d, got %d", maxBodyRead, len(bodyBytes))
	}

	// Assert that we can still read resp.Body after Do returns
	readBody, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("failed to read resp.Body after Do returned: %v", err)
	}

	// The body from resp.Body should match the truncated bodyBytes
	if !bytes.Equal(readBody, bodyBytes) {
		t.Errorf("resp.Body after Do returns different content than bodyBytes\n"+
			"bodyBytes len: %d\nreadBody len:  %d", len(bodyBytes), len(readBody))
	}

	// Verify resp.Body also has the same truncated length
	if len(readBody) != maxBodyRead {
		t.Errorf("expected readBody length %d, got %d", maxBodyRead, len(readBody))
	}
}
