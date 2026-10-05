package canary

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"
)

type fakeClock struct{ t time.Time }

func (c *fakeClock) Now() time.Time { return c.t }
func (c *fakeClock) Sleep(_ context.Context, d time.Duration) error {
	c.t = c.t.Add(d)
	return nil
}

type fakeSP struct {
	createErr error
	specs     []any
	script    [][]SPAttempt
	calls     int
}

func (f *fakeSP) CreateCard(_ context.Context, _, _ string, spec any) (string, error) {
	if f.createErr != nil {
		return "", f.createErr
	}
	f.specs = append(f.specs, spec)
	return "crd_1", nil
}

func (f *fakeSP) Attempts(context.Context, string, string) ([]SPAttempt, error) {
	if len(f.script) == 0 {
		return nil, nil
	}
	i := f.calls
	if i >= len(f.script) {
		i = len(f.script) - 1
	}
	f.calls++
	return f.script[i], nil
}

type fakeAPI struct {
	docs    []RunDoc
	byRun   map[string]RunDoc
	calls   int
	lines   []LogLine
	logsErr error
}

func (f *fakeAPI) GetRun(_ context.Context, _, runID string) (RunDoc, error) {
	if d, ok := f.byRun[runID]; ok {
		return d, nil
	}
	if len(f.docs) == 0 {
		return RunDoc{}, errors.New("HTTP 404")
	}
	i := f.calls
	if i >= len(f.docs) {
		i = len(f.docs) - 1
	}
	f.calls++
	return f.docs[i], nil
}

func (f *fakeAPI) Logs(context.Context, string, string) ([]LogLine, error) { return f.lines, f.logsErr }

type fakeMCP struct {
	verdict  Verdict
	err      error
	doc      RunDoc
	recorded []VerdictInput
}

func (f *fakeMCP) RecordVerdict(_ context.Context, in VerdictInput) (Verdict, error) {
	f.recorded = append(f.recorded, in)
	return f.verdict, f.err
}
func (f *fakeMCP) GetRun(context.Context, string, string) (RunDoc, error) { return f.doc, nil }

type fakeTraces struct {
	spans    []Span
	traceErr error
	found    []Span
	findErr  error
	scan     ScanResult
	scanErr  error
}

func (f *fakeTraces) Trace(context.Context, string) ([]Span, error) { return f.spans, f.traceErr }
func (f *fakeTraces) FindByTag(_ context.Context, service, key, value string, _, _ time.Time) ([]Span, string, error) {
	return f.found, "GET /select/jaeger/api/traces?service=" + service + "&tags=" + key + ":" + value, f.findErr
}
func (f *fakeTraces) Scan(context.Context, time.Time, time.Time, map[string]string) (ScanResult, error) {
	return f.scan, f.scanErr
}

type fakeLogs struct {
	scan  ScanResult
	err   error
	start time.Time
}

func (f *fakeLogs) Scan(_ context.Context, start, _ time.Time, _ map[string]string) (ScanResult, error) {
	f.start = start
	return f.scan, f.err
}

type fakeErrors struct {
	err      error
	injected []string
}

func (f *fakeErrors) InjectError(_ context.Context, runID, traceID, nonce string, _ time.Time) error {
	f.injected = append(f.injected, runID+"|"+traceID+"|"+nonce)
	return f.err
}

type fakes struct {
	clock  *fakeClock
	sp     *fakeSP
	api    *fakeAPI
	mcp    *fakeMCP
	traces *fakeTraces
	logs   *fakeLogs
	errs   *fakeErrors
}

func strp(s string) *string { return &s }

func ended(runID, outcome string) SPAttempt {
	return SPAttempt{RunID: runID, StageKey: "work", AgentID: "agt_kai", Status: "ended", Outcome: strp(outcome)}
}

func started(runID string) SPAttempt {
	return SPAttempt{RunID: runID, StageKey: "work", AgentID: "agt_kai", Status: "started"}
}

func controlScan() ScanResult {
	s := newScanResult()
	s.Scanned = 100
	s.Hits[LabelControlRun] = 1
	return s
}

var t0 = time.Date(2026, 10, 10, 2, 30, 0, 0, time.UTC)

const zeroPlain = "swcanary0000000000000000"
const zeroMarker = "sk-swcanary-000000000000000000000000"

func testConfig() Config {
	return Config{BoardID: "brd_c", WorkStage: "work", GateStage: "review", ExpectHarness: "hermes",
		Standard: "rubric:superwitness-canary@1", OTLPLogsURL: "http://127.0.0.1:4318/v1/logs",
		ClaimTimeout: 10 * time.Minute, RunTimeout: 30 * time.Minute, SettleDelay: 90 * time.Second,
		DocTimeout: 5 * time.Minute, PollInterval: 15 * time.Second}
}

func goodDeps() (Deps, *fakes) {
	f := &fakes{
		clock:  &fakeClock{t: t0},
		sp:     &fakeSP{script: [][]SPAttempt{{}, {started("run_1")}, {ended("run_1", "completed")}}},
		api:    &fakeAPI{docs: []RunDoc{goodDoc()}, lines: goodLogs()},
		mcp:    &fakeMCP{verdict: Verdict{ID: "vrd_1", Judge: "prn_canary", JudgeKind: "grader"}, doc: gradedDoc()},
		traces: &fakeTraces{spans: goodSpans(), found: workersSpans(), scan: controlScan()},
		logs:   &fakeLogs{scan: controlScan()},
		errs:   &fakeErrors{},
	}
	return Deps{SP: f.sp, API: f.api, MCP: f.mcp, Traces: f.traces, Logs: f.logs, Errors: f.errs,
		OTLPLogsURL: "http://127.0.0.1:4318/v1/logs", Principal: "prn_canary",
		Now: f.clock.Now, Sleep: f.clock.Sleep, Rand: zeroRand()}, f
}

func checkByID(r Result, id string) Check {
	for _, c := range r.Checks {
		if c.ID == id {
			return c
		}
	}
	return Check{}
}

func TestRunPassesAndProvesEightFlows(t *testing.T) {
	d, f := goodDeps()
	r := Run(context.Background(), testConfig(), d, TriggerNightly)
	if !r.Pass || len(r.Checks) != 9 || r.RunID != "run_1" || r.CardID != "crd_1" || r.RunOutcome != "completed" || !r.GatePending {
		t.Fatalf("result = %+v", r)
	}
	var flows []int
	for _, p := range r.Proofs {
		flows = append(flows, p.Flow)
		if p.Query == "" || p.RunRef != "superpipeline:brd_c/run_1" {
			t.Errorf("proof without evidence: %+v", p)
		}
	}
	if fmt.Sprint(flows) != "[1 2 3 4 5 7 8 9]" {
		t.Errorf("proved flows %v", flows)
	}
	spec, _ := json.Marshal(f.sp.specs[0])
	if !strings.Contains(string(spec), zeroPlain) || !strings.Contains(string(spec), zeroMarker) {
		t.Errorf("markers not planted in the spec: %s", spec)
	}
	if len(f.errs.injected) != 1 || f.errs.injected[0] != "run_1|t1|"+testNonce {
		t.Errorf("injected = %v", f.errs.injected)
	}
	if !f.logs.start.Equal(t0.Add(-time.Minute)) {
		t.Errorf("log window starts %v", f.logs.start)
	}
	if len(f.mcp.recorded) != 2 || f.mcp.recorded[0].IdempotencyKey != "superwitness-canary:run_1" ||
		f.mcp.recorded[0].SubjectRef != "superpipeline:brd_c/run_1" {
		t.Errorf("recorded = %+v", f.mcp.recorded)
	}
	if q := checkByID(r, "5").Query; !strings.Contains(q, "service=superpipeline-api") {
		t.Errorf("flow 5 query = %q", q)
	}
}

func TestRunFailsWhenCardNeverClaimed(t *testing.T) {
	d, f := goodDeps()
	f.sp.script = [][]SPAttempt{{}}
	r := Run(context.Background(), testConfig(), d, TriggerNightly)
	if r.Pass || !strings.Contains(r.Error, "not claimed") || len(r.Checks) != 0 || len(r.Proofs) != 0 {
		t.Fatalf("result = %+v", r)
	}
	if took := f.clock.t.Sub(t0); took < 10*time.Minute || took > 10*time.Minute+15*time.Second {
		t.Errorf("gave up after %v", took)
	}
}

func TestRunFailsWhenRunNeverEnds(t *testing.T) {
	d, f := goodDeps()
	f.sp.script = [][]SPAttempt{{started("run_1")}}
	r := Run(context.Background(), testConfig(), d, TriggerNightly)
	if r.Pass || !strings.Contains(r.Error, "did not end") {
		t.Fatalf("result = %+v", r)
	}
}

func TestRunWaitsPastAReclaimedAttempt(t *testing.T) {
	d, f := goodDeps()
	f.sp.script = [][]SPAttempt{{ended("run_0", "reclaimed")}, {ended("run_0", "reclaimed"), ended("run_1", "completed")}}
	r := Run(context.Background(), testConfig(), d, TriggerNightly)
	if r.RunID != "run_1" || !r.Pass {
		t.Fatalf("result = %+v", r)
	}
}

func TestRunNamesTheLastOutcomeWhenNoRunCompletes(t *testing.T) {
	d, f := goodDeps()
	f.sp.script = [][]SPAttempt{{ended("run_0", "failed")}}
	r := Run(context.Background(), testConfig(), d, TriggerNightly)
	if !strings.Contains(r.Error, "last run outcome failed") {
		t.Fatalf("error = %q", r.Error)
	}
}

func TestRunPollsTheDocumentUntilTheTraceJoins(t *testing.T) {
	d, f := goodDeps()
	early := goodDoc()
	early.Trace = DocTrace{Status: "none"}
	f.api.docs = []RunDoc{early, goodDoc()}
	r := Run(context.Background(), testConfig(), d, TriggerNightly)
	if !r.Pass || f.api.calls != 3 { // two until settled, one to see the injected error
		t.Fatalf("pass=%v calls=%d result=%+v", r.Pass, f.api.calls, r)
	}
}

func TestRunChecksTheLastDocumentWhenItNeverSettles(t *testing.T) {
	d, f := goodDeps()
	early := goodDoc()
	early.Trace = DocTrace{Status: "none"}
	f.api.docs = []RunDoc{early}
	r := Run(context.Background(), testConfig(), d, TriggerNightly)
	if r.Pass || checkByID(r, "3").Pass || r.Error != "" {
		t.Fatalf("result = %+v", r)
	}
}

func TestRunWaitsForTheInjectedErrorToAppear(t *testing.T) {
	d, f := goodDeps()
	noErr := goodDoc()
	noErr.Errors = nil
	f.api.docs = []RunDoc{noErr, noErr, goodDoc()}
	r := Run(context.Background(), testConfig(), d, TriggerNightly)
	if !checkByID(r, "8").Pass || f.api.calls != 3 {
		t.Fatalf("calls=%d flow 8 = %+v", f.api.calls, checkByID(r, "8"))
	}
}

func TestRunIsolatesFailuresToTheirFlow(t *testing.T) {
	cases := map[string]struct {
		breakIt func(*fakes)
		flow    string
	}{
		"MCP refusal":       {func(f *fakes) { f.mcp.err = errors.New("record_verdict refused: subject_unresolved") }, "7"},
		"injection refused": {func(f *fakes) { f.errs.err = errors.New("OTLP logs: HTTP 503") }, "8"},
		"no node-agent":     {func(f *fakes) { f.traces.spans = without(goodSpans(), "acp session/prompt") }, "4"},
		"no Workers span":   {func(f *fakes) { f.traces.found = nil }, "5"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			d, f := goodDeps()
			tc.breakIt(f)
			r := Run(context.Background(), testConfig(), d, TriggerNightly)
			for _, c := range r.Checks {
				if (c.ID == tc.flow) == c.Pass {
					t.Errorf("flow %s pass=%v detail=%q", c.ID, c.Pass, c.Detail)
				}
			}
			for _, p := range r.Proofs {
				if fmt.Sprint(p.Flow) == tc.flow {
					t.Errorf("failed flow %s was proved", tc.flow)
				}
			}
		})
	}
}

func TestRunRedactsMarkerFromEveryDetail(t *testing.T) {
	d, f := goodDeps()
	f.traces.traceErr = fmt.Errorf("trace echoed %s and %s", zeroPlain, zeroMarker)
	r := Run(context.Background(), testConfig(), d, TriggerNightly)
	b, _ := json.Marshal(r)
	if strings.Contains(string(b), zeroPlain) || strings.Contains(string(b), zeroMarker) {
		t.Fatalf("marker in result: %s", b)
	}
	d2, g := goodDeps()
	g.sp.createErr = fmt.Errorf("refused spec containing %s", zeroPlain)
	r2 := Run(context.Background(), testConfig(), d2, TriggerNightly)
	if !strings.Contains(r2.Error, "<marker_plain>") {
		t.Fatalf("error = %q", r2.Error)
	}
}

func priorPass(runID string, at time.Time) Result {
	r := Result{V: ResultVersion, Trigger: TriggerNightly, StartedAt: at, BoardID: "brd_c", RunID: runID,
		GatePending: true, Checks: allPassing()}
	r.Finish(at)
	return r
}

func decidedDoc(judge, kind string) RunDoc {
	d := goodDoc()
	d.Verdicts[0].Value = map[string]any{"decision": "approved"}
	d.Verdicts[0].Status = "resolved"
	d.Verdicts[0].Judge = judge
	d.Verdicts[0].JudgeKind = kind
	return d
}

func agentUnknownDoc() RunDoc {
	d := decidedDoc("prn_op", "human")
	d.Run.Agent = "unknown"
	return d
}

func cancelledDoc() RunDoc {
	d := goodDoc()
	d.Verdicts[0].Status = "cancelled"
	return d
}

func unknownDoc() RunDoc {
	d := goodDoc()
	d.Verdicts[0].Status = "unknown"
	return d
}

func TestFollowUpGates(t *testing.T) {
	cases := map[string]struct {
		doc        RunDoc
		sawPending bool
		resolved   int
		pass       bool
		detail     string
	}{
		"human approved":         {decidedDoc("prn_op", "human"), true, 1, true, "human"},
		"agent decided":          {decidedDoc("prn_x", "agent"), true, 1, false, "judge_kind agent"},
		"executor":               {decidedDoc("prn_exec01", "human"), true, 1, false, "executing agent"},
		"still pending":          {goodDoc(), true, 0, false, ""},
		"pending never observed": {decidedDoc("prn_op", "human"), false, 0, false, ""},
		"agent mapping unknown":  {agentUnknownDoc(), true, 0, false, ""},
		"judge kind unknown":     {decidedDoc("prn_op", "unknown"), true, 0, false, ""},
		"cancelled":              {cancelledDoc(), true, 1, false, "cancelled"},
		"unknown status":         {unknownDoc(), true, 0, false, ""},
		"own gate beats a later": {laterGateFirstDoc(), true, 1, true, "human"},
		// Only a gate tied to the run counts.
		"own gate absent, another run's decided": {otherRunsGateDoc(), true, 0, false, ""},
		"single null-run_id gate decided":        {nullRunGatesDoc(1), true, 1, true, "human"},
		"two null-run_id gates":                  {nullRunGatesDoc(2), true, 0, false, ""},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			d, f := goodDeps()
			f.api.byRun = map[string]RunDoc{"run_old": tc.doc}
			prior := priorPass("run_old", t0.Add(-24*time.Hour))
			prior.GatePending = tc.sawPending
			d.Prior = []Result{prior}
			got, proofs := FollowUpGates(context.Background(), testConfig(), d)
			if len(got) != tc.resolved {
				t.Fatalf("got %+v", got)
			}
			wantProofs := 0
			if tc.pass {
				wantProofs = 1
			}
			if len(proofs) != wantProofs {
				t.Fatalf("proofs %+v", proofs)
			}
			if tc.resolved == 1 && (got[0].Pass != tc.pass || !strings.Contains(got[0].Detail, tc.detail) || got[0].RunID != "run_old") {
				t.Fatalf("got %+v", got[0])
			}
			if tc.pass && (proofs[0].Flow != 6 || proofs[0].RunRef != "superpipeline:brd_c/run_old" ||
				!strings.Contains(proofs[0].Output, "gate_1") || proofs[0].Query == "") {
				t.Fatalf("proof %+v", proofs[0])
			}
		})
	}
}

func TestFollowUpGatesSkipsAResolvedRun(t *testing.T) {
	d, f := goodDeps()
	f.api.byRun = map[string]RunDoc{"run_old": decidedDoc("prn_op", "human")}
	resolved := priorPass("run_new", t0.Add(-time.Hour))
	resolved.GateResolutions = []GateResolution{{RunID: "run_old", Pass: true}}
	d.Prior = []Result{priorPass("run_old", t0.Add(-24*time.Hour)), resolved}
	got, _ := FollowUpGates(context.Background(), testConfig(), d)
	for _, g := range got {
		if g.RunID == "run_old" {
			t.Fatalf("run_old resolved twice: %+v", g)
		}
	}
}

// laterGateFirstDoc lists a newer, still-pending gate for another run ahead of run_old's
// own decided gate: run_old's gate must be the one followed.
func laterGateFirstDoc() RunDoc {
	d := decidedDoc("prn_op", "human")
	d.Verdicts[0].RunID = "run_old"
	d.Verdicts[0].At = "2026-10-04T10:00:00Z"
	later := DocVerdict{ID: "gate_2", Kind: "gate", Source: "superpipeline", Standard: d.Verdicts[0].Standard,
		Value: map[string]any{"decision": nil}, Status: "pending", Judge: "unknown", JudgeKind: "unknown",
		RunID: "run_new", At: "2026-10-05T10:00:00Z"}
	d.Verdicts = append([]DocVerdict{later}, d.Verdicts...)
	return d
}

// otherRunsGateDoc: run_old's own gate is absent; a later run's gate on the card was decided
// by a human. That decision is not run_old's, so it proves nothing.
func otherRunsGateDoc() RunDoc {
	d := decidedDoc("prn_op", "human")
	d.Verdicts[0].RunID = "run_new"
	d.Verdicts[0].At = "2026-10-05T10:00:00Z"
	return d
}

// nullRunGatesDoc: n human-decided review gates with no run_id (advanceCard gates). One is the
// card's only gate, so it is tied to the run; two are ambiguous.
func nullRunGatesDoc(n int) RunDoc {
	d := decidedDoc("prn_op", "human")
	d.Verdicts[0].RunID = "unknown"
	d.Verdicts[0].At = "2026-10-04T10:00:00Z"
	for i := 1; i < n; i++ {
		g := d.Verdicts[0]
		g.ID = fmt.Sprintf("gate_%d", i+1)
		g.At = fmt.Sprintf("2026-10-04T1%d:00:00Z", i)
		d.Verdicts = append(d.Verdicts, g)
	}
	return d
}

func TestDocSettledNeedsAGateTiedToTheRun(t *testing.T) {
	own := goodDoc()
	if !docSettled(own, "review", "run_1") {
		t.Error("the card's only gate did not settle the document")
	}
	other := goodDoc()
	other.Verdicts[0].RunID = "run_0"
	if docSettled(other, "review", "run_1") {
		t.Error("another run's gate settled the document")
	}
	two := nullRunGatesDoc(2)
	if docSettled(two, "review", "run_1") {
		t.Error("two unattributed gates settled the document")
	}
}
