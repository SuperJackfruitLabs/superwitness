package join

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/SuperJackfruitLabs/superwitness/internal/source"
	"github.com/SuperJackfruitLabs/superwitness/internal/source/fake"
	"github.com/SuperJackfruitLabs/superwitness/internal/verdicts"
)

func devJoiner(t *testing.T) (*Joiner, *fake.Dev, *verdicts.MemStore) {
	t.Helper()
	d, err := fake.NewDev()
	if err != nil {
		t.Fatal(err)
	}
	store := verdicts.NewMemStore()
	j := &Joiner{Superpipeline: d.SP, AgentPod: d.AP, Traces: d.Traces, Logs: d.Logs, Errors: d.Errors,
		Verdicts: store, Principals: d, Timeout: 200 * time.Millisecond}
	return j, d, store
}

func devRef(t *testing.T) source.RunRef {
	t.Helper()
	r, err := source.NewSuperpipelineRef(fake.DevBoard, fake.DevRun)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestBuildAllSourcesOK(t *testing.T) {
	j, _, store := devJoiner(t)
	_, _, _ = store.Insert(context.Background(), verdicts.Verdict{ID: "vrd_1", IdempotencyKey: "k", Kind: "grader",
		SubjectKind: verdicts.SubjectRun, SubjectRef: "superpipeline:brd_01/run_01", Judge: "prn_grader01",
		JudgeKind: verdicts.JudgeAgent, Standard: "rubric:press@1", Value: json.RawMessage(`{"score":0.7}`),
		EvidenceRefs: json.RawMessage(`[]`), CreatedAt: time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC)})

	doc, err := j.Build(context.Background(), devRef(t))
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"superpipeline", "agentpod", "traces", "logs", "errors", "verdicts"} {
		if doc.Sources[name] != source.StatusOK {
			t.Errorf("sources.%s = %s", name, doc.Sources[name])
		}
	}
	if doc.Run.Ref != "superpipeline:brd_01/run_01" || doc.Run.Stage != "draft" || doc.Run.State != "completed" ||
		doc.Run.Agent != fake.DevExecutor || doc.Run.AgentRef != "agt_01" || doc.Run.Card.Title != "Write the release note" {
		t.Errorf("run = %+v", doc.Run)
	}
	if len(doc.Attempts) != 1 || doc.Attempts[0].Fingerprint.Harness != "hermes" ||
		doc.Attempts[0].SpanCount != KnownCount(2) || doc.Attempts[0].Station != "stn_01" {
		t.Errorf("attempts = %+v", doc.Attempts)
	}
	if doc.Trace.Status != TracePartial || len(doc.Trace.TraceIDs) != 2 {
		t.Errorf("trace = %+v", doc.Trace) // the dev set has a Workers span in its own trace
	}
	if len(doc.Errors) != 1 || doc.Errors[0].Service != "agentpod-hub" {
		t.Errorf("errors = %+v", doc.Errors)
	}
	if doc.Cost.Status != "unreported" || doc.Cost.InputTokens != nil {
		t.Errorf("cost = %+v", doc.Cost)
	}
	if doc.LogCount != KnownCount(2) {
		t.Errorf("log_count = %+v", doc.LogCount)
	}
	// gate_01 (rejected, decided_by_principal_id), gate_02 (changes_requested, run_id null,
	// decider resolved from decided_by_hub_sub), gate_03 (pending), gate_04 (cancelled), then the owned grader verdict.
	if len(doc.Verdicts) != 5 {
		t.Fatalf("verdicts = %+v", doc.Verdicts)
	}
	g1, g2, g3, g4, own := doc.Verdicts[0], doc.Verdicts[1], doc.Verdicts[2], doc.Verdicts[3], doc.Verdicts[4]
	if g1.ID != "gate_01" || g1.Kind != "gate" || g1.Judge != fake.DevHuman || g1.JudgeKind != "human" ||
		g1.Standard != "stage:draft" || string(g1.Value) != `{"decision":"rejected"}` || g1.RunID != "run_01" {
		t.Errorf("gate_01 = %+v", g1)
	}
	if g2.RunID != Unknown || g2.Judge != fake.DevHuman2 || g2.JudgeKind != "human" || g2.Status != "resolved" ||
		string(g2.Value) != `{"decision":"changes_requested"}` {
		t.Errorf("gate_02 = %+v", g2)
	}
	if string(g3.Value) != `{"decision":null}` || g3.Status != "pending" || g3.Judge != Unknown || g3.JudgeKind != Unknown {
		t.Errorf("pending gate = %+v", g3)
	}
	// a cancelled gate is never shown as pending; At falls back to resolved_at
	if g4.ID != "gate_04" || g4.Status != "cancelled" || string(g4.Value) != `{"decision":null}` ||
		g4.At != "2026-10-04T10:45:00Z" || g4.Judge != Unknown || g4.JudgeKind != Unknown {
		t.Errorf("cancelled gate = %+v", g4)
	}
	if own.ID != "vrd_1" || own.Source != "superwitness" || own.Kind != "grader" || own.JudgeKind != "agent" {
		t.Errorf("owned = %+v", own)
	}
}

func TestUnmappedGateDeciderShowsRawID(t *testing.T) {
	j, d, _ := devJoiner(t)
	d.SP.Fragment.Run.Gates[0].DecidedByPrincipalID = ""        // no principal id, no hub sub
	d.SP.Fragment.Run.Gates[1].DecidedByHubSub = "hubuser_gone" // a hub sub the principals route does not know
	doc, err := j.Build(context.Background(), devRef(t))
	if err != nil {
		t.Fatal(err)
	}
	if doc.Verdicts[0].Judge != "usr_reviewer" || doc.Verdicts[0].JudgeKind != Unknown {
		t.Errorf("gate_01 = %+v", doc.Verdicts[0])
	}
	if doc.Verdicts[1].Judge != "hubuser_7f3a" || doc.Verdicts[1].JudgeKind != Unknown {
		t.Errorf("gate_02 with an unknown hub sub = %+v", doc.Verdicts[1])
	}
}

func TestPrincipalRouteDownLeavesJudgeKindUnknown(t *testing.T) {
	j, _, _ := devJoiner(t)
	j.Principals = nil
	doc, err := j.Build(context.Background(), devRef(t))
	if err != nil {
		t.Fatal(err)
	}
	if doc.Verdicts[0].Judge != fake.DevHuman || doc.Verdicts[0].JudgeKind != Unknown {
		t.Errorf("gate_01 keeps its producer-mapped judge = %+v", doc.Verdicts[0])
	}
	if doc.Verdicts[1].Judge != "hubuser_7f3a" || doc.Verdicts[1].JudgeKind != Unknown {
		t.Errorf("gate_02 = %+v", doc.Verdicts[1])
	}
}

func TestNotFoundCombinations(t *testing.T) {
	statuses := []source.SourceStatus{source.StatusOK, source.StatusNotFound, source.StatusUnavailable,
		source.StatusTimeout, source.StatusUnauthorized}
	for _, sp := range statuses {
		for _, ap := range statuses {
			j, d, _ := devJoiner(t)
			d.SP.Status, d.AP.Status = sp, ap
			doc, err := j.Build(context.Background(), devRef(t))
			if sp == source.StatusNotFound && ap == source.StatusNotFound {
				if !errors.Is(err, ErrRunNotFound) {
					t.Errorf("sp=%s ap=%s: err = %v, want ErrRunNotFound", sp, ap, err)
				}
				continue
			}
			if err != nil {
				t.Errorf("sp=%s ap=%s: err = %v", sp, ap, err)
				continue
			}
			if doc.Sources["superpipeline"] != sp || doc.Sources["agentpod"] != ap {
				t.Errorf("sp=%s ap=%s: sources = %v", sp, ap, doc.Sources)
			}
			if doc.Run.Ref != "superpipeline:brd_01/run_01" {
				t.Errorf("sp=%s ap=%s: ref = %q", sp, ap, doc.Run.Ref)
			}
		}
	}
}

// A card deleted before superpipeline answers; the hub ledger still knows the run.
func TestDeletedCardRendersFromLedger(t *testing.T) {
	j, d, _ := devJoiner(t)
	d.SP.Status = source.StatusNotFound
	doc, err := j.Build(context.Background(), devRef(t))
	if err != nil {
		t.Fatal(err)
	}
	if doc.Sources["superpipeline"] != source.StatusNotFound || doc.Run.Card.ID != "crd_01" ||
		doc.Run.State != "completed" || len(doc.Attempts) != 1 || doc.Run.Card.Title != Unknown {
		t.Errorf("doc = %+v", doc.Run)
	}
	if doc.Cost.Status != Unknown {
		t.Errorf("cost without superpipeline = %+v", doc.Cost)
	}
}

func TestSlowSourceTimesOutAlone(t *testing.T) {
	j, d, _ := devJoiner(t)
	j.Timeout = 50 * time.Millisecond
	d.Traces.Delay = 2 * time.Second
	start := time.Now()
	doc, err := j.Build(context.Background(), devRef(t))
	if err != nil {
		t.Fatal(err)
	}
	if time.Since(start) > 500*time.Millisecond {
		t.Errorf("Build took %v", time.Since(start))
	}
	if doc.Sources["traces"] != source.StatusTimeout || doc.Trace.Status != TraceUnknown ||
		doc.Attempts[0].SpanCount.Known || doc.Trace.Sampled.Known {
		t.Errorf("trace = %+v, attempt = %+v", doc.Trace, doc.Attempts[0])
	}
	b, _ := json.Marshal(doc)
	if !strings.Contains(string(b), `"span_count":"unknown"`) || !strings.Contains(string(b), `"sampled":"unknown"`) {
		t.Errorf("unknowns must be explicit in JSON: %s", b)
	}
}

func TestPanickingSourceIsUnavailable(t *testing.T) {
	j, d, _ := devJoiner(t)
	d.Errors.Panic = true
	doc, err := j.Build(context.Background(), devRef(t))
	if err != nil || doc.Sources["errors"] != source.StatusUnavailable || doc.Errors == nil {
		t.Errorf("err = %v, sources = %v, errors = %v", err, doc.Sources, doc.Errors)
	}
}

func TestNoSpans(t *testing.T) {
	j, d, _ := devJoiner(t)
	d.Traces.Fragment = source.Fragment{Traces: &source.TracesFragment{TraceIDs: []string{}}}
	doc, _ := j.Build(context.Background(), devRef(t))
	if doc.Trace.Status != TraceNone || len(doc.Verdicts) == 0 || doc.Run.Stage != "draft" {
		t.Errorf("trace = %+v", doc.Trace)
	}
}

func TestSingleDispatchTraceIsJoined(t *testing.T) {
	j, d, _ := devJoiner(t)
	spans := d.Traces.Fragment.Traces.Spans[:3] // drop the Workers span
	d.Traces.Fragment = source.Fragment{Traces: &source.TracesFragment{TraceIDs: source.TraceIDsOf(spans), Spans: spans}}
	doc, _ := j.Build(context.Background(), devRef(t))
	if doc.Trace.Status != TraceJoined || !doc.Trace.Sampled.Known || !doc.Trace.Sampled.V {
		t.Errorf("trace = %+v", doc.Trace)
	}
}

func TestVerdictStoreDownStillShowsGates(t *testing.T) {
	j, _, store := devJoiner(t)
	store.Fail = verdicts.ErrUnavailable
	doc, _ := j.Build(context.Background(), devRef(t))
	if doc.Sources["verdicts"] != source.StatusUnavailable || len(doc.Verdicts) != 4 {
		t.Errorf("sources = %v, verdicts = %d", doc.Sources, len(doc.Verdicts))
	}
}

func TestPhaseTwoGetsTraceIDs(t *testing.T) {
	j, d, _ := devJoiner(t)
	_, _ = j.Build(context.Background(), devRef(t))
	seen := d.Logs.Seen()
	if len(seen) != 1 || len(seen[0].TraceIDs) != 2 {
		t.Errorf("logs saw %+v", seen)
	}
}

// A run still in progress.
func TestRunInProgress(t *testing.T) {
	j, d, _ := devJoiner(t)
	running := "running"
	d.SP.Fragment.Run.Run.Status, d.SP.Fragment.Run.Run.Outcome, d.SP.Fragment.Run.Run.EndedAt = running, nil, nil
	d.AP.Fragment.Ledger.Attempts[0].EndSeq, d.AP.Fragment.Ledger.Attempts[0].EndedAt = nil, nil
	doc, err := j.Build(context.Background(), devRef(t))
	if err != nil {
		t.Fatal(err)
	}
	b, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	// Decode the wire form: a present JSON null is "not ended" / "unreported"; a missing key or
	// "unknown" or 0 would each be a different (wrong) answer, so check presence and value.
	var got struct {
		Run      map[string]json.RawMessage   `json:"run"`
		Attempts []map[string]json.RawMessage `json:"attempts"`
		Cost     map[string]json.RawMessage   `json:"cost"`
	}
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	isNull := func(where string, m map[string]json.RawMessage, key string) {
		t.Helper()
		v, ok := m[key]
		if !ok {
			t.Errorf("%s.%s is missing; want null", where, key)
		} else if string(v) != "null" {
			t.Errorf("%s.%s = %s; want null", where, key, v)
		}
	}
	if string(got.Run["state"]) != `"running"` {
		t.Errorf("run.state = %s", got.Run["state"])
	}
	isNull("run", got.Run, "ended_at")
	if len(got.Attempts) != 1 {
		t.Fatalf("attempts = %d", len(got.Attempts))
	}
	isNull("attempts[0]", got.Attempts[0], "ended_at")
	isNull("attempts[0]", got.Attempts[0], "seq_to")
	if string(got.Cost["status"]) != `"unreported"` {
		t.Errorf("cost.status = %s", got.Cost["status"])
	}
	for _, k := range []string{"input_tokens", "output_tokens", "usd"} {
		isNull("cost", got.Cost, k)
	}
}

func TestCountAndFlagJSON(t *testing.T) {
	for v, want := range map[any]string{Count{}: `"unknown"`, KnownCount(0): `0`, KnownCount(7): `7`,
		Flag{}: `"unknown"`, Flag{V: true, Known: true}: `true`} {
		b, _ := json.Marshal(v)
		if string(b) != want {
			t.Errorf("%#v → %s, want %s", v, b, want)
		}
	}
}

// Fragments are shared with the source and read-only: Build must not change them.
func TestBuildDoesNotMutateFragments(t *testing.T) {
	j, d, _ := devJoiner(t)
	before, _ := json.Marshal([]any{d.SP.Fragment.Run, d.AP.Fragment.Ledger, d.Traces.Fragment.Traces})
	if _, err := j.Build(context.Background(), devRef(t)); err != nil {
		t.Fatal(err)
	}
	after, _ := json.Marshal([]any{d.SP.Fragment.Run, d.AP.Fragment.Ledger, d.Traces.Fragment.Traces})
	if string(before) != string(after) {
		t.Errorf("fragments changed:\n%s\n%s", before, after)
	}
}

func TestCancelledGateAtAndJudge(t *testing.T) {
	j, d, _ := devJoiner(t)
	g := &d.SP.Fragment.Run.Gates[3]
	g.ResolvedAt = nil // At falls back to created_at
	doc, err := j.Build(context.Background(), devRef(t))
	if err != nil {
		t.Fatal(err)
	}
	if v := doc.Verdicts[3]; v.Status != "cancelled" || v.At != "2026-10-04T10:40:00Z" {
		t.Errorf("cancelled gate without resolved_at = %+v", v)
	}
	by := "usr_x"
	g.DecidedBy = &by // a decider on a cancelled gate follows the resolved-gate resolution
	doc, err = j.Build(context.Background(), devRef(t))
	if err != nil {
		t.Fatal(err)
	}
	if v := doc.Verdicts[3]; v.Status != "cancelled" || v.Judge != "usr_x" {
		t.Errorf("cancelled gate with decider = %+v", v)
	}
}

func TestUnknownGateStatusIsNeverPending(t *testing.T) {
	j, d, _ := devJoiner(t)
	d.SP.Fragment.Run.Gates[3].Status = "expired"
	doc, err := j.Build(context.Background(), devRef(t))
	if err != nil {
		t.Fatal(err)
	}
	if v := doc.Verdicts[3]; v.Status != Unknown {
		t.Errorf("unknown-status gate = %+v", v)
	}
}
