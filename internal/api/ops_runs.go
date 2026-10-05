package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"slices"
	"time"

	"github.com/SuperJackfruitLabs/superwitness/internal/auth"
	"github.com/SuperJackfruitLabs/superwitness/internal/config"
	"github.com/SuperJackfruitLabs/superwitness/internal/runs"
)

type ReportResult struct {
	ExternalRef string `json:"external_ref"`
	Applied     bool   `json:"applied"`
}

func registryDown() *APIError {
	return &APIError{Status: http.StatusServiceUnavailable, Code: "store_unavailable",
		Message: "the run registry is unavailable; retry", Retryable: true}
}

// ReportRuns records a validated batch for a reporter. Each report must be for the one source
// SW_RUN_SOURCES binds the caller to; the first that is not refuses the whole batch.
func (o *Ops) ReportRuns(ctx context.Context, caller auth.Principal, batch []runs.Run) ([]ReportResult, error) {
	bound, ok := o.RunSources[caller.ID]
	for i, r := range batch {
		if !ok || r.Source != bound {
			msg := fmt.Sprintf("%s is not bound to any source; SW_RUN_SOURCES names the reporters", caller.ID)
			if ok {
				msg = fmt.Sprintf("%s may report only for source %q, not %q", caller.ID, bound, r.Source)
			}
			idx := i
			return nil, &APIError{Status: http.StatusForbidden, Code: "source_not_allowed", Message: msg, Index: &idx}
		}
	}
	applied, err := o.Runs.Upsert(ctx, batch, o.now())
	if errors.Is(err, runs.ErrUnavailable) {
		return nil, registryDown()
	}
	if err != nil {
		return nil, err
	}
	out := make([]ReportResult, len(batch))
	for i, r := range batch {
		out[i] = ReportResult{ExternalRef: r.ExternalRef, Applied: applied[i]}
	}
	return out, nil
}

const (
	DefaultRunPage = 50
	MaxRunPage     = 200
)

var sourceFilter = regexp.MustCompile(config.SourceNamePattern)

// RunQuery is GET /v1/runs's parameters, and list_runs's arguments, unparsed.
type RunQuery struct {
	Source       string
	Scope        string
	Status       []string
	Executor     string
	Since, Until string
	NeedsVerdict bool
	Cursor       string
	Limit        int
}

type RunPage struct {
	Runs       []runs.Run     `json:"runs"`
	NextCursor *string        `json:"next_cursor"` // null on the last page
	Counts     map[string]int `json:"counts"`      // every status, for the filter without status
}

func (o *Ops) ListRuns(ctx context.Context, q RunQuery) (RunPage, error) {
	f := runs.Filter{Source: q.Source, ScopeID: q.Scope, Executor: q.Executor, NeedsVerdict: q.NeedsVerdict}
	if q.Source != "" && !sourceFilter.MatchString(q.Source) {
		return RunPage{}, badRequest("invalid_source", "source is lowercase letters, digits and -, starting with a letter")
	}
	for _, st := range q.Status {
		if !runs.ValidStatus(st) {
			return RunPage{}, badRequest("invalid_status", "status is one of queued, running, waiting, succeeded, failed, cancelled")
		}
	}
	f.Statuses = slices.Clone(q.Status)
	var err error
	if f.Since, err = parseQueryTime("since", q.Since); err != nil {
		return RunPage{}, err
	}
	if f.Until, err = parseQueryTime("until", q.Until); err != nil {
		return RunPage{}, err
	}
	if f.After, err = runs.DecodeCursor(q.Cursor); err != nil {
		return RunPage{}, badRequest("invalid_cursor", err.Error())
	}
	switch {
	case q.Limit < 0:
		return RunPage{}, badRequest("invalid_limit", "limit must be a positive integer")
	case q.Limit == 0:
		f.Limit = DefaultRunPage
	case q.Limit > MaxRunPage:
		f.Limit = MaxRunPage
	default:
		f.Limit = q.Limit
	}
	p, err := o.Runs.List(ctx, f)
	if errors.Is(err, runs.ErrUnavailable) {
		return RunPage{}, registryDown()
	}
	if err != nil {
		return RunPage{}, err
	}
	out := RunPage{Runs: p.Runs, Counts: p.Counts}
	if p.Next != nil {
		c := runs.EncodeCursor(*p.Next)
		out.NextCursor = &c
	}
	return out, nil
}

func parseQueryTime(name, v string) (*time.Time, error) {
	if v == "" {
		return nil, nil
	}
	t, err := time.Parse(time.RFC3339Nano, v)
	if err != nil {
		return nil, badRequest("invalid_time", name+" must be an RFC 3339 timestamp such as 2026-10-06T00:00:00Z")
	}
	t = t.UTC()
	return &t, nil
}
