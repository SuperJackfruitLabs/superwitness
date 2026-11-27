package agentpod

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/SuperJackfruitLabs/superwitness/internal/auth"
	"github.com/SuperJackfruitLabs/superwitness/internal/source"
	"github.com/SuperJackfruitLabs/superwitness/internal/source/fake"
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
		_, _ = w.Write(fake.DevHubRun)
	})
	mux.HandleFunc("GET /api/evidence/attempts/{id}", func(w http.ResponseWriter, r *http.Request) {
		switch r.PathValue("id") {
		case "attempt_01":
			fmt.Fprint(w, `{"external_source":"superpipeline","external_run_id":"run_01","board_id":"brd_01"}`)
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
			fmt.Fprint(w, `{"id":"prn_human01","kind":"human","handle":"human01","suspended":false}`)
		case "hubuser_7f3a":
			fmt.Fprint(w, `{"id":"prn_human02","kind":"human","handle":"former","suspended":false}`)
		case "hubuser_gone":
			fmt.Fprint(w, `{"id":"prn_human09","kind":"human","handle":"gone","suspended":true}`)
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
	if tokens.Calls() != 2 {
		t.Errorf("token calls = %d", tokens.Calls())
	}
	if f.Execution != nil || f.ExecutionStatus != source.StatusUnavailable {
		t.Fatal("legacy C5 must not imply execution evidence")
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
	if p, err := a.Lookup(context.Background(), "hubuser_gone"); err != nil || p.ID != "prn_human09" || !p.Suspended {
		t.Errorf("suspended = %+v %v", p, err)
	}
	if p, _ := a.Lookup(context.Background(), "prn_human01"); p.Suspended {
		t.Errorf("prn_human01 reads as suspended")
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

func TestNegotiatedExecutionEvidence(t *testing.T) {
	envelope := map[string]any{}
	if err := json.Unmarshal(evidenceCase(t, "readExecutionEvidence", "every execution of AgentPod's corpus"), &envelope); err != nil {
		t.Fatal(err)
	}
	// The corpus's third execution repeats the second's session, which SuperWitness refuses as a
	// conflicting join (source's TestStricterThanTreatyIsOnlyTheDuplicateSession).
	envelope["executions"] = envelope["executions"].([]any)[:2]
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/execution") {
			if r.URL.Query().Get("schema_version") != source.ExecutionSchemaV1 {
				t.Error("missing explicit schema negotiation")
			}
			if err := json.NewEncoder(w).Encode(envelope); err != nil {
				t.Error(err)
			}
			return
		}
		// The ledger's card binding is the authority checked below.
		fmt.Fprint(w, ledgerOf("run_abcdef", "brd_abcdef", "card_abcdef"))
	}))
	defer srv.Close()
	adapter := New(srv.URL, &auth.StaticTokenSource{Value: "test"}, srv.Client())
	ref, _ := source.NewSuperpipelineRef("brd_abcdef", "run_abcdef")
	f, st := adapter.Fetch(context.Background(), ref)
	if st != source.StatusOK || f.ExecutionStatus != source.StatusOK || f.Execution == nil {
		t.Fatalf("%s %+v", st, f)
	}
	if len(f.Execution.Executions) != 2 || len(f.Execution.Executions[1].Tools) != 2 {
		t.Fatal("actual receipt metadata omitted")
	}
	envelope["executions"].([]any)[0].(map[string]any)["task"].(map[string]any)["card_id"] = "card_foreign"
	f, st = adapter.Fetch(context.Background(), ref)
	if st != source.StatusOK || f.Execution != nil || f.ExecutionStatus != source.StatusUnavailable {
		t.Fatal("conflicting execution evidence should be unavailable while legacy remains readable")
	}
}

func TestFetchCommittedExportReceipt(t *testing.T) {
	var receipt map[string]any
	if err := json.Unmarshal(evidenceCase(t, "readExecutionExport", "committed-manifest"), &receipt); err != nil {
		t.Fatal(err)
	}
	id := receipt["export_id"].(string)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/evidence/exports/"+id || r.URL.Query().Get("schema_version") != source.ExecutionExportSchemaV1 || r.Header.Get("Authorization") != "Bearer test" {
			t.Error("unscoped export receipt request")
		}
		if err := json.NewEncoder(w).Encode(receipt); err != nil {
			t.Error(err)
		}
	}))
	defer srv.Close()
	a := New(srv.URL, &auth.StaticTokenSource{Value: "test"}, srv.Client())
	got, status := a.FetchExport(context.Background(), id)
	if status != source.StatusOK || got.ManifestSHA256 != receipt["manifest_sha256"] || got.Verification != "download-sha256-verified" {
		t.Fatalf("%s %+v", status, got)
	}
	library := receipt["library"].(map[string]any)
	library["bytes"] = library["bytes"].(float64) + 1
	if _, status = a.FetchExport(context.Background(), id); status != source.StatusUnavailable {
		t.Fatal("contradictory Library commit receipt accepted")
	}
}
