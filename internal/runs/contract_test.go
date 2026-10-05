package runs

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/SuperJackfruitLabs/superwitness/internal/verdicts"
)

func ptr[T any](v T) *T { return &v }

// mk builds a validated-looking run; started is minutes after t0, or none when negative.
func mk(ref, status string, startedMin int, reported time.Time) Run {
	r := Run{Source: "superpipeline", ExternalRef: ref, Status: status, SourceStatus: status,
		ScopeID: ptr("brd_01"), ScopeName: ptr("Press"), Executor: ptr("prn_agent01"), ReportedAt: reported}
	if startedMin >= 0 {
		r.StartedAt = ptr(t0.Add(time.Duration(startedMin) * time.Minute))
	}
	return r
}

func refs(rs []Run) []string {
	out := []string{}
	for _, r := range rs {
		out = append(out, r.ExternalRef)
	}
	return out
}

func eq(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func list(t *testing.T, s Store, f Filter) Page {
	t.Helper()
	if f.Limit == 0 {
		f.Limit = 50
	}
	p, err := s.List(context.Background(), f)
	if err != nil {
		t.Fatalf("list %+v: %v", f, err)
	}
	return p
}

func upsert(t *testing.T, s Store, now time.Time, rs ...Run) []bool {
	t.Helper()
	applied, err := s.Upsert(context.Background(), rs, now)
	if err != nil {
		t.Fatalf("upsert: %v", err)
	}
	return applied
}

// storeContract holds every Store to the same behaviour. addVerdict records a verdict in the
// database the store reads verdicts from.
func storeContract(t *testing.T, s Store, addVerdict func(verdicts.Verdict)) {
	t.Helper()
	now := t0.Add(time.Hour)

	// insert, then stale, equal and newer reports
	if a := upsert(t, s, now, mk("brd_01/run_01", "running", 0, t0)); !a[0] {
		t.Fatal("first report not applied")
	}
	if a := upsert(t, s, now.Add(time.Minute), mk("brd_01/run_01", "queued", 0, t0.Add(-time.Second))); a[0] {
		t.Error("stale: an older report was applied")
	}
	if a := upsert(t, s, now.Add(time.Minute), mk("brd_01/run_01", "failed", 0, t0)); a[0] {
		t.Error("equal: a report with the same reported_at was applied")
	}
	got := list(t, s, Filter{}).Runs
	if len(got) != 1 || got[0].Status != "running" || !got[0].FirstSeenAt.Equal(now) || !got[0].UpdatedAt.Equal(now) {
		t.Fatalf("after stale and equal reports: %+v", got)
	}
	if a := upsert(t, s, now.Add(2*time.Minute), mk("brd_01/run_01", "succeeded", 0, t0.Add(time.Second))); !a[0] {
		t.Error("a newer report was not applied")
	}
	got = list(t, s, Filter{}).Runs
	if got[0].Status != "succeeded" || !got[0].FirstSeenAt.Equal(now) || !got[0].UpdatedAt.Equal(now.Add(2*time.Minute)) {
		t.Errorf("after a newer report: status %s first_seen %v updated %v", got[0].Status, got[0].FirstSeenAt, got[0].UpdatedAt)
	}

	// a batch holding the same run twice applies in order
	if a := upsert(t, s, now, mk("brd_01/run_02", "queued", -1, t0), mk("brd_01/run_02", "running", 5, t0.Add(time.Second))); !a[0] || !a[1] {
		t.Errorf("same run twice in one batch: %v", a)
	}

	// more runs, for order, filters and counts
	other := mk("brd_02/run_01", "failed", 10, t0)
	other.ScopeID, other.ScopeName, other.Executor = ptr("brd_02"), ptr("Ops"), ptr("prn_agent02")
	canary := mk("x/1", "waiting", 10, t0)
	canary.Source, canary.ScopeID, canary.ScopeName = "canary", nil, nil
	upsert(t, s, now, other, canary, mk("brd_01/run_03", "cancelled", 20, t0))

	// order: sort_at desc, then source, then external_ref. run_02 started at +5; run_01 at 0;
	// brd_02/run_01 and canary x/1 both at +10, so source breaks the tie ("canary" < "superpipeline").
	want := []string{"brd_01/run_03", "x/1", "brd_02/run_01", "brd_01/run_02", "brd_01/run_01"}
	if p := list(t, s, Filter{}); !eq(refs(p.Runs), want) {
		t.Errorf("order = %v, want %v", refs(p.Runs), want)
	}

	// filters
	for name, c := range map[string]struct {
		f    Filter
		want []string
	}{
		"source":   {Filter{Source: "canary"}, []string{"x/1"}},
		"scope":    {Filter{ScopeID: "brd_02"}, []string{"brd_02/run_01"}},
		"executor": {Filter{Executor: "prn_agent02"}, []string{"brd_02/run_01"}},
		"statuses": {Filter{Statuses: []string{"failed", "cancelled"}}, []string{"brd_01/run_03", "brd_02/run_01"}},
		"since":    {Filter{Since: ptr(t0.Add(10 * time.Minute))}, []string{"brd_01/run_03", "x/1", "brd_02/run_01"}},
		"until":    {Filter{Until: ptr(t0.Add(10 * time.Minute))}, []string{"brd_01/run_02", "brd_01/run_01"}},
	} {
		if p := list(t, s, c.f); !eq(refs(p.Runs), c.want) {
			t.Errorf("%s: %v, want %v", name, refs(p.Runs), c.want)
		}
	}

	// counts ignore the status filter but honour the others
	p := list(t, s, Filter{ScopeID: "brd_01", Statuses: []string{"running"}})
	wantCounts := map[string]int{"queued": 0, "running": 1, "waiting": 0, "succeeded": 1, "failed": 0, "cancelled": 1}
	for k, v := range wantCounts {
		if p.Counts[k] != v {
			t.Errorf("counts[%s] = %d, want %d (all: %v)", k, p.Counts[k], v, p.Counts)
		}
	}
	if len(p.Counts) != 6 {
		t.Errorf("counts has %d keys, want all 6 statuses", len(p.Counts))
	}

	// paging: two pages of two and a last page of one; the cursor is stable across inserts
	p1 := list(t, s, Filter{Limit: 2})
	if !eq(refs(p1.Runs), want[:2]) || p1.Next == nil {
		t.Fatalf("page 1 = %v next %v", refs(p1.Runs), p1.Next)
	}
	upsert(t, s, now, mk("brd_01/run_new", "running", 30, t0)) // newer than everything on page 1
	p2 := list(t, s, Filter{Limit: 2, After: p1.Next})
	if !eq(refs(p2.Runs), want[2:4]) || p2.Next == nil {
		t.Errorf("page 2 = %v, want %v", refs(p2.Runs), want[2:4])
	}
	p3 := list(t, s, Filter{Limit: 2, After: p2.Next})
	if !eq(refs(p3.Runs), want[4:]) || p3.Next != nil {
		t.Errorf("page 3 = %v next %v, want %v and no next", refs(p3.Runs), p3.Next, want[4:])
	}

	// scopes: one per (source, id), named by the newest report
	renamed := mk("brd_02/run_09", "running", 40, t0)
	renamed.ScopeID, renamed.ScopeName = ptr("brd_02"), ptr("Operations")
	upsert(t, s, now.Add(time.Hour), renamed)
	scopes, err := s.Scopes(context.Background())
	if err != nil || len(scopes) != 2 || scopes[0].ID != "brd_01" || scopes[0].Runs != 4 || *scopes[0].Name != "Press" ||
		scopes[1].ID != "brd_02" || scopes[1].Runs != 2 || *scopes[1].Name != "Operations" {
		t.Errorf("scopes = %+v %v", scopes, err)
	}

	// latest_verdict and needs_verdict
	ref := "superpipeline:brd_01/run_03"
	nv := func() []string { return refs(list(t, s, Filter{NeedsVerdict: true}).Runs) }
	if got := nv(); !eq(got, []string{"brd_01/run_03", "brd_02/run_01", "brd_01/run_01"}) {
		t.Errorf("needs_verdict before any verdict = %v", got)
	}
	v1 := verdicts.Verdict{ID: "vrd_1", IdempotencyKey: "k1", Kind: "review", SubjectKind: verdicts.SubjectRun, SubjectRef: ref,
		Judge: "prn_human01", JudgeKind: verdicts.JudgeHuman, Standard: "stage:review", Value: json.RawMessage(`{"decision":"pass"}`),
		EvidenceRefs: json.RawMessage(`[]`), CreatedAt: t0.Add(time.Minute)}
	addVerdict(v1)
	v2 := v1
	v2.ID, v2.IdempotencyKey, v2.Value, v2.Supersedes, v2.CreatedAt = "vrd_2", "k2", json.RawMessage(`{"decision":"fail"}`), ptr("vrd_1"), t0.Add(2*time.Minute)
	addVerdict(v2)
	v3 := v1 // a later verdict on the run's attempt does not count
	v3.ID, v3.IdempotencyKey, v3.SubjectKind, v3.SubjectRef, v3.CreatedAt = "vrd_3", "k3", verdicts.SubjectAttempt, "attempt_01", t0.Add(3*time.Minute)
	addVerdict(v3)
	if got := nv(); !eq(got, []string{"brd_02/run_01", "brd_01/run_01"}) {
		t.Errorf("needs_verdict after a verdict = %v", got)
	}
	for _, r := range list(t, s, Filter{}).Runs {
		switch r.ExternalRef {
		case "brd_01/run_03":
			if r.LatestVerdict == nil || r.LatestVerdict.ID != "vrd_2" || string(r.LatestVerdict.Value) != `{"decision":"fail"}` {
				t.Errorf("latest_verdict = %+v, want vrd_2 (the one that supersedes vrd_1)", r.LatestVerdict)
			}
		default:
			if r.LatestVerdict != nil {
				t.Errorf("%s has latest_verdict %+v", r.ExternalRef, r.LatestVerdict)
			}
		}
	}
}
