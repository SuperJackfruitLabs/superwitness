package superpipeline

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/SuperJackfruitLabs/superwitness/internal/auth"
	"github.com/SuperJackfruitLabs/superwitness/internal/source"
	"github.com/SuperJackfruitLabs/superwitness/internal/treaty"
)

// cases is one board-read operation's 200 cases, from the vendored treaty data.
func cases(t *testing.T, operation string) treaty.Cases {
	t.Helper()
	c, err := treaty.Response(treaty.BoardReadCases, operation, "200")
	if err != nil {
		t.Fatal(err)
	}
	return c
}

// devRun is the case the dev run is: run_01 on brd_01, four gates, usage unreported.
func devRun(t *testing.T) []byte {
	t.Helper()
	b, err := cases(t, "readRunEvidence").Case("gates resolved, decided by an unlinked person, pending and cancelled")
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// runOf is the board and run a run-evidence body is about.
func runOf(t *testing.T, body []byte) source.RunRef {
	t.Helper()
	type run struct {
		ID      string `json:"id"`
		BoardID string `json:"board_id"`
	}
	var b struct {
		Run      run `json:"run"`
		Pipeline struct {
			Run run `json:"run"`
		} `json:"pipeline"`
	}
	_ = json.Unmarshal(body, &b) // an invalid case may lack either; the ref then falls back below
	r := b.Run
	if r.ID == "" {
		r = b.Pipeline.Run
	}
	ref, err := source.NewSuperpipelineRef(r.BoardID, r.ID)
	if err != nil {
		t.Fatal(err)
	}
	return ref
}

func ref(t *testing.T, run string) source.RunRef {
	t.Helper()
	r, err := source.NewSuperpipelineRef("brd_01", run)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestFetchDecodesC4(t *testing.T) {
	var gotPath, gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotAuth = r.URL.Path, r.Header.Get("Authorization")
		_, _ = w.Write(devRun(t))
	}))
	defer srv.Close()
	tokens := &auth.StaticTokenSource{Value: "tok-sp"}
	a := New(srv.URL, tokens, srv.Client())

	f, st := a.Fetch(context.Background(), ref(t, "run_01"))
	if st != source.StatusOK {
		t.Fatalf("status = %s", st)
	}
	if gotPath != "/v1/boards/brd_01/runs/run_01/evidence-v1" || gotAuth != "Bearer tok-sp" {
		t.Errorf("request = %s %q", gotPath, gotAuth)
	}
	if tokens.Calls() != 1 {
		t.Errorf("token calls = %d", tokens.Calls())
	}
	if f.Work != nil || f.WorkStatus != source.StatusUnavailable {
		t.Fatal("legacy must not imply versioned provenance")
	}
	if f.Source != source.Superpipeline || f.Version != "2026-10-04T11:00:00Z" || f.Run == nil {
		t.Fatalf("fragment = %+v", f)
	}
	if f.Run.Run.StageKey != "draft" || len(f.Run.Gates) != 4 || f.Run.Gates[1].RunID != nil {
		t.Errorf("run = %+v", f.Run)
	}
	if f.Run.Usage.Status != "unreported" || f.Run.Usage.InputTokens != nil || f.Run.Usage.CostUSD != nil {
		t.Errorf("usage = %+v", f.Run.Usage)
	}
}

func TestNotFoundCodes(t *testing.T) {
	for _, code := range []string{"RUN_NOT_FOUND", "BOARD_NOT_FOUND"} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(404)
			fmt.Fprintf(w, `{"error":{"code":%q}}`, code)
		}))
		a := New(srv.URL, &auth.StaticTokenSource{Value: "t"}, srv.Client())
		if _, st := a.Fetch(context.Background(), ref(t, "run_01")); st != source.StatusNotFound {
			t.Errorf("%s: %s", code, st)
		}
		srv.Close()
	}
}

// Before superpipeline deploys the route, the Worker answers a plain 404.
func TestBare404IsUnavailable(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	defer srv.Close()
	a := New(srv.URL, &auth.StaticTokenSource{Value: "t"}, srv.Client())
	if _, st := a.Fetch(context.Background(), ref(t, "run_01")); st != source.StatusUnavailable {
		t.Errorf("status = %s, want unavailable", st)
	}
}

func TestUnauthorizedInvalidatesToken(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(401) }))
	defer srv.Close()
	tokens := &auth.StaticTokenSource{Value: "stale"}
	a := New(srv.URL, tokens, srv.Client())
	if _, st := a.Fetch(context.Background(), ref(t, "run_01")); st != source.StatusUnauthorized {
		t.Errorf("status = %s", st)
	}
	if tokens.Invalidations() != 1 {
		t.Errorf("invalidations = %d", tokens.Invalidations())
	}
}

func TestTokenFailureMakesNoRequest(t *testing.T) {
	hit := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { hit = true }))
	defer srv.Close()
	a := New(srv.URL, &auth.StaticTokenSource{Err: fmt.Errorf("x: %w", auth.ErrTokenRejected)}, srv.Client())
	if _, st := a.Fetch(context.Background(), ref(t, "run_01")); st != source.StatusUnauthorized || hit {
		t.Errorf("status = %s, hit = %v", st, hit)
	}
}

func TestMismatchedRunIsUnavailable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(devRun(t)) // always run_01
	}))
	defer srv.Close()
	a := New(srv.URL, &auth.StaticTokenSource{Value: "t"}, srv.Client())
	if _, st := a.Fetch(context.Background(), ref(t, "run_02")); st != source.StatusUnavailable {
		t.Errorf("status = %s", st)
	}
}

func TestTimeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
	}))
	defer srv.Close()
	a := New(srv.URL, &auth.StaticTokenSource{Value: "t"}, srv.Client())
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if _, st := a.Fetch(ctx, ref(t, "run_01")); st != source.StatusTimeout {
		t.Errorf("status = %s", st)
	}
}

func TestVersionedWorkEvidenceFetch(t *testing.T) {
	v1, err := cases(t, "readRunEvidenceV1").Case("superpipeline's run-evidence-v1 fixture")
	if err != nil {
		t.Fatal(err)
	}
	var parts map[string]json.RawMessage
	if err := json.Unmarshal(v1, &parts); err != nil {
		t.Fatal(err)
	}
	served := v1
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "evidence-v1") {
			_, _ = w.Write(served)
			return
		}
		_, _ = w.Write(parts["pipeline"])
	}))
	defer srv.Close()
	a := New(srv.URL, &auth.StaticTokenSource{Value: "test"}, srv.Client())
	ref := runOf(t, v1)
	f, st := a.Fetch(context.Background(), ref)
	if st != source.StatusOK || f.WorkStatus != source.StatusOK || f.Work == nil {
		t.Fatalf("%s %+v", st, f)
	}
	if f.Work.Usage.CostProvenance != "agent-reported-not-provider-bill" {
		t.Fatal("cost provenance lost")
	}
	var work map[string]any
	if err := json.Unmarshal(v1, &work); err != nil {
		t.Fatal(err)
	}
	work["executor_evidence"].(map[string]any)["external_run_id"] = "foreign"
	served, _ = json.Marshal(work)
	f, st = a.Fetch(context.Background(), ref)
	if st != source.StatusOK || f.Work != nil {
		t.Fatal("foreign metadata accepted")
	}
}

// Every run-evidence case of treaty's board-read cases is a run SuperWitness shows, and every
// invalid one is unavailable: an answer outside the contract is not an answer.
func TestFetchRunsEveryTreatyCase(t *testing.T) {
	c := cases(t, "readRunEvidence")
	serve := func(t *testing.T, body []byte) *Adapter {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if strings.HasSuffix(r.URL.Path, "evidence-v1") {
				w.WriteHeader(404)
				return
			}
			_, _ = w.Write(body)
		}))
		t.Cleanup(srv.Close)
		return New(srv.URL, &auth.StaticTokenSource{Value: "t"}, srv.Client())
	}
	for name, body := range c.Valid {
		t.Run(name, func(t *testing.T) {
			f, st := serve(t, body).Fetch(context.Background(), runOf(t, body))
			if st != source.StatusOK || f.Run == nil || f.Work != nil {
				t.Fatalf("status %s, fragment %+v", st, f)
			}
		})
	}
	for _, inv := range c.Invalid {
		t.Run(inv.Why, func(t *testing.T) {
			if _, st := serve(t, inv.Body).Fetch(context.Background(), runOf(t, inv.Body)); st != source.StatusUnavailable {
				t.Fatalf("status %s, want unavailable", st)
			}
		})
	}
}

// Every evidence-v1 case rides along with its pipeline; every invalid one, an unknown
// schema_version among them, leaves its versioned evidence unavailable, never read as v1 (and
// the run too, when the case breaks the pipeline it nests).
func TestFetchWorkEvidenceEveryTreatyCase(t *testing.T) {
	c := cases(t, "readRunEvidenceV1")
	fetch := func(t *testing.T, body []byte) source.Fragment {
		var parts map[string]json.RawMessage
		_ = json.Unmarshal(body, &parts)
		pipeline := parts["pipeline"]
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if strings.HasSuffix(r.URL.Path, "evidence-v1") {
				_, _ = w.Write(body)
				return
			}
			_, _ = w.Write(pipeline)
		}))
		t.Cleanup(srv.Close)
		f, _ := New(srv.URL, &auth.StaticTokenSource{Value: "t"}, srv.Client()).Fetch(context.Background(), runOf(t, body))
		return f
	}
	for name, body := range c.Valid {
		t.Run(name, func(t *testing.T) {
			if f := fetch(t, body); f.Run == nil || f.WorkStatus != source.StatusOK || f.Work == nil {
				t.Fatalf("work evidence: %s", f.WorkStatus)
			}
		})
	}
	for _, inv := range c.Invalid {
		t.Run(inv.Why, func(t *testing.T) {
			if f := fetch(t, inv.Body); f.Work != nil || (f.Run != nil && f.WorkStatus != source.StatusUnavailable) {
				t.Fatalf("work evidence: %s", f.WorkStatus)
			}
		})
	}
}

// The adapter's routes are the contract's: the run-evidence paths board-read names.
func TestRoutesAreTheContracts(t *testing.T) {
	ops, err := treaty.Operations(treaty.BoardRead)
	if err != nil {
		t.Fatal(err)
	}
	var paths []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.Method+" "+r.URL.Path)
		w.WriteHeader(404)
	}))
	defer srv.Close()
	ref, _ := source.NewSuperpipelineRef("brd_B", "run_R")
	_, _ = New(srv.URL, &auth.StaticTokenSource{Value: "t"}, srv.Client()).Fetch(context.Background(), ref)
	want := []string{}
	for _, op := range []string{"readRunEvidence"} {
		o := ops[op]
		want = append(want, o.Method+" "+strings.NewReplacer("{boardId}", "brd_B", "{runId}", "run_R").Replace(o.Path))
	}
	if strings.Join(paths, ",") != strings.Join(want, ",") {
		t.Fatalf("requests %v, want %v", paths, want)
	}
	// evidence-v1 is read only once the legacy route answered; its path is the contract's too.
	if v1 := ops["readRunEvidenceV1"].Path; v1 != ops["readRunEvidence"].Path+"-v1" {
		t.Fatalf("evidence-v1 is %s", v1)
	}
}
