package session

import (
	"context"
	"sync"
	"time"
)

// MemStore mirrors PGStore in memory, for unit tests. Touches counts Touch calls. FailDelete
// fails only Delete, after Fail is checked. SetGrantFailures fails that many SetGrant calls.
type MemStore struct {
	Fail             error
	FailDelete       error
	SetGrantFailures int
	Touches          int

	mu   sync.Mutex
	rows map[string]Session
}

func NewMemStore() *MemStore { return &MemStore{rows: map[string]Session{}} }

func (m *MemStore) Create(_ context.Context, s Session) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.Fail != nil {
		return m.Fail
	}
	m.rows[string(s.IDHash)] = s
	return nil
}

func (m *MemStore) Get(_ context.Context, id []byte) (Session, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.Fail != nil {
		return Session{}, m.Fail
	}
	s, ok := m.rows[string(id)]
	if !ok {
		return Session{}, ErrNotFound
	}
	return s, nil
}

func (m *MemStore) Touch(_ context.Context, id []byte, at time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.Fail != nil {
		return m.Fail
	}
	m.Touches++
	if s, ok := m.rows[string(id)]; ok {
		s.LastSeenAt = at
		m.rows[string(id)] = s
	}
	return nil
}

func (m *MemStore) SetGrant(_ context.Context, id []byte, g PlaneGrant) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.Fail != nil {
		return m.Fail
	}
	if m.SetGrantFailures > 0 {
		m.SetGrantFailures--
		return ErrUnavailable
	}
	if s, ok := m.rows[string(id)]; ok {
		s.Grant = g
		m.rows[string(id)] = s
	}
	return nil
}

func (m *MemStore) Delete(_ context.Context, id []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.Fail != nil {
		return m.Fail
	}
	if m.FailDelete != nil {
		return m.FailDelete
	}
	delete(m.rows, string(id))
	return nil
}

func (m *MemStore) Sweep(_ context.Context, now time.Time) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.Fail != nil {
		return 0, m.Fail
	}
	var n int64
	for k, s := range m.rows {
		if s.Expired(now) {
			delete(m.rows, k)
			n++
		}
	}
	return n, nil
}

// Len is the number of stored sessions.
func (m *MemStore) Len() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.rows)
}
