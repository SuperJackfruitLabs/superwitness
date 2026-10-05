package runs

import (
	"context"
	"slices"
	"strings"
)

// Scope is one place runs belong to, such as a board, with the name its newest run reported.
type Scope struct {
	Source string  `json:"source"`
	ID     string  `json:"id"`
	Name   *string `json:"name"`
	Runs   int     `json:"runs"`
}

func (m *MemStore) Scopes(_ context.Context) ([]Scope, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.Fail != nil {
		return nil, m.Fail
	}
	type k struct{ source, id string }
	found := map[k]*Scope{}
	newest := map[k]Run{}
	for _, r := range m.rows {
		if r.ScopeID == nil {
			continue
		}
		key := k{r.Source, *r.ScopeID}
		s := found[key]
		if s == nil {
			s = &Scope{Source: r.Source, ID: *r.ScopeID}
			found[key] = s
		}
		s.Runs++
		if n, ok := newest[key]; !ok || r.UpdatedAt.After(n.UpdatedAt) {
			newest[key] = r
			s.Name = r.ScopeName
		}
	}
	out := make([]Scope, 0, len(found))
	for _, s := range found {
		out = append(out, *s)
	}
	slices.SortFunc(out, func(a, b Scope) int {
		if c := strings.Compare(a.Source, b.Source); c != 0 {
			return c
		}
		return strings.Compare(a.ID, b.ID)
	})
	return out, nil
}

func (s *PGStore) Scopes(ctx context.Context) ([]Scope, error) {
	if err := s.ready(); err != nil {
		return nil, err
	}
	rows, err := s.Pool.Query(ctx, `SELECT source, scope_id, (array_agg(scope_name ORDER BY updated_at DESC))[1], count(*)
		FROM runs WHERE scope_id IS NOT NULL GROUP BY source, scope_id ORDER BY source, scope_id COLLATE "C"`)
	if err != nil {
		return nil, unavailable(err)
	}
	defer rows.Close()
	out := []Scope{}
	for rows.Next() {
		var sc Scope
		if err := rows.Scan(&sc.Source, &sc.ID, &sc.Name, &sc.Runs); err != nil {
			return nil, unavailable(err)
		}
		out = append(out, sc)
	}
	if err := rows.Err(); err != nil {
		return nil, unavailable(err)
	}
	return out, nil
}
