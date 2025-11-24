package runstore

import (
	"sort"
	"sync"
	"time"
)

// Run represents the persisted metadata for a run.
type Run struct {
	ID         string         `json:"id"`
	JobID      string         `json:"job_id"`
	Status     string         `json:"status"`
	StartedAt  time.Time      `json:"started_at"`
	FinishedAt *time.Time     `json:"finished_at,omitempty"`
	Result     map[string]any `json:"result,omitempty"`
	Executor   string         `json:"executor,omitempty"`
	Runtime    string         `json:"runtime,omitempty"`
	Provenance map[string]any `json:"provenance,omitempty"`
}

// Store keeps runs in memory for serve mode.
type Store struct {
	mu   sync.RWMutex
	runs map[string]Run
}

// New returns an empty run store.
func New() *Store {
	return &Store{
		runs: make(map[string]Run),
	}
}

// Create inserts or replaces a run.
func (s *Store) Create(run Run) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.runs[run.ID] = run
}

// Update replaces the stored run if it exists.
func (s *Store) Update(run Run) {
	s.Create(run)
}

// Get retrieves a run by ID.
func (s *Store) Get(id string) (Run, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	run, ok := s.runs[id]
	return run, ok
}

// List returns runs sorted by StartedAt descending.
func (s *Store) List() []Run {
	s.mu.RLock()
	defer s.mu.RUnlock()

	out := make([]Run, 0, len(s.runs))
	for _, run := range s.runs {
		out = append(out, run)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].StartedAt.After(out[j].StartedAt)
	})
	return out
}
