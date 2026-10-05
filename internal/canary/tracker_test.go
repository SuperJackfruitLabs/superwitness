package canary

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func day(d int) time.Time { return time.Date(2026, 10, d, 2, 30, 0, 0, time.UTC) }

// passingRun is a run whose nine checks passed, with its proofs derived.
func passingRun(at time.Time, runID, trigger string) Result {
	r := Result{V: ResultVersion, Trigger: trigger, StartedAt: at, BoardID: "brd_c", RunID: runID,
		GatePending: true, Checks: allPassing()}
	r.Finish(at.Add(10 * time.Minute))
	r.DeriveProofs()
	return r
}

func failingIn(r Result, flow string) Result {
	for i := range r.Checks {
		if r.Checks[i].ID == flow {
			r.Checks[i].Pass = false
			r.Checks[i].Detail = "broke " + flow
		}
	}
	r.Proofs = nil
	r.Finish(r.EndedAt)
	r.DeriveProofs()
	return r
}

// gateProofRun is a later run that also observed run `earlier`'s gate decided by a human.
func gateProofRun(at time.Time, runID, earlier string) Result {
	r := passingRun(at, runID, TriggerNightly)
	r.GateResolutions = []GateResolution{{RunID: earlier, GateID: "gate_1", Decision: "approved", JudgeKind: "human", Pass: true}}
	r.Proofs = append([]FlowProof{{Flow: 6, RunRef: "superpipeline:brd_c/" + earlier,
		Output: "gate gate_1 seen pending; now approved by prn_op (judge_kind human)",
		Query:  "GET /v1/runs/superpipeline/brd_c/" + earlier, At: at}}, r.Proofs...)
	return r
}

func alertProofLine(at time.Time) Result {
	return Result{V: ResultVersion, Trigger: TriggerAlert, StartedAt: at, EndedAt: at, BoardID: "brd_c", Pass: true,
		Proofs: []FlowProof{{Flow: 10, RunRef: "forced:" + at.Format(time.RFC3339), Output: "ntfy accepted and stored message m1",
			Query: "POST /superwitness-canary; GET /superwitness-canary/json?poll=1&since=15m", At: at}}}
}

func flow(s Summary, n int) FlowStatus { return s.Flows[n-1] }

func TestAppendAndLoadRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "results.jsonl")
	if rs, err := LoadResults(path); err != nil || rs != nil {
		t.Fatalf("missing file: %v %v", rs, err)
	}
	for _, r := range []Result{passingRun(day(1), "run_1", TriggerNightly), alertProofLine(day(2))} {
		if err := AppendResult(path, r); err != nil {
			t.Fatal(err)
		}
	}
	rs, err := LoadResults(path)
	if err != nil || len(rs) != 2 || len(rs[0].Proofs) != 8 || rs[1].Proofs[0].Flow != 10 {
		t.Fatalf("rs=%+v err=%v", rs, err)
	}
}

func TestLoadResultsRejectsACorruptLine(t *testing.T) {
	path := filepath.Join(t.TempDir(), "results.jsonl")
	_ = AppendResult(path, passingRun(day(1), "run_1", TriggerNightly))
	f, _ := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o640)
	_, _ = f.WriteString("{not json\n")
	_ = f.Close()
	if _, err := LoadResults(path); err == nil || !strings.Contains(err.Error(), "line 2") {
		t.Fatalf("err = %v", err)
	}
}

func TestSummarizePartialProof(t *testing.T) {
	s := Summarize([]Result{passingRun(day(1), "run_1", TriggerNightly)})
	if s.Proven != 8 || s.Accepted || flow(s, 6).Proven || flow(s, 10).Proven || !flow(s, 5).Proven {
		t.Fatalf("s = %+v", s)
	}
	if p := flow(s, 9).Proof; p == nil || p.RunRef != "superpipeline:brd_c/run_1" || p.Query == "" || p.Output == "" {
		t.Fatalf("flow 9 proof = %+v", p)
	}
}

func TestSummarizeAllTenProvenAcrossRuns(t *testing.T) {
	s := Summarize([]Result{
		passingRun(day(1), "run_1", TriggerNightly),
		gateProofRun(day(2), "run_2", "run_1"),
		alertProofLine(day(3)),
	})
	if !s.Accepted || s.Proven != 10 || s.AcceptedAt != day(3).Format(time.RFC3339) {
		t.Fatalf("s = %+v", s)
	}
	if got := flow(s, 6).Proof.RunRef; got != "superpipeline:brd_c/run_1" {
		t.Errorf("flow 6 proved against %s", got)
	}
	if got := flow(s, 1).Proof.RunRef; got != "superpipeline:brd_c/run_1" {
		t.Errorf("flow 1 keeps its FIRST proof, got %s", got)
	}
}

func TestSummarizeManualRunsCount(t *testing.T) {
	s := Summarize([]Result{passingRun(day(1), "run_1", TriggerManual)})
	if s.Proven != 8 || s.RunsRecorded != 1 {
		t.Fatalf("s = %+v", s)
	}
}

func TestSummarizeALaterFailureIsARegressionNotAnUnproof(t *testing.T) {
	s := Summarize([]Result{
		passingRun(day(1), "run_1", TriggerNightly),
		gateProofRun(day(2), "run_2", "run_1"),
		alertProofLine(day(2)),
		failingIn(passingRun(day(4), "run_4", TriggerNightly), "5"),
	})
	f5 := flow(s, 5)
	if !s.Accepted || !f5.Proven || !f5.Regressed || !strings.Contains(f5.LastFailure, "run_4") || !strings.Contains(f5.LastFailure, "broke 5") {
		t.Fatalf("flow 5 = %+v accepted=%v", f5, s.Accepted)
	}
	if s.LastRunPass {
		t.Error("the last run failed")
	}
}

func TestSummarizeARecoveredRegressionClears(t *testing.T) {
	s := Summarize([]Result{
		passingRun(day(1), "run_1", TriggerNightly),
		failingIn(passingRun(day(4), "run_4", TriggerNightly), "5"),
		passingRun(day(5), "run_5", TriggerNightly),
	})
	if f5 := flow(s, 5); f5.Regressed || f5.LastFailure != "" {
		t.Fatalf("flow 5 = %+v", f5)
	}
}

func TestSummarizeAFailureBeforeAnyProofIsNotARegression(t *testing.T) {
	s := Summarize([]Result{failingIn(passingRun(day(1), "run_1", TriggerNightly), "4")})
	if f4 := flow(s, 4); f4.Proven || f4.Regressed || !strings.Contains(f4.LastFailure, "broke 4") {
		t.Fatalf("flow 4 = %+v", f4)
	}
}

func TestSummarizeSkipsAlertLinesAndForcedRunsAsRuns(t *testing.T) {
	forced := Result{V: ResultVersion, Trigger: TriggerManual, Forced: true, StartedAt: day(2), Error: "forced failure"}
	forced.Finish(day(2))
	s := Summarize([]Result{passingRun(day(1), "run_1", TriggerNightly), forced, alertProofLine(day(2))})
	if s.RunsRecorded != 1 || !s.LastRunPass || !flow(s, 10).Proven {
		t.Fatalf("s = %+v", s)
	}
}

func TestFormatSummary(t *testing.T) {
	partial := FormatSummary(Summarize([]Result{passingRun(day(1), "run_1", TriggerNightly)}))
	for _, want := range []string{"NOT YET ACCEPTED: 8/10 flows proven", "  6  not yet", "supi gates", "force-fail", "query:"} {
		if !strings.Contains(partial, want) {
			t.Errorf("missing %q in:\n%s", want, partial)
		}
	}
	done := FormatSummary(Summarize([]Result{
		passingRun(day(1), "run_1", TriggerNightly), gateProofRun(day(2), "run_2", "run_1"), alertProofLine(day(3)),
		failingIn(passingRun(day(4), "run_4", TriggerNightly), "5"),
	}))
	for _, want := range []string{"ACCEPTED at " + day(3).Format(time.RFC3339), "PROVEN, REGRESSED"} {
		if !strings.Contains(done, want) {
			t.Errorf("missing %q in:\n%s", want, done)
		}
	}
}

func TestSummarizeACancelledGateAfterProofIsAFlow6Regression(t *testing.T) {
	r := passingRun(day(5), "run_5", TriggerNightly)
	r.GateResolutions = []GateResolution{{RunID: "run_2", GateID: "gate_2", Decision: "cancelled", JudgeKind: "human", Pass: false, ObservedAt: day(5)}}
	s := Summarize([]Result{passingRun(day(1), "run_1", TriggerNightly), gateProofRun(day(2), "run_2", "run_1"), alertProofLine(day(3)), r})
	if f6 := flow(s, 6); !f6.Proven || !f6.Regressed || !strings.Contains(f6.LastFailure, "cancelled") {
		t.Fatalf("flow 6 = %+v", f6)
	}
}

func TestSummarizeCancelledGateBeforeProofIsNotARegression(t *testing.T) {
	r := passingRun(day(1), "run_1", TriggerNightly)
	r.GateResolutions = []GateResolution{{RunID: "run_0", GateID: "g0", Decision: "cancelled", Pass: false, ObservedAt: day(1)}}
	s := Summarize([]Result{r, gateProofRun(day(2), "run_2", "run_1")})
	if f6 := flow(s, 6); !f6.Proven || f6.Regressed {
		t.Fatalf("flow 6 = %+v", f6)
	}
}

func cancelledGateRun(at time.Time, runID string) Result {
	r := passingRun(at, runID, TriggerNightly)
	r.GateResolutions = []GateResolution{{RunID: "run_2", GateID: "gate_2", Decision: "cancelled", JudgeKind: "human", Pass: false, ObservedAt: at}}
	return r
}

func TestSummarizeAPlainPassingRunDoesNotWipeACancelledGateRegression(t *testing.T) {
	s := Summarize([]Result{passingRun(day(1), "run_1", TriggerNightly), gateProofRun(day(2), "run_2", "run_1"),
		cancelledGateRun(day(5), "run_5"), passingRun(day(6), "run_6", TriggerNightly)})
	if f6 := flow(s, 6); !f6.Proven || !f6.Regressed || !strings.Contains(f6.LastFailure, "cancelled") {
		t.Fatalf("flow 6 = %+v", f6)
	}
}

func TestSummarizeAPassingGateResolutionClearsAGateRegression(t *testing.T) {
	later := passingRun(day(7), "run_7", TriggerNightly)
	later.GateResolutions = []GateResolution{{RunID: "run_6", GateID: "gate_6", Decision: "approved", JudgeKind: "human", Pass: true, ObservedAt: day(7)}}
	s := Summarize([]Result{passingRun(day(1), "run_1", TriggerNightly), gateProofRun(day(2), "run_2", "run_1"),
		cancelledGateRun(day(5), "run_5"), later})
	if f6 := flow(s, 6); f6.Regressed || f6.LastFailure != "" {
		t.Fatalf("flow 6 = %+v", f6)
	}
}
