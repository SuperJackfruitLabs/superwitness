package auth

import (
	"context"
	"sync"
)

// StaticTokenSource returns one fixed token, or Err. Adapter tests use it.
type StaticTokenSource struct {
	Value string
	Err   error

	mu            sync.Mutex
	calls         int
	invalidations int
}

func (s *StaticTokenSource) Token(context.Context) (string, error) {
	s.mu.Lock()
	s.calls++
	s.mu.Unlock()
	return s.Value, s.Err
}

func (s *StaticTokenSource) Invalidate() {
	s.mu.Lock()
	s.invalidations++
	s.mu.Unlock()
}

func (s *StaticTokenSource) Calls() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.calls
}

func (s *StaticTokenSource) Invalidations() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.invalidations
}
