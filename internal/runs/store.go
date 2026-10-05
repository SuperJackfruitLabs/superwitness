package runs

import (
	"context"
	"encoding/json"
	"errors"
	"time"
)

var ErrUnavailable = errors.New("run registry unavailable")

// VerdictSummary is a run's latest verdict that no other verdict supersedes.
type VerdictSummary struct {
	ID        string          `json:"id"`
	Judge     string          `json:"judge"`
	JudgeKind string          `json:"judge_kind"`
	Standard  string          `json:"standard"`
	Value     json.RawMessage `json:"value"`
	CreatedAt time.Time       `json:"created_at"`
}

// Filter selects runs. Zero fields select everything; Limit must be 1 or more.
type Filter struct {
	Source       string
	ScopeID      string
	Statuses     []string
	Executor     string
	Since, Until *time.Time // on the sort key: Since inclusive, Until exclusive
	NeedsVerdict bool       // terminal, and no verdict names the run
	After        *Cursor
	Limit        int
}

type Page struct {
	Runs   []Run
	Next   *Cursor        // nil on the last page
	Counts map[string]int // every status, for the filter without Statuses and After
}

type Store interface {
	// Upsert applies each report whose reported_at is newer than the stored one, in one
	// transaction, and says which were applied. now becomes updated_at (and first_seen_at for a new run).
	Upsert(ctx context.Context, batch []Run, now time.Time) ([]bool, error)
	List(ctx context.Context, f Filter) (Page, error)
}

func zeroCounts() map[string]int {
	m := make(map[string]int, len(Statuses))
	for _, s := range Statuses {
		m[s] = 0
	}
	return m
}
