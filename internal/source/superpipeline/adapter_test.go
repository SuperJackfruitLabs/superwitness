package superpipeline

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/SuperJackfruitLabs/superwitness/internal/auth"
	"github.com/SuperJackfruitLabs/superwitness/internal/contracts"
	"github.com/SuperJackfruitLabs/superwitness/internal/source"
)

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
		_, _ = w.Write(contracts.SuperpipelineEvidence)
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
		_, _ = w.Write(contracts.SuperpipelineEvidence) // always run_01
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
	b, err := os.ReadFile("../testdata/run-evidence-v1.json")
	if err != nil {
		t.Fatal(err)
	}
	var work source.WorkEvidence
	if err := json.Unmarshal(b, &work); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "evidence-v1") {
			json.NewEncoder(w).Encode(work)
			return
		}
		json.NewEncoder(w).Encode(work.Pipeline)
	}))
	defer srv.Close()
	a := New(srv.URL, &auth.StaticTokenSource{Value: "test"}, srv.Client())
	f, st := a.Fetch(context.Background(), ref(t, "run_01"))
	if st != source.StatusOK || f.WorkStatus != source.StatusOK || f.Work == nil {
		t.Fatalf("%s %+v", st, f)
	}
	if f.Work.Usage.CostProvenance != "agent-reported-not-provider-bill" {
		t.Fatal("cost provenance lost")
	}
	work.ExecutorEvidence.ExternalRunID = "foreign"
	f, st = a.Fetch(context.Background(), ref(t, "run_01"))
	if st != source.StatusOK || f.Work != nil {
		t.Fatal("foreign metadata accepted")
	}
}
