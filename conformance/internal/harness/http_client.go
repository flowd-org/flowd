package harness

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
)

const maxBodyRead = 64 * 1024 // 64KiB

// Client wraps http.Client with automatic Authorization header and redaction-safe diagnostics.
type Client struct {
	BaseURL string
	Token   string
	HTTP    *http.Client
	Verbose bool
}

// NewRequest creates an HTTP request with the Authorization header set.
// The Authorization header is always set to "Bearer <token>".
func (c *Client) NewRequest(ctx context.Context, method, path string, body io.Reader) (*http.Request, error) {
	url := c.BaseURL + path
	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Always set Authorization header
	req.Header.Set("Authorization", "Bearer "+c.Token)

	// Set Accept header for JSON requests
	if method == http.MethodGet || method == http.MethodPost || method == http.MethodPut || method == http.MethodPatch {
		req.Header.Set("Accept", "application/json")
	}

	return req, nil
}

// Do executes the HTTP request and returns the response, body bytes, and error.
// The body is read and returned (bounded to maxBodyRead, truncated with marker).
// The response is returned even on non-2xx status codes to allow scenario assertions.
func (c *Client) Do(req *http.Request) (*http.Response, []byte, error) {
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, nil, fmt.Errorf("request failed: %w", err)
	}

	// Read body (bounded)
	var bodyBytes []byte
	if resp.Body != nil {
		raw := resp.Body
		bodyBytes, err = io.ReadAll(io.LimitReader(raw, maxBodyRead+1))
		_ = raw.Close()
		if err != nil {
			return nil, nil, fmt.Errorf("failed to read response body: %w", err)
		}
	}

	// Check if body was truncated
	truncated := len(bodyBytes) > maxBodyRead
	if truncated {
		bodyBytes = bodyBytes[:maxBodyRead]
	}

	// Replace resp.Body with a fresh reader so callers can read it after Do returns
	resp.Body = io.NopCloser(bytes.NewReader(bodyBytes))

	// In verbose mode, log the request and response (redacted)
	if c.Verbose {
		fmt.Printf("HTTP %s %s -> %d (truncated:%v)\n", req.Method, req.URL.Path, resp.StatusCode, truncated)
	}

	return resp, bodyBytes, nil
}

// FormatHTTPFailure formats a failure message for HTTP requests.
// It includes method, path, status code, and a truncated/redacted body excerpt.
// The raw token is never included in the output.
func FormatHTTPFailure(req *http.Request, resp *http.Response, body []byte) string {
	var buf bytes.Buffer

	buf.WriteString(fmt.Sprintf("HTTP %s %s failed", req.Method, req.URL.Path))
	buf.WriteString(fmt.Sprintf(" (status=%d)", resp.StatusCode))

	// Truncate body for display
	bodyStr := string(body)
	if len(bodyStr) > maxBodyRead {
		bodyStr = bodyStr[:maxBodyRead] + "...[truncated]"
	}

	// Redact any secrets in the body (including potential Authorization headers)
	redacted := RedactSecrets(bodyStr, req.Header.Get("Authorization"))

	// Include a small excerpt of the body (up to 256 chars after redaction)
	excerptLen := 256
	if len(redacted) > excerptLen {
		redacted = redacted[:excerptLen] + "...[truncated]"
	}

	if redacted != "" {
		buf.WriteString(fmt.Sprintf("\nResponse body: %s", redacted))
	}

	return buf.String()
}

// FormatHTTPFailureContext is a convenience wrapper that takes request details as strings.
func FormatHTTPFailureContext(method, path string, statusCode int, body []byte) string {
	// Create a minimal request for formatting
	reqURL, _ := url.Parse(path)
	req := &http.Request{
		Method: method,
		URL:    reqURL,
		Header: make(http.Header),
	}
	// Don't include the token in the request header for safety

	resp := &http.Response{
		StatusCode: statusCode,
	}

	return FormatHTTPFailure(req, resp, body)
}
