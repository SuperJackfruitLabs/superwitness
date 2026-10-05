package agentpod

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/SuperJackfruitLabs/superwitness/internal/auth"
	"github.com/SuperJackfruitLabs/superwitness/internal/contracts"
	"github.com/SuperJackfruitLabs/superwitness/internal/source"
)

func hub(t *testing.T, principalHits *atomic.Int32) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/evidence/runs/superpipeline/{run}", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer tok-hub" {
			w.WriteHeader(401)
			return
		}
		if r.PathValue("run") != "run_01" {
			w.WriteHeader(404)
			fmt.Fprint(w, `{"error":"not_found"}`)
			return
		}
		_, _ = w.Write(contracts.HubEvidenceRun)
	})
	mux.HandleFunc("GET /api/evidence/attempts/{id}", func(w http.ResponseWriter, r *http.Request) {
		switch r.PathValue("id") {
		case "attempt_01":
			_, _ = w.Write(contracts.HubEvidenceAttempt)
		case "attempt_solo":
			fmt.Fprint(w, `{"external_source":null,"external_run_id":null,"board_id":null}`)
		default:
			w.WriteHeader(404)
			fmt.Fprint(w, `{"error":"not_found"}`)
		}
	})
	mux.HandleFunc("GET /api/evidence/principals/{id}", func(w http.ResponseWriter, r *http.Request) {
		principalHits.Add(1)
		switch r.PathValue("id") {
		case "prn_human01":
			_, _ = w.Write(contracts.HubPrincipal)
		case "hubuser_7f3a":
			fmt.Fprint(w, `{"id":"prn_human02","kind":"human","handle":"former","suspended":false}`)
		case "prn_broken":
			w.WriteHeader(500)
		default:
			w.WriteHeader(404)
			fmt.Fprint(w, `{"error":"not_found"}`)
		}
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

func TestFetchDecodesC5(t *testing.T) {
	var hits atomic.Int32
	srv := hub(t, &hits)
	tokens := &auth.StaticTokenSource{Value: "tok-hub"}
	a := New(srv.URL, tokens, srv.Client())
	r, _ := source.NewSuperpipelineRef("brd_01", "run_01")
	f, st := a.Fetch(context.Background(), r)
	if st != source.StatusOK {
		t.Fatalf("status = %s", st)
	}
	if tokens.Calls() != 1 {
		t.Errorf("token calls = %d", tokens.Calls())
	}
	at := f.Ledger.Attempts[0]
	if at.ID != "attempt_01" || at.EndSeq == nil || *at.EndSeq != 9 || at.Fingerprint.Harness != "hermes" ||
		at.Fingerprint.Digest != "sha256:c1a88701c4409ae0eafb5f51ec7dabbe5f18037c1256baa045ab8cabf7ed3426" {
		t.Errorf("attempt = %+v", at)
	}
	if f.Ledger.Dispatch == nil || f.Ledger.Dispatch.Outcome != "completed" || f.Version != "2026-10-04T11:00:00Z" {
		t.Errorf("ledger = %+v", f.Ledger)
	}
}

// The hub's board_id, card_id and dispatch are null when the ledger holds no row for the run.
func TestFetchAcceptsNullLedgerRow(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"external_source":"superpipeline","external_run_id":"run_01","board_id":null,"card_id":null,
		  "dispatch":null,"attempts":[],"as_of":"2026-10-04T11:00:00Z"}`)
	}))
	defer srv.Close()
	a := New(srv.URL, &auth.StaticTokenSource{Value: "t"}, srv.Client())
	r, _ := source.NewSuperpipelineRef("brd_01", "run_01")
	f, st := a.Fetch(context.Background(), r)
	if st != source.StatusOK || f.Ledger.Dispatch != nil || f.Ledger.BoardID != nil {
		t.Errorf("status = %s, ledger = %+v", st, f.Ledger)
	}
}

func TestHubNotFoundAndBoardMismatch(t *testing.T) {
	var hits atomic.Int32
	srv := hub(t, &hits)
	a := New(srv.URL, &auth.StaticTokenSource{Value: "tok-hub"}, srv.Client())
	missing, _ := source.NewSuperpipelineRef("brd_01", "run_99")
	if _, st := a.Fetch(context.Background(), missing); st != source.StatusNotFound {
		t.Errorf("missing run: %s", st)
	}
	elsewhere, _ := source.NewSuperpipelineRef("brd_other", "run_01")
	if _, st := a.Fetch(context.Background(), elsewhere); st != source.StatusNotFound {
		t.Errorf("run on another board: %s", st)
	}
}

// The hub route not deployed yet: a plain 404.
func TestHubBare404IsUnavailable(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	defer srv.Close()
	a := New(srv.URL, &auth.StaticTokenSource{Value: "t"}, srv.Client())
	r, _ := source.NewSuperpipelineRef("brd_01", "run_01")
	if _, st := a.Fetch(context.Background(), r); st != source.StatusUnavailable {
		t.Errorf("fetch: %s", st)
	}
	if _, st := a.ResolveAttempt(context.Background(), "attempt_01"); st != source.StatusUnavailable {
		t.Errorf("attempt: %s", st)
	}
	if _, err := a.Lookup(context.Background(), "prn_human01"); !errors.Is(err, auth.ErrLookupUnavailable) {
		t.Errorf("kind: %v", err)
	}
}

func TestResolveAttempt(t *testing.T) {
	var hits atomic.Int32
	srv := hub(t, &hits)
	a := New(srv.URL, &auth.StaticTokenSource{Value: "tok-hub"}, srv.Client())
	link, st := a.ResolveAttempt(context.Background(), "attempt_01")
	if r, ok := link.Ref(); st != source.StatusOK || !ok || r.String() != "superpipeline:brd_01/run_01" {
		t.Errorf("dispatched: %s %+v", st, link)
	}
	link, st = a.ResolveAttempt(context.Background(), "attempt_solo")
	if _, ok := link.Ref(); st != source.StatusOK || ok {
		t.Errorf("undispatched: %s %+v", st, link)
	}
	if _, st := a.ResolveAttempt(context.Background(), "attempt_nope"); st != source.StatusNotFound {
		t.Errorf("unknown: %s", st)
	}
	if _, st := a.ResolveAttempt(context.Background(), "../admin"); st != source.StatusNotFound {
		t.Errorf("invalid id should be not_found without a request: %s", st)
	}
}

func TestLookupCachesAndMapsErrors(t *testing.T) {
	var hits atomic.Int32
	srv := hub(t, &hits)
	a := New(srv.URL, &auth.StaticTokenSource{Value: "tok-hub"}, srv.Client())
	for range 3 {
		if p, err := a.Lookup(context.Background(), "prn_human01"); err != nil || p.Kind != auth.KindHuman || p.ID != "prn_human01" {
			t.Fatalf("lookup = %+v %v", p, err)
		}
	}
	if hits.Load() != 1 {
		t.Errorf("principal hits = %d, want 1 (cached)", hits.Load())
	}
	// A hub auth user id (superpipeline's decided_by_hub_sub) resolves to its prn_.
	if p, err := a.Lookup(context.Background(), "hubuser_7f3a"); err != nil || p.ID != "prn_human02" || p.Kind != auth.KindHuman {
		t.Errorf("hub sub = %+v %v", p, err)
	}
	if _, err := a.Lookup(context.Background(), "prn_nobody"); !errors.Is(err, auth.ErrPrincipalNotFound) {
		t.Errorf("nobody: %v", err)
	}
	if _, err := a.Lookup(context.Background(), "prn_broken"); !errors.Is(err, auth.ErrLookupUnavailable) {
		t.Errorf("broken: %v", err)
	}
	if _, err := a.Lookup(context.Background(), "../admin"); !errors.Is(err, auth.ErrPrincipalNotFound) {
		t.Errorf("unsafe key must not reach the hub: %v", err)
	}
}

func TestUnauthorizedInvalidates(t *testing.T) {
	var hits atomic.Int32
	srv := hub(t, &hits)
	tokens := &auth.StaticTokenSource{Value: "wrong"}
	a := New(srv.URL, tokens, srv.Client())
	r, _ := source.NewSuperpipelineRef("brd_01", "run_01")
	if _, st := a.Fetch(context.Background(), r); st != source.StatusUnauthorized {
		t.Errorf("status = %s", st)
	}
	if tokens.Invalidations() != 1 {
		t.Error("token not invalidated")
	}
}
