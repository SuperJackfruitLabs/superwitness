package verdicts

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"

	"github.com/jackc/pgx/v5"
)

var ErrRubricNotFound = errors.New("rubric not found")

func sortRubrics(rs []Rubric) {
	sort.Slice(rs, func(i, j int) bool {
		if rs[i].ID != rs[j].ID {
			return rs[i].ID < rs[j].ID
		}
		return rs[i].Version > rs[j].Version
	})
}

func (m *MemStore) ListRubrics(_ context.Context) ([]Rubric, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.Fail != nil {
		return nil, m.Fail
	}
	out := make([]Rubric, 0, len(m.rubrics))
	for _, r := range m.rubrics {
		out = append(out, r)
	}
	sortRubrics(out)
	return out, nil
}

func (m *MemStore) GetRubric(_ context.Context, id string, version int) (Rubric, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.Fail != nil {
		return Rubric{}, m.Fail
	}
	r, ok := m.rubrics[fmt.Sprintf("%s@%d", id, version)]
	if !ok {
		return Rubric{}, ErrRubricNotFound
	}
	return r, nil
}

const rubricCols = `id, version, name, scale, body, created_by, created_at`

func scanRubric(row pgx.Row) (Rubric, error) {
	var r Rubric
	var scale []byte
	if err := row.Scan(&r.ID, &r.Version, &r.Name, &scale, &r.Body, &r.CreatedBy, &r.CreatedAt); err != nil {
		return Rubric{}, err
	}
	r.Scale, r.CreatedAt = canonical(scale), r.CreatedAt.UTC()
	return r, nil
}

// canonical re-encodes jsonb text compactly. Key order is Postgres's own, so the API never
// promises a scale's original bytes, only its meaning.
func canonical(raw []byte) json.RawMessage {
	var buf bytes.Buffer
	if json.Compact(&buf, raw) != nil {
		return raw
	}
	return buf.Bytes()
}

func (s *PGStore) ListRubrics(ctx context.Context) ([]Rubric, error) {
	rows, err := s.Pool.Query(ctx, `SELECT `+rubricCols+` FROM rubrics ORDER BY id, version DESC`)
	if err != nil {
		return nil, unavailable(err)
	}
	defer rows.Close()
	out := []Rubric{}
	for rows.Next() {
		r, err := scanRubric(rows)
		if err != nil {
			return nil, unavailable(err)
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, unavailable(err)
	}
	return out, nil
}

func (s *PGStore) GetRubric(ctx context.Context, id string, version int) (Rubric, error) {
	r, err := scanRubric(s.Pool.QueryRow(ctx, `SELECT `+rubricCols+` FROM rubrics WHERE id = $1 AND version = $2`, id, version))
	if errors.Is(err, pgx.ErrNoRows) {
		return Rubric{}, ErrRubricNotFound
	}
	if err != nil {
		return Rubric{}, unavailable(err)
	}
	return r, nil
}

func (g *Gated) ListRubrics(ctx context.Context) ([]Rubric, error) {
	if !g.Ready() {
		return nil, ErrUnavailable
	}
	rr, ok := g.Inner.(RubricReader)
	if !ok {
		return nil, ErrUnavailable
	}
	return rr.ListRubrics(ctx)
}

func (g *Gated) GetRubric(ctx context.Context, id string, version int) (Rubric, error) {
	if !g.Ready() {
		return Rubric{}, ErrUnavailable
	}
	rr, ok := g.Inner.(RubricReader)
	if !ok {
		return Rubric{}, ErrUnavailable
	}
	return rr.GetRubric(ctx, id, version)
}
