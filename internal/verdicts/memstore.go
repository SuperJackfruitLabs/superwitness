package verdicts

import (
	"context"
	"fmt"
	"sort"
	"sync"
)

// MemStore mirrors PGStore's constraints in memory, for unit tests.
type MemStore struct {
	Fail error

	mu      sync.Mutex
	byID    map[string]Verdict
	byKey   map[string]string
	rubrics map[string]Rubric
}

func NewMemStore() *MemStore {
	return &MemStore{byID: map[string]Verdict{}, byKey: map[string]string{}, rubrics: map[string]Rubric{}}
}

func (m *MemStore) Insert(_ context.Context, v Verdict) (Verdict, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.Fail != nil {
		return Verdict{}, false, m.Fail
	}
	if id, ok := m.byKey[v.IdempotencyKey]; ok {
		return m.byID[id], false, nil
	}
	if v.Supersedes != nil {
		if _, ok := m.byID[*v.Supersedes]; !ok {
			return Verdict{}, false, fmt.Errorf("%w: supersedes unknown verdict %s", ErrUnavailable, *v.Supersedes)
		}
		for _, e := range m.byID {
			if e.Supersedes != nil && *e.Supersedes == *v.Supersedes {
				return Verdict{}, false, ErrAlreadySuperseded
			}
		}
	}
	m.byID[v.ID] = v
	m.byKey[v.IdempotencyKey] = v.ID
	return v, true, nil
}

func (m *MemStore) GetByKey(_ context.Context, key string) (Verdict, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.Fail != nil {
		return Verdict{}, m.Fail
	}
	id, ok := m.byKey[key]
	if !ok {
		return Verdict{}, ErrNotFound
	}
	return m.byID[id], nil
}

func (m *MemStore) Get(_ context.Context, id string) (Verdict, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.Fail != nil {
		return Verdict{}, m.Fail
	}
	v, ok := m.byID[id]
	if !ok {
		return Verdict{}, ErrNotFound
	}
	return v, nil
}

func (m *MemStore) HasSuccessor(_ context.Context, id string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.Fail != nil {
		return false, m.Fail
	}
	for _, e := range m.byID {
		if e.Supersedes != nil && *e.Supersedes == id {
			return true, nil
		}
	}
	return false, nil
}

func (m *MemStore) ListCurrent(_ context.Context, subjects []SubjectKey) ([]Verdict, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.Fail != nil {
		return nil, m.Fail
	}
	want := map[SubjectKey]bool{}
	for _, s := range subjects {
		want[s] = true
	}
	superseded := map[string]bool{}
	for _, e := range m.byID {
		if e.Supersedes != nil {
			superseded[*e.Supersedes] = true
		}
	}
	out := []Verdict{}
	for _, e := range m.byID {
		if want[SubjectKey{e.SubjectKind, e.SubjectRef}] && !superseded[e.ID] {
			out = append(out, e)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if !out[i].CreatedAt.Equal(out[j].CreatedAt) {
			return out[i].CreatedAt.Before(out[j].CreatedAt)
		}
		return out[i].ID < out[j].ID
	})
	return out, nil
}

func (m *MemStore) RubricExists(_ context.Context, id string, version int) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.Fail != nil {
		return false, m.Fail
	}
	_, ok := m.rubrics[fmt.Sprintf("%s@%d", id, version)]
	return ok, nil
}

func (m *MemStore) InsertRubric(_ context.Context, r Rubric) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.Fail != nil {
		return m.Fail
	}
	k := fmt.Sprintf("%s@%d", r.ID, r.Version)
	if _, ok := m.rubrics[k]; ok {
		return ErrRubricExists
	}
	m.rubrics[k] = r
	return nil
}
