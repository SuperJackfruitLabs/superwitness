package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/SuperJackfruitLabs/superwitness/internal/auth"
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
