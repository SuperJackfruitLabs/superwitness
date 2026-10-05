package runs

import (
	"context"
	"maps"
	"slices"
	"sync"
	"time"

	"github.com/SuperJackfruitLabs/superwitness/internal/verdicts"
)

type key struct{ source, ref string }

// MemStore mirrors PGStore in memory, for unit tests. It reads verdicts from Verdicts.
type MemStore struct {
	Verdicts *verdicts.MemStore // nil: no verdicts
	Fail     error

	mu   sync.Mutex
	rows map[key]Run
}

func NewMemStore(v *verdicts.MemStore) *MemStore { return &MemStore{Verdicts: v, rows: map[key]Run{}} }

func (m *MemStore) Upsert(_ context.Context, batch []Run, now time.Time) ([]bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.Fail != nil {
		return nil, m.Fail
	}
	now = now.UTC().Truncate(time.Microsecond)
	next := maps.Clone(m.rows)
	applied := make([]bool, len(batch))
	for i, r := range batch {
		k := key{r.Source, r.ExternalRef}
		old, seen := next[k]
		if seen && !r.ReportedAt.After(old.ReportedAt) {
			continue
		}
		r.FirstSeenAt, r.UpdatedAt, r.LatestVerdict = now, now, nil
		if seen {
			r.FirstSeenAt = old.FirstSeenAt
		}
		next[k] = r
		applied[i] = true
	}
	m.rows = next
	return applied, nil
}

func (m *MemStore) List(_ context.Context, f Filter) (Page, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.Fail != nil {
		return Page{}, m.Fail
	}
	var vs []verdicts.Verdict
	if m.Verdicts != nil {
		vs = m.Verdicts.All()
	}
	page := Page{Runs: []Run{}, Counts: zeroCounts()}
	var hits []Run
	for _, r := range m.rows {
		if !matchesBase(r, f, vs) {
			continue
		}
		page.Counts[r.Status]++
		if len(f.Statuses) > 0 && !slices.Contains(f.Statuses, r.Status) {
			continue
		}
		if f.After != nil && !f.After.after(r) {
			continue
		}
		r.LatestVerdict = latest(vs, r.SubjectRef())
		hits = append(hits, r)
	}
	slices.SortFunc(hits, compareRuns)
	if len(hits) > f.Limit {
		hits = hits[:f.Limit]
		c := CursorOf(hits[len(hits)-1])
		page.Next = &c
	}
	page.Runs = append(page.Runs, hits...)
	return page, nil
}

func matchesBase(r Run, f Filter, vs []verdicts.Verdict) bool {
	at := r.SortAt()
	switch {
	case f.Source != "" && r.Source != f.Source,
		f.ScopeID != "" && (r.ScopeID == nil || *r.ScopeID != f.ScopeID),
		f.Executor != "" && (r.Executor == nil || *r.Executor != f.Executor),
		f.Since != nil && at.Before(*f.Since),
		f.Until != nil && !at.Before(*f.Until):
		return false
	}
	if f.NeedsVerdict {
		if !Terminal[r.Status] {
			return false
		}
		for _, v := range vs {
			if v.SubjectKind == verdicts.SubjectRun && v.SubjectRef == r.SubjectRef() {
				return false
			}
		}
	}
	return true
}

// latest is the newest verdict on the run that no other verdict supersedes.
func latest(vs []verdicts.Verdict, ref string) *VerdictSummary {
	superseded := map[string]bool{}
	for _, v := range vs {
		if v.Supersedes != nil {
			superseded[*v.Supersedes] = true
		}
	}
	var best *verdicts.Verdict
	for i, v := range vs {
		if v.SubjectKind != verdicts.SubjectRun || v.SubjectRef != ref || superseded[v.ID] {
			continue
		}
		if best == nil || v.CreatedAt.After(best.CreatedAt) || (v.CreatedAt.Equal(best.CreatedAt) && v.ID > best.ID) {
			best = &vs[i]
		}
	}
	if best == nil {
		return nil
	}
	return &VerdictSummary{ID: best.ID, Judge: best.Judge, JudgeKind: string(best.JudgeKind), Standard: best.Standard,
		Value: best.Value, CreatedAt: best.CreatedAt.UTC()}
}
