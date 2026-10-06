package canary

// Contract tests: the canary's real clients and checks against the service's real handler
// (internal/api, internal/mcp, internal/join) over the development data set
// (internal/source/fake). Every other canary test runs against fakes written from the
// documented contracts; these prove the canary and the service agree on the wire today.

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/SuperJackfruitLabs/superwitness/internal/api"
	"github.com/SuperJackfruitLabs/superwitness/internal/auth"
	"github.com/SuperJackfruitLabs/superwitness/internal/contracts"
	"github.com/SuperJackfruitLabs/superwitness/internal/join"
	"github.com/SuperJackfruitLabs/superwitness/internal/mcp"
	"github.com/SuperJackfruitLabs/superwitness/internal/source"
	"github.com/SuperJackfruitLabs/superwitness/internal/source/fake"
	"github.com/SuperJackfruitLabs/superwitness/internal/verdicts"
)

const (
	contractPrincipal = "prn_canary"
	contractStandard  = "rubric:superwitness-canary@1"
	// DevAuthenticator's token shape: the caller is a service principal, as the canary is.
	contractToken = "dev:" + contractPrincipal + ":service"
)

type contractEnv struct {
	dev   *fake.Dev
	store *verdicts.MemStore
	sw    SuperwitnessClient
	mcp   *MCPClient
}

// newContractEnv serves the service's handler exactly as api_test and mcp/server_test wire it, with
// the MCP route on the same server, as app.Build mounts it. mutate may change the dev data
// before the first request.
func newContractEnv(t *testing.T, mutate func(*fake.Dev)) contractEnv {
	t.Helper()
	d, err := fake.NewDev()
	if err != nil {
		t.Fatal(err)
	}
	if mutate != nil {
		mutate(d)
	}
	store := verdicts.NewMemStore()
	if err := store.InsertRubric(context.Background(), verdicts.Rubric{ID: "superwitness-canary", Version: 1,
		Name: "superwitness canary round trip", Scale: json.RawMessage(`{"score":[0,1]}`), CreatedBy: "prn_operator"}); err != nil {
		t.Fatal(err)
	}
	ops := &api.Ops{
		Join: &join.Joiner{Superpipeline: d.SP, AgentPod: d.AP, Traces: d.Traces, Logs: d.Logs, Errors: d.Errors,
			Verdicts: store, Principals: d},
		Spans: d, Logs: d, Attempts: d,
		Verdicts: &verdicts.Service{Store: store, Subjects: &join.Subjects{Superpipeline: d.SP, AgentPod: d.AP, Attempts: d}},
	}
	srv := httptest.NewServer((&api.Server{Ops: ops, Auth: auth.DevAuthenticator{}, MCP: mcp.NewHandler(ops, "contract")}).Handler())
	t.Cleanup(srv.Close)
	tok := staticToken(contractToken)
	return contractEnv{dev: d, store: store,
		sw:  SuperwitnessClient{BaseURL: srv.URL, Tokens: tok},
		mcp: &MCPClient{URL: srv.URL + "/mcp", Tokens: tok}}
}

// canaryVerdict is the input runFlow7 builds (run.go), for the dev run.
func canaryVerdict(key string) VerdictInput {
	return VerdictInput{
		IdempotencyKey: key,
		SubjectKind:    "run",
		SubjectRef:     "superpipeline:" + fake.DevBoard + "/" + fake.DevRun,
		Standard:       contractStandard,
		Value:          map[string]any{"score": 1.0},
		Comment:        "superwitness canary: a round-trip check, not a judgement of the work",
	}
}

func ctxT(t *testing.T) context.Context {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
	return ctx
}

func TestContractGetRunDecodesWS5Document(t *testing.T) {
	env := newContractEnv(t, nil)
	doc, err := env.sw.GetRun(ctxT(t), fake.DevBoard, fake.DevRun)
	if err != nil {
		t.Fatalf("GetRun against the service: %v", err)
	}

	if want := "superpipeline:" + fake.DevBoard + "/" + fake.DevRun; doc.Run.Ref != want {
		t.Errorf("run.ref = %q, want %q", doc.Run.Ref, want)
	}
	if doc.Run.Card.ID != "crd_01" || doc.Run.Card.Title != "Write the release note" {
		t.Errorf("run.card = %+v", doc.Run.Card)
	}
	if doc.Run.Stage != "draft" || doc.Run.Agent != fake.DevExecutor || doc.Run.State != "completed" {
		t.Errorf("run = %+v", doc.Run)
	}
	for _, name := range []string{"superpipeline", "agentpod", "traces", "logs", "errors", "verdicts"} {
		if doc.Source(name) != "ok" {
			t.Errorf("sources.%s = %s", name, doc.Source(name))
		}
	}
	if len(doc.Attempts) != 1 {
		t.Fatalf("attempts = %+v", doc.Attempts)
	}
	a := doc.Attempts[0]
	if a.ID != fake.DevAttempt || a.Station != "stn_01" || a.State != "completed" {
		t.Errorf("attempt = %+v", a)
	}
	if n, ok := a.SpanCount(); !ok || n != 5 {
		t.Errorf("span_count = %s, want 5 (the attempt, turn, two tool call and permission spans)", a.SpanCountRaw)
	}
	fp := a.Fingerprint
	if fp.Harness != "hermes" || fp.HarnessVersion != "unknown" || fp.Model != "unknown" || fp.ReportedBy != "hub" {
		t.Errorf("fingerprint = %+v", fp)
	}
	// Two trace ids (hub + Workers) and a dispatch root: the service says partial, sampled true.
	if doc.Trace.Status != "partial" || doc.Trace.SampledState() != "true" ||
		!reflect.DeepEqual(doc.Trace.TraceIDs, []string{contracts.TraceHub, contracts.TraceWorkers}) {
		t.Errorf("trace = %+v (sampled %s)", doc.Trace, doc.Trace.SampledState())
	}
	if len(doc.Errors) != 1 || doc.Errors[0].Service != "agentpod-hub" || doc.Errors[0].TraceID != contracts.TraceHub ||
		doc.Errors[0].At == "" {
		t.Errorf("errors = %+v", doc.Errors)
	}
	if countGates(doc) != 4 {
		t.Errorf("gates = %d, want the fixture's 4", countGates(doc))
	}

	// The checks the canary runs on this document read it as the service meant it.
	if c := CheckRunEvidence(doc, fake.DevBoard, fake.DevRun, "crd_01", "draft"); !c.Pass {
		t.Errorf("flow 1 on the service's document: %s", c.Detail)
	}
	// Flow 2 recomputes the fingerprint digest from the service's fields: the hub fixture's digest must match.
	if c := CheckAttempts(doc, "hermes"); !c.Pass || !strings.Contains(c.Detail, "unknown fields: attempt_01.harness_version,attempt_01.model") {
		t.Errorf("flow 2 on the service's document: pass=%v %s", c.Pass, c.Detail)
	}
}

// When a source cannot answer, the service writes the string "unknown" where a number or flag
// would be. The canary must still decode the document and read those as not-known.
func TestContractGetRunDecodesUnknownCountsAndFlags(t *testing.T) {
	env := newContractEnv(t, func(d *fake.Dev) {
		d.Traces.Status = source.StatusUnavailable
		d.Logs.Status = source.StatusTimeout
	})
	doc, err := env.sw.GetRun(ctxT(t), fake.DevBoard, fake.DevRun)
	if err != nil {
		t.Fatalf("GetRun with traces unavailable: %v", err)
	}
	if got := doc.Trace.SampledState(); got != "unknown" {
		t.Errorf("trace.sampled = %q (raw %s), want unknown", got, doc.Trace.Sampled)
	}
	if doc.Trace.Status != "unknown" || doc.Source("traces") != "unavailable" || doc.Source("logs") != "timeout" {
		t.Errorf("trace.status %s, sources %v", doc.Trace.Status, doc.Sources)
	}
	if len(doc.Attempts) != 1 {
		t.Fatalf("attempts = %+v", doc.Attempts)
	}
	if n, ok := doc.Attempts[0].SpanCount(); ok {
		t.Errorf("span_count = %d known; raw %s, want \"unknown\"", n, doc.Attempts[0].SpanCountRaw)
	}
	if c := CheckHubSpansJoined(doc, fake.DevRun, nil); c.Pass || c.Detail != "sources.traces is unavailable" {
		t.Errorf("flow 3 with traces unavailable: pass=%v %s", c.Pass, c.Detail)
	}
}

func TestContractLogsDecodesC6bPage(t *testing.T) {
	env := newContractEnv(t, nil)
	ctx := ctxT(t)
	lines, err := env.sw.Logs(ctx, fake.DevBoard, fake.DevRun)
	if err != nil {
		t.Fatalf("Logs against the service: %v", err)
	}
	if len(lines) != 2 {
		t.Fatalf("lines = %+v", lines)
	}
	first := lines[0]
	if first.Service != "agentpod-hub" || first.Level != "info" || first.Message != "attempt opened" ||
		first.TraceID != contracts.TraceHub || first.RunID != fake.DevRun {
		t.Errorf("line 0 = %+v", first)
	}
	if _, err := time.Parse(time.RFC3339Nano, first.At); err != nil {
		t.Errorf("line 0 at = %q: %v", first.At, err)
	}
	if lines[1].Level != "error" || lines[1].RunID != "" || lines[1].TraceID != contracts.TraceHub {
		t.Errorf("line 1 (trace-linked only) = %+v", lines[1])
	}

	// Flow 8 counts both lines as linked (one by run.id, one by trace id); it fails only
	// because the dev data has no canary-injected error.
	doc, err := env.sw.GetRun(ctx, fake.DevBoard, fake.DevRun)
	if err != nil {
		t.Fatal(err)
	}
	c := CheckLinkedLogsAndErrors(doc, lines, nil, fake.DevRun, "swcerr-00000000")
	if c.Pass || !strings.HasPrefix(c.Detail, "2 linked product log line(s), but the injected error") {
		t.Errorf("flow 8 on the service's logs: pass=%v %s", c.Pass, c.Detail)
	}
}

func TestContractMCPGetRunMatchesTheAPI(t *testing.T) {
	env := newContractEnv(t, nil)
	ctx := ctxT(t)
	viaAPI, err := env.sw.GetRun(ctx, fake.DevBoard, fake.DevRun)
	if err != nil {
		t.Fatal(err)
	}
	viaMCP, err := env.mcp.GetRun(ctx, fake.DevBoard, fake.DevRun)
	if err != nil {
		t.Fatalf("MCP get_run against the service: %v", err)
	}
	if !reflect.DeepEqual(viaAPI, viaMCP) {
		a, _ := json.Marshal(viaAPI)
		m, _ := json.Marshal(viaMCP)
		t.Errorf("API and MCP documents differ:\napi %s\nmcp %s", a, m)
	}
}

func TestContractMCPRecordVerdictRoundTrip(t *testing.T) {
	env := newContractEnv(t, nil)
	ctx := ctxT(t)
	in := canaryVerdict("superwitness-canary:" + fake.DevRun)
	first, err := env.mcp.RecordVerdict(ctx, in)
	if err != nil {
		t.Fatalf("record_verdict against the service: %v", err)
	}
	if !strings.HasPrefix(first.ID, "vrd_") || first.Judge != contractPrincipal || first.JudgeKind != "grader" {
		t.Errorf("verdict = %+v, want vrd_… by %s as grader", first, contractPrincipal)
	}
	again, err := env.mcp.RecordVerdict(ctx, in)
	if err != nil {
		t.Fatalf("retry: %v", err)
	}
	if again != first {
		t.Errorf("retry with the same key = %+v, first %+v", again, first)
	}
	doc, err := env.mcp.GetRun(ctx, fake.DevBoard, fake.DevRun)
	if err != nil {
		t.Fatal(err)
	}
	if c := CheckGraderRoundTrip(first, again, doc, contractPrincipal, contractStandard); !c.Pass {
		t.Errorf("flow 7 on the service's answers: %s", c.Detail)
	}
	var owned *DocVerdict
	for i, v := range doc.Verdicts {
		if v.ID == first.ID {
			owned = &doc.Verdicts[i]
		}
	}
	if owned == nil || owned.Source != "superwitness" || owned.Kind != "grader" || owned.Value["score"] != 1.0 || owned.At == "" {
		t.Errorf("recorded verdict on the document = %+v", owned)
	}
}

func TestContractMCPRecordVerdictRefusalsNameTheCode(t *testing.T) {
	t.Run("unknown rubric", func(t *testing.T) {
		env := newContractEnv(t, nil)
		in := canaryVerdict("k-rubric")
		in.Standard = "rubric:superwitness-canary@2"
		_, err := env.mcp.RecordVerdict(ctxT(t), in)
		if err == nil || !strings.Contains(err.Error(), "unknown_rubric") {
			t.Errorf("err = %v, want unknown_rubric", err)
		}
	})
	t.Run("agent unmapped", func(t *testing.T) {
		env := newContractEnv(t, func(d *fake.Dev) {
			run := *d.SP.Fragment.Run
			run.Run.AgentPrincipalID = ""
			d.SP.Fragment.Run = &run
			led := *d.AP.Fragment.Ledger
			led.Attempts = append([]source.LedgerAttempt(nil), led.Attempts...)
			led.Attempts[0].AgentPrincipalID = ""
			d.AP.Fragment.Ledger = &led
		})
		_, err := env.mcp.RecordVerdict(ctxT(t), canaryVerdict("k-unresolved"))
		if err == nil || !strings.Contains(err.Error(), "subject_unresolved") {
			t.Errorf("err = %v, want subject_unresolved", err)
		}
	})
	t.Run("same key, different verdict", func(t *testing.T) {
		env := newContractEnv(t, nil)
		ctx := ctxT(t)
		if _, err := env.mcp.RecordVerdict(ctx, canaryVerdict("k-conflict")); err != nil {
			t.Fatal(err)
		}
		in := canaryVerdict("k-conflict")
		in.Value = map[string]any{"score": 0.5}
		_, err := env.mcp.RecordVerdict(ctx, in)
		if err == nil || !strings.Contains(err.Error(), "idempotency_conflict") {
			t.Errorf("err = %v, want idempotency_conflict", err)
		}
	})
}

// Gate views against the service's real handler: the dev run's four stage:draft gates are
// resolved (by principal id), resolved (by hub sub), pending and cancelled.
func TestContractGateViewsRead(t *testing.T) {
	env := newContractEnv(t, func(d *fake.Dev) {
		run := *d.SP.Fragment.Run
		run.Gates = append([]source.SPGate(nil), run.Gates...)
		g := run.Gates[3] // a fifth gate in a status this build of the service does not know
		g.ID, g.Status = "gate_05", "escalated"
		run.Gates = append(run.Gates, g)
		d.SP.Fragment.Run = &run
	})
	doc, err := env.sw.GetRun(ctxT(t), fake.DevBoard, fake.DevRun)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]struct{ status, decision, judge, judgeKind string }{
		"gate_01": {"resolved", "rejected", fake.DevHuman, "human"},
		"gate_02": {"resolved", "changes_requested", fake.DevHuman2, "human"},
		"gate_03": {"pending", "pending", "unknown", "unknown"},
		"gate_04": {"cancelled", "cancelled", "unknown", "unknown"},
		"gate_05": {"unknown", "unknown", "unknown", "unknown"},
	}
	seen := 0
	for _, v := range doc.Verdicts {
		if v.Kind != "gate" {
			continue
		}
		seen++
		w, ok := want[v.ID]
		if !ok {
			t.Errorf("unexpected gate %+v", v)
			continue
		}
		if v.Source != "superpipeline" || v.Standard != "stage:draft" || v.Status != w.status ||
			gateDecision(v) != w.decision || v.Judge != w.judge || v.JudgeKind != w.judgeKind || v.At == "" {
			t.Errorf("%s: status %q decision %q judge %q/%q at %q; want %+v",
				v.ID, v.Status, gateDecision(v), v.Judge, v.JudgeKind, v.At, w)
		}
		if _, present := v.Value["decision"]; !present {
			t.Errorf("%s: value %v has no decision key", v.ID, v.Value)
		}
	}
	if seen != len(want) {
		t.Errorf("saw %d gates, want %d", seen, len(want))
	}

	// findGate on the service's real gates: this run's gates (gate_01, gate_03, gate_04) beat the card-level
	// gate_02 and the unknown-status gate_05; the latest of them is gate_04, cancelled at 10:45.
	g, ok := findGate(doc, "draft", fake.DevRun)
	if !ok || g.ID != "gate_04" || g.RunID != fake.DevRun {
		t.Errorf("findGate(this run) = %+v %v, want gate_04", g, ok)
	}
	if c, pending := CheckGateShown(doc, "draft", fake.DevRun); c.Pass || pending || !strings.Contains(c.Detail, "status cancelled") {
		t.Errorf("flow 6 on gate_04: pass=%v pending=%v %s", c.Pass, pending, c.Detail)
	}
	if g, ok := runGate(doc, "draft", fake.DevRun); !ok || g.ID != "gate_04" {
		t.Errorf("runGate(this run) = %+v %v, want gate_04", g, ok)
	}
	// For another run, findGate still names the latest gate (gate_04 ties gate_05 at
	// 10:45; document order wins), but no gate is tied to that run: four gates, and the only
	// unattributed one (gate_02) is not the card's only gate.
	if g, ok := findGate(doc, "draft", "run_other"); !ok || g.ID != "gate_04" {
		t.Errorf("findGate(other run) = %+v %v, want gate_04", g, ok)
	}
	if g, ok := runGate(doc, "draft", "run_other"); ok {
		t.Errorf("runGate(other run) = %+v, want none", g)
	}
	if c, pending := CheckGateShown(doc, "draft", "run_other"); c.Pass || pending ||
		!strings.Contains(c.Detail, "no gate for stage:draft tied to run run_other") || !strings.Contains(c.Detail, "latest seen: gate_04") {
		t.Errorf("flow 6 for another run: pass=%v pending=%v %s", c.Pass, pending, c.Detail)
	}
	for _, v := range doc.Verdicts {
		if v.ID == "gate_02" && v.RunID != "unknown" {
			t.Errorf("gate_02 run_id = %q, want unknown", v.RunID)
		}
	}
}
