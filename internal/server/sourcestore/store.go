package sourcestore

import (
	"sort"
	"sync"

	"github.com/flowd-org/flowd/internal/types"
)

// Source represents a configured source entry.
type Source struct {
	Name             string               `json:"name"`
	Type             string               `json:"type"`
	Ref              string               `json:"ref,omitempty"`
	ResolvedRef      string               `json:"resolved_ref,omitempty"`
	ResolvedCommit   string               `json:"resolved_commit,omitempty"`
	URL              string               `json:"url,omitempty"`
	Trust            map[string]any       `json:"trust,omitempty"`
	Aliases          []types.CommandAlias `json:"aliases,omitempty"`
	Metadata         map[string]any       `json:"metadata,omitempty"`
	LocalPath        string               `json:"-"`
	Digest           string               `json:"digest,omitempty"`
	PullPolicy       string               `json:"pull_policy,omitempty"`
	VerifySignatures bool                 `json:"verify_signatures,omitempty"`
	Provenance       map[string]any       `json:"provenance,omitempty"`
	Expose           string               `json:"expose,omitempty"`
}

// Store keeps sources in memory for the API lifetime.
type Store struct {
	mu      sync.RWMutex
	sources map[string]Source
}

// New returns an empty sources store.
func New() *Store {
	return &Store{
		sources: make(map[string]Source),
	}
}

// List returns all sources in lexical order of their keys.
func (s *Store) List() []Source {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Source, 0, len(s.sources))
	keys := make([]string, 0, len(s.sources))
	for name := range s.sources {
		keys = append(keys, name)
	}
	sort.Strings(keys)
	for _, name := range keys {
		out = append(out, s.sources[name])
	}
	return out
}

// Get retrieves a source by name.
func (s *Store) Get(name string) (Source, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	src, ok := s.sources[name]
	return src, ok
}

// Upsert inserts or updates the source; returns true if it was newly created.
func (s *Store) Upsert(src Source) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, exists := s.sources[src.Name]
	s.sources[src.Name] = src
	return !exists
}

// Delete removes a source by name and returns true if it existed.
func (s *Store) Delete(name string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.sources[name]; !exists {
		return false
	}
	delete(s.sources, name)
	return true
}
