package verdicts

import (
	"context"
	"sort"
)

// MaxHistory bounds one subject's history read.
const MaxHistory = 500

// HistoryReader reads every verdict on one subject, superseded ones included, oldest first.
type HistoryReader interface {
	ListSubject(ctx context.Context, subject SubjectKey) ([]Verdict, error)
}

func (m *MemStore) ListSubject(_ context.Context, k SubjectKey) ([]Verdict, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.Fail != nil {
		return nil, m.Fail
	}
	out := []Verdict{}
	for _, v := range m.byID {
		if v.SubjectKind == k.Kind && v.SubjectRef == k.Ref {
			out = append(out, v)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if !out[i].CreatedAt.Equal(out[j].CreatedAt) {
			return out[i].CreatedAt.Before(out[j].CreatedAt)
		}
		return out[i].ID < out[j].ID
	})
	if len(out) > MaxHistory {
		out = out[:MaxHistory]
	}
	return out, nil
}

func (s *PGStore) ListSubject(ctx context.Context, k SubjectKey) ([]Verdict, error) {
	rows, err := s.Pool.Query(ctx, `SELECT `+cols+` FROM verdicts WHERE subject_kind = $1 AND subject_ref = $2
		ORDER BY created_at, id LIMIT $3`, string(k.Kind), k.Ref, MaxHistory)
	if err != nil {
		return nil, unavailable(err)
	}
	defer rows.Close()
	out := []Verdict{}
	for rows.Next() {
		v, err := scanVerdict(rows)
		if err != nil {
			return nil, unavailable(err)
		}
		out = append(out, v)
	}
	if err := rows.Err(); err != nil {
		return nil, unavailable(err)
	}
	return out, nil
}

func (g *Gated) ListSubject(ctx context.Context, k SubjectKey) ([]Verdict, error) {
	if !g.Ready() {
		return nil, ErrUnavailable
	}
	hr, ok := g.Inner.(HistoryReader)
	if !ok {
		return nil, ErrUnavailable
	}
	return hr.ListSubject(ctx, k)
}
