package response

import (
	"encoding/json"
	"fmt"
	"net/http"
)

// Problem represents an RFC7807 problem response with optional custom extensions.
type Problem struct {
	Type     string
	Title    string
	Status   int
	Detail   string
	Instance string
	Ext      map[string]any
}

// Option configures a Problem instance.
type Option func(*Problem)

// WithType sets the problem type URI.
func WithType(t string) Option {
	return func(p *Problem) {
		p.Type = t
	}
}

// WithDetail sets the human-readable detail string.
func WithDetail(detail string) Option {
	return func(p *Problem) {
		p.Detail = detail
	}
}

// WithInstance sets the instance URI for the problem detail.
func WithInstance(instance string) Option {
	return func(p *Problem) {
		p.Instance = instance
	}
}

// WithExtension attaches an arbitrary RFC7807 extension field.
func WithExtension(key string, value any) Option {
	return func(p *Problem) {
		if p.Ext == nil {
			p.Ext = map[string]any{}
		}
		p.Ext[key] = value
	}
}

// New constructs a Problem and applies the provided options.
func New(status int, title string, opts ...Option) Problem {
	p := Problem{
		Status: status,
		Title:  title,
	}
	for _, opt := range opts {
		opt(&p)
	}
	return p
}

// Write serializes and writes the problem response with appropriate headers.
func Write(w http.ResponseWriter, p Problem) {
	if p.Status == 0 {
		p.Status = http.StatusInternalServerError
	}
	body := map[string]any{
		"title":  p.Title,
		"status": p.Status,
	}
	if p.Type != "" {
		body["type"] = p.Type
	}
	if p.Detail != "" {
		body["detail"] = p.Detail
	}
	if p.Instance != "" {
		body["instance"] = p.Instance
	}
	for k, v := range p.Ext {
		if _, exists := body[k]; exists {
			panic(fmt.Sprintf("problem extension %q collides with base field", k))
		}
		body[k] = v
	}
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(p.Status)
	_ = json.NewEncoder(w).Encode(body)
}
