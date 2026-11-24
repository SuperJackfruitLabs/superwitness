package agentpod

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/SuperJackfruitLabs/superwitness/internal/auth"
	"github.com/SuperJackfruitLabs/superwitness/internal/source"
	"github.com/SuperJackfruitLabs/superwitness/internal/treaty"
)

// Every read this adapter makes runs every case of agentpod.evidence (treaty's
// fixtures/http/agentpod/evidence.v1.cases.json, vendored in internal/treaty): each valid body is
// read, each invalid one is refused.

func evidenceCases(t *testing.T, operation string) treaty.Cases {
	t.Helper()
	c, err := treaty.Response(treaty.EvidenceCases, operation, "200")
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func evidenceCase(t *testing.T, operation, name string) []byte {
	t.Helper()
	b, err := evidenceCases(t, operation).Case(name)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// ledgerOf is the hub's run evidence for a run with no attempts yet.
func ledgerOf(run, board, card string) string {
	return fmt.Sprintf(`{"external_source":"superpipeline","external_run_id":%q,"board_id":%q,"card_id":%q,"dispatch":null,"attempts":[],"as_of":"2026-10-21T00:00:01.000Z"}`, run, board, card)
}

// serveAll answers every request with body.
func serveAll(t *testing.T, body []byte) *Adapter {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write(body) }))
	t.Cleanup(srv.Close)
	return New(srv.URL, &auth.StaticTokenSource{Value: "t"}, srv.Client())
}

func field(t *testing.T, body []byte, key string) string {
	t.Helper()
	var m map[string]any
	_ = json.Unmarshal(body, &m)
	s, _ := m[key].(string)
	return s
}

func TestRunEvidenceEveryCase(t *testing.T) {
	c := evidenceCases(t, "readRunEvidence")
	refOf := func(t *testing.T, body []byte) source.RunRef {
		board := field(t, body, "board_id")
		if board == "" {
			board = "brd_any" // no ledger row: the hub names no board
		}
		r, err := source.NewSuperpipelineRef(board, field(t, body, "external_run_id"))
		if err != nil {
			t.Fatal(err)
		}
		return r
	}
	for name, body := range c.Valid {
		t.Run(name, func(t *testing.T) {
			f, st := serveAll(t, body).Fetch(context.Background(), refOf(t, body))
			if st != source.StatusOK || f.Ledger == nil {
				t.Fatalf("status %s", st)
			}
		})
	}
	for _, inv := range c.Invalid {
		t.Run(inv.Why, func(t *testing.T) {
			if _, st := serveAll(t, inv.Body).Fetch(context.Background(), refOf(t, inv.Body)); st != source.StatusUnavailable {
				t.Fatalf("status %s, want unavailable", st)
			}
		})
	}
}

func TestAttemptRunEveryCase(t *testing.T) {
	c := evidenceCases(t, "readAttemptRun")
	const attempt = "attempt_9f1c2ab0-4d7e-4b3a-8c88-0d6e2f7c1b90"
	for name, body := range c.Valid {
		t.Run(name, func(t *testing.T) {
			link, st := serveAll(t, body).ResolveAttempt(context.Background(), attempt)
			if st != source.StatusOK {
				t.Fatalf("status %s", st)
			}
			if _, ok := link.Ref(); ok != (field(t, body, "external_run_id") != "") {
				t.Fatalf("dispatched = %v for %s", ok, body)
			}
		})
	}
	for _, inv := range c.Invalid {
		t.Run(inv.Why, func(t *testing.T) {
			if _, st := serveAll(t, inv.Body).ResolveAttempt(context.Background(), attempt); st != source.StatusUnavailable {
				t.Fatalf("status %s, want unavailable", st)
			}
		})
	}
}

func TestPrincipalEveryCase(t *testing.T) {
	c := evidenceCases(t, "readEvidencePrincipal")
	for name, body := range c.Valid {
		t.Run(name, func(t *testing.T) {
			id := field(t, body, "id")
			p, err := serveAll(t, body).Lookup(context.Background(), id)
			if err != nil || p.ID != id || string(p.Kind) != field(t, body, "kind") {
				t.Fatalf("%+v %v", p, err)
			}
		})
	}
	for _, inv := range c.Invalid {
		t.Run(inv.Why, func(t *testing.T) {
			if p, err := serveAll(t, inv.Body).Lookup(context.Background(), "prn_0123456789abcdef0123"); err == nil {
				t.Fatalf("accepted %+v", p)
			}
		})
	}
}

// An execution envelope rides along with its run's ledger. Every case reads; every invalid one,
// an execution or envelope whose schema_version SuperWitness does not know among them, leaves
// the ledger readable and the execution evidence unavailable: never read as v1.
func TestExecutionEvidenceEveryCase(t *testing.T) {
	c := evidenceCases(t, "readExecutionEvidence")
	fetch := func(t *testing.T, body []byte) source.Fragment {
		run, board := field(t, body, "external_run_id"), field(t, body, "board_id")
		if run == "" {
			run, board = "run_abcdef", "brd_abcdef"
		}
		ledger := ledgerOf(run, board, "card_abcdef")
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if strings.HasSuffix(r.URL.Path, "/execution") {
				_, _ = w.Write(body)
				return
			}
			fmt.Fprint(w, ledger)
		}))
		t.Cleanup(srv.Close)
		ref, _ := source.NewSuperpipelineRef(board, run)
		f, st := New(srv.URL, &auth.StaticTokenSource{Value: "t"}, srv.Client()).Fetch(context.Background(), ref)
		if st != source.StatusOK || f.Ledger == nil {
			t.Fatalf("the ledger: %s", st)
		}
		return f
	}
	for name, body := range c.Valid {
		t.Run(name, func(t *testing.T) {
			var e struct{ Executions []json.RawMessage }
			_ = json.Unmarshal(body, &e)
			f := fetch(t, body)
			if name == "every execution of AgentPod's corpus" {
				// Two of its executions share a session: a conflicting join SuperWitness refuses
				// (source's TestStricterThanTreatyIsOnlyTheDuplicateSession).
				if f.Execution != nil || f.ExecutionStatus != source.StatusUnavailable {
					t.Fatal("two executions of one session were read")
				}
				return
			}
			// A run with no executions has no execution evidence to show; that is the adapter's
			// rule, not a refusal of the body (source's tests decode it).
			if want := len(e.Executions) > 0; (f.Execution != nil) != want || (f.ExecutionStatus == source.StatusOK) != want {
				t.Fatalf("execution %v, status %s", f.Execution != nil, f.ExecutionStatus)
			}
		})
	}
	for _, inv := range c.Invalid {
		t.Run(inv.Why, func(t *testing.T) {
			if f := fetch(t, inv.Body); f.Execution != nil || f.ExecutionStatus != source.StatusUnavailable {
				t.Fatalf("execution %v, status %s", f.Execution != nil, f.ExecutionStatus)
			}
		})
	}
}

func TestExportReceiptEveryCase(t *testing.T) {
	c := evidenceCases(t, "readExecutionExport")
	for name, body := range c.Valid {
		t.Run(name, func(t *testing.T) {
			id := field(t, body, "export_id")
			if r, st := serveAll(t, body).FetchExport(context.Background(), id); st != source.StatusOK || r.ExportID != id {
				t.Fatalf("status %s", st)
			}
		})
	}
	for _, inv := range c.Invalid {
		t.Run(inv.Why, func(t *testing.T) {
			if _, st := serveAll(t, inv.Body).FetchExport(context.Background(), field(t, inv.Body, "export_id")); st != source.StatusUnavailable {
				t.Fatalf("status %s, want unavailable", st)
			}
		})
	}
}

// The hub refuses a schema_version it does not have with 400 unsupported_evidence_version.
// SuperWitness asks only for v1; if the hub answers that, the evidence is unavailable and the
// adapter never asks again in another version or reads the run's other evidence as it.
func TestUnsupportedEvidenceVersionIsUnavailable(t *testing.T) {
	var asked []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/execution"), strings.HasPrefix(r.URL.Path, "/api/evidence/exports/"):
			asked = append(asked, r.URL.Query().Get("schema_version"))
			w.WriteHeader(400)
			fmt.Fprint(w, `{"error":"unsupported_evidence_version"}`)
		default:
			fmt.Fprint(w, ledgerOf("run_abcdef", "brd_abcdef", "card_abcdef"))
		}
	}))
	defer srv.Close()
	a := New(srv.URL, &auth.StaticTokenSource{Value: "t"}, srv.Client())
	ref, _ := source.NewSuperpipelineRef("brd_abcdef", "run_abcdef")
	f, st := a.Fetch(context.Background(), ref)
	if st != source.StatusOK || f.Ledger == nil || f.Execution != nil || f.ExecutionStatus != source.StatusUnavailable {
		t.Fatalf("status %s, execution %v %s", st, f.Execution != nil, f.ExecutionStatus)
	}
	if _, st := a.FetchExport(context.Background(), "00000000-0000-4000-8000-000000000001"); st != source.StatusUnavailable {
		t.Fatalf("export: %s", st)
	}
	if strings.Join(asked, ",") != source.ExecutionSchemaV1+","+source.ExecutionExportSchemaV1 {
		t.Fatalf("asked for %v: one v1 request each, nothing else", asked)
	}
}

func TestTranscriptEveryCase(t *testing.T) {
	for _, op := range []string{"readTranscript", "readTranscriptItem"} {
		c := evidenceCases(t, op)
		read := func(t *testing.T, body []byte) (json.RawMessage, source.SourceStatus) {
			session := field(t, body, "session_id")
			if !strings.HasPrefix(session, "acps_") {
				session = "acps_3f1c2b7e-5d4a-4c9b-8e2f-1a6b7c8d9e0f"
			}
			a := serveAll(t, body)
			if op == "readTranscript" {
				return a.TranscriptPage(context.Background(), source.TranscriptQuery{SessionID: session})
			}
			return a.TranscriptItem(context.Background(), source.ItemQuery{SessionID: session, SeqFrom: 1})
		}
		for name, body := range c.Valid {
			t.Run(op+"/"+name, func(t *testing.T) {
				got, st := read(t, body)
				if st != source.StatusOK || string(got) != string(body) {
					t.Fatalf("status %s: want the hub's body untouched", st)
				}
			})
		}
		for _, inv := range c.Invalid {
			t.Run(op+"/"+inv.Why, func(t *testing.T) {
				if got, st := read(t, inv.Body); st != source.StatusUnavailable || got != nil {
					t.Fatalf("status %s, %d bytes: want unavailable and no body", st, len(got))
				}
			})
		}
	}
}

// A page or item about another session than the one asked for is not an answer.
func TestTranscriptOfAnotherSessionIsUnavailable(t *testing.T) {
	body := evidenceCase(t, "readTranscript", "an empty session")
	if _, st := serveAll(t, body).TranscriptPage(context.Background(), source.TranscriptQuery{SessionID: "acps_00000000-0000-4000-8000-000000000000"}); st != source.StatusUnavailable {
		t.Fatalf("status %s", st)
	}
}

// The adapter's requests are the contract's routes, path for path.
func TestRoutesAreTheContracts(t *testing.T) {
	ops, err := treaty.Operations(treaty.Evidence)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = append(got, r.Method+" "+r.URL.Path)
		if strings.HasPrefix(r.URL.Path, "/api/evidence/runs/") && !strings.HasSuffix(r.URL.Path, "/execution") {
			fmt.Fprint(w, ledgerOf("run_R", "brd_B", "card_C"))
			return
		}
		w.WriteHeader(500)
	}))
	defer srv.Close()
	a := New(srv.URL, &auth.StaticTokenSource{Value: "t"}, srv.Client())
	ref, _ := source.NewSuperpipelineRef("brd_B", "run_R")
	const export, attempt, principal, session = "00000000-0000-4000-8000-000000000001", "attempt_A", "prn_P", "acps_S"
	a.Fetch(context.Background(), ref)
	a.FetchExport(context.Background(), export)
	a.ResolveAttempt(context.Background(), attempt)
	_, _ = a.Lookup(context.Background(), principal)
	a.TranscriptPage(context.Background(), source.TranscriptQuery{SessionID: session})
	a.TranscriptItem(context.Background(), source.ItemQuery{SessionID: session, SeqFrom: 7})
	fill := strings.NewReplacer("{source}", "superpipeline", "{externalRunId}", "run_R", "{exportId}", export,
		"{attemptId}", attempt, "{principalId}", principal, "{sessionId}", session, "{seqFrom}", strconv.Itoa(7))
	var want []string
	for _, op := range []string{"readRunEvidence", "readExecutionEvidence", "readExecutionExport", "readAttemptRun", "readEvidencePrincipal", "readTranscript", "readTranscriptItem"} {
		o, ok := ops[op]
		if !ok {
			t.Fatalf("the contract has no %s", op)
		}
		want = append(want, o.Method+" "+fill.Replace(o.Path))
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("requests:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}
