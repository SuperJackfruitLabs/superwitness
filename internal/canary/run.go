package canary

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"
)

type Superpipeline interface {
	CreateCard(ctx context.Context, boardID, title string, spec any) (string, error)
	Attempts(ctx context.Context, boardID, cardID string) ([]SPAttempt, error)
}

type RunReader interface {
	GetRun(ctx context.Context, boardID, runID string) (RunDoc, error)
	Logs(ctx context.Context, boardID, runID string) ([]LogLine, error)
}

type Grader interface {
	RecordVerdict(ctx context.Context, in VerdictInput) (Verdict, error)
	GetRun(ctx context.Context, boardID, runID string) (RunDoc, error)
}

type SpanReader interface {
	Trace(ctx context.Context, traceID string) ([]Span, error)
	FindByTag(ctx context.Context, service, key, value string, start, end time.Time) ([]Span, string, error)
	Scan(ctx context.Context, start, end time.Time, needles map[string]string) (ScanResult, error)
}

type LogScanner interface {
	Scan(ctx context.Context, start, end time.Time, needles map[string]string) (ScanResult, error)
}

type ErrorInjector interface {
	InjectError(ctx context.Context, runID, traceID, nonce string, at time.Time) error
}

type Deps struct {
	SP          Superpipeline
	API         RunReader
	MCP         Grader
	Traces      SpanReader
	Logs        LogScanner
	Errors      ErrorInjector
	OTLPLogsURL string
	Principal   string
	Now         func() time.Time
	Sleep       func(context.Context, time.Duration) error
	Rand        io.Reader
	Prior       []Result
}

const followUpLimit = 7

// CardSpec is the prompt. It plants both markers in prose and in a file write, so they
// travel through the prompt, the response and a tool call: every content path that must
// never reach telemetry.
func CardSpec(m Markers) map[string]any {
	line := m.Plain + " " + m.Secret
	return map[string]any{
		"kind": "superwitness-canary",
		"task": "Write a file named superwitness-canary.txt in your workspace containing exactly this line: " +
			line + " — then reply with one sentence that repeats the same line, and complete the run.",
	}
}

func Run(ctx context.Context, cfg Config, d Deps, trigger string) Result {
	r := Result{V: ResultVersion, Trigger: trigger, StartedAt: d.Now().UTC(), BoardID: cfg.BoardID}
	r.GateResolutions, r.Proofs = FollowUpGates(ctx, cfg, d)
	markers, err := NewMarkers(d.Rand)
	if err != nil {
		r.Error = "markers: " + err.Error()
		r.Finish(d.Now().UTC())
		return r
	}
	done := func() Result {
		r.Finish(d.Now().UTC())
		r.Redact(markers.Needles())
		r.DeriveProofs()
		return r
	}
	nonce, err := newNonce(d.Rand)
	if err != nil {
		r.Error = "nonce: " + err.Error()
		return done()
	}

	windowStart := r.StartedAt.Add(-time.Minute)
	title := "superwitness canary " + r.StartedAt.Format("2006-01-02 15:04Z")
	cardID, err := d.SP.CreateCard(ctx, cfg.BoardID, title, CardSpec(markers))
	if err != nil {
		r.Error = "create card: " + err.Error()
		return done()
	}
	r.CardID = cardID

	att, err := waitForRun(ctx, cfg, d, cardID, r.StartedAt)
	if err != nil {
		r.Error = err.Error()
		return done()
	}
	r.RunID = att.RunID
	if att.Outcome != nil {
		r.RunOutcome = *att.Outcome
	}

	if err := d.Sleep(ctx, cfg.SettleDelay); err != nil {
		r.Error = "interrupted: " + err.Error()
		return done()
	}
	doc, err := waitForDoc(ctx, cfg, d, r.RunID, func(doc RunDoc) bool { return docSettled(doc, cfg.GateStage, r.RunID) })
	if err != nil {
		r.Error = "superwitness run API: " + err.Error()
		return done()
	}
	docQuery := "GET /v1/runs/superpipeline/" + cfg.BoardID + "/" + r.RunID

	// Flow 8's injected line goes out first, so it is indexing while the other checks run.
	traceID := ""
	if len(doc.Trace.TraceIDs) > 0 {
		traceID = doc.Trace.TraceIDs[0]
	}
	injectErr := d.Errors.InjectError(ctx, r.RunID, traceID, nonce, d.Now().UTC())

	spans, spanErr := fetchSpans(ctx, d, doc.Trace.TraceIDs)
	traceQuery := docQuery + "; GET /select/jaeger/api/traces/{" + strings.Join(doc.Trace.TraceIDs, ",") + "}"
	workers, workersQuery, workersErr := d.Traces.FindByTag(ctx, workersService, "run.id", r.RunID, windowStart, d.Now().UTC())

	r.Checks = append(r.Checks,
		withQuery(CheckRunEvidence(doc, cfg.BoardID, r.RunID, cardID, cfg.WorkStage), docQuery),
		withQuery(CheckAttempts(doc, cfg.ExpectHarness), docQuery))
	if spanErr != nil {
		r.Checks = append(r.Checks,
			Check{ID: "3", Name: FlowNames[3], Detail: "VictoriaTraces: " + spanErr.Error(), Query: traceQuery},
			Check{ID: "4", Name: FlowNames[4], Detail: "VictoriaTraces: " + spanErr.Error(), Query: traceQuery})
	} else {
		r.Checks = append(r.Checks,
			withQuery(CheckHubSpansJoined(doc, r.RunID, spans), traceQuery),
			withQuery(CheckNodeAgentSpan(spans), traceQuery))
	}
	r.Checks = append(r.Checks, withQuery(CheckWorkersSpan(r.RunID, spans, workers, workersErr), workersQuery))
	gate, pending := CheckGateShown(doc, cfg.GateStage, r.RunID)
	r.GatePending = pending
	r.Checks = append(r.Checks, withQuery(gate, docQuery), gradeRoundTrip(ctx, cfg, d, r.RunID))

	logsQuery := fmt.Sprintf("POST %s (one ERROR record, service.name %s, %s); GET /v1/runs/superpipeline/%s/%s/logs; %s (errors)",
		d.OTLPLogsURL, canaryService, nonce, cfg.BoardID, r.RunID, docQuery)
	if injectErr != nil {
		r.Checks = append(r.Checks, Check{ID: "8", Name: FlowNames[8], Detail: "injecting the error line: " + injectErr.Error(), Query: logsQuery})
	} else {
		withErr, _ := waitForDoc(ctx, cfg, d, r.RunID, func(doc RunDoc) bool { return hasInjected(doc, nonce) })
		if withErr.Sources == nil {
			withErr = doc
		}
		lines, logsErr := d.API.Logs(ctx, cfg.BoardID, r.RunID)
		r.Checks = append(r.Checks, withQuery(CheckLinkedLogsAndErrors(withErr, lines, logsErr, r.RunID, nonce), logsQuery))
	}

	end := d.Now().UTC()
	needles := merge(markers.Needles(), Controls(r.RunID, doc.Trace.TraceIDs))
	logs, logsErr := d.Logs.Scan(ctx, windowStart, end, needles)
	traces, tracesErr := d.Traces.Scan(ctx, windowStart, end, needles)
	scanQuery := fmt.Sprintf("POST /select/logsql/query query=_time:[%s, %s]; GET /select/jaeger/api/traces?service=<each>&start=%d&end=%d",
		windowStart.Format(time.RFC3339), end.Format(time.RFC3339), windowStart.UnixMicro(), end.UnixMicro())
	r.Checks = append(r.Checks, withQuery(CheckMarkerAbsent(logs, logsErr, traces, tracesErr), scanQuery))
	return done()
}

// waitForRun returns the first COMPLETED run on the work stage. A reclaimed or failed run
// is passed over, because the card may be claimed again.
func waitForRun(ctx context.Context, cfg Config, d Deps, cardID string, start time.Time) (SPAttempt, error) {
	claimBy := start.Add(cfg.ClaimTimeout)
	endBy := start.Add(cfg.RunTimeout)
	var lastErr error
	var lastEnded *SPAttempt
	claimed := false
	for {
		atts, err := d.SP.Attempts(ctx, cfg.BoardID, cardID)
		if err != nil {
			lastErr = err
		}
		for i := range atts {
			a := atts[i]
			if a.StageKey != cfg.WorkStage {
				continue
			}
			claimed = true
			if a.Status != "ended" {
				continue
			}
			if a.Outcome != nil && *a.Outcome == "completed" {
				return a, nil
			}
			lastEnded = &a
		}
		now := d.Now()
		if !claimed && now.After(claimBy) {
			return SPAttempt{}, fmt.Errorf("card %s not claimed on stage %q within %s%s", cardID, cfg.WorkStage, cfg.ClaimTimeout, suffix(lastErr))
		}
		if now.After(endBy) {
			msg := fmt.Sprintf("run on card %s did not end completed within %s", cardID, cfg.RunTimeout)
			if lastEnded != nil && lastEnded.Outcome != nil {
				msg += fmt.Sprintf("; last run outcome %s (%s)", *lastEnded.Outcome, lastEnded.RunID)
			}
			return SPAttempt{}, fmt.Errorf("%s%s", msg, suffix(lastErr))
		}
		if err := d.Sleep(ctx, cfg.PollInterval); err != nil {
			return SPAttempt{}, err
		}
	}
}

func suffix(err error) string {
	if err == nil {
		return ""
	}
	return " (last poll error: " + err.Error() + ")"
}

func docSettled(doc RunDoc, gateStage, runID string) bool {
	_, gate := runGate(doc, gateStage, runID)
	return (doc.Trace.Status == "joined" || doc.Trace.Status == "partial") && len(doc.Attempts) > 0 && gate
}

func hasInjected(doc RunDoc, nonce string) bool {
	for _, e := range doc.Errors {
		if e.Service == canaryService && strings.Contains(e.Message, nonce) {
			return true
		}
	}
	return false
}

// waitForDoc polls until ready(doc), then returns. At the deadline it returns the last
// document it got, so the checks can name what is missing.
func waitForDoc(ctx context.Context, cfg Config, d Deps, runID string, ready func(RunDoc) bool) (RunDoc, error) {
	deadline := d.Now().Add(cfg.DocTimeout)
	var last RunDoc
	have := false
	for {
		got, err := d.API.GetRun(ctx, cfg.BoardID, runID)
		if err == nil {
			last, have = got, true
			if ready(got) {
				return got, nil
			}
		}
		if d.Now().After(deadline) {
			if have {
				return last, nil
			}
			return RunDoc{}, err
		}
		if serr := d.Sleep(ctx, cfg.PollInterval); serr != nil {
			if have {
				return last, nil
			}
			return RunDoc{}, serr
		}
	}
}

func fetchSpans(ctx context.Context, d Deps, traceIDs []string) ([]Span, error) {
	var all []Span
	for _, id := range traceIDs {
		spans, err := d.Traces.Trace(ctx, id)
		if err != nil {
			return nil, err
		}
		all = append(all, spans...)
	}
	return all, nil
}

func gradeRoundTrip(ctx context.Context, cfg Config, d Deps, runID string) Check {
	in := VerdictInput{
		IdempotencyKey: "superwitness-canary:" + runID,
		SubjectKind:    "run",
		SubjectRef:     "superpipeline:" + cfg.BoardID + "/" + runID,
		Standard:       cfg.Standard,
		Value:          map[string]any{"score": 1.0},
		Comment:        "superwitness canary: a round-trip check, not a judgement of the work",
	}
	query := fmt.Sprintf("MCP tools/call record_verdict {idempotency_key: %s, subject_ref: %s, standard: %s} twice; MCP tools/call get_run {source: superpipeline, board_id: %s, run_id: %s}",
		in.IdempotencyKey, in.SubjectRef, in.Standard, cfg.BoardID, runID)
	fail := func(detail string) Check {
		return Check{ID: "7", Name: FlowNames[7], Detail: detail, Query: query}
	}
	first, err := d.MCP.RecordVerdict(ctx, in)
	if err != nil {
		return fail("record_verdict: " + err.Error())
	}
	again, err := d.MCP.RecordVerdict(ctx, in)
	if err != nil {
		return fail("record_verdict retry: " + err.Error())
	}
	doc, err := d.MCP.GetRun(ctx, cfg.BoardID, runID)
	if err != nil {
		return fail("get_run over MCP: " + err.Error())
	}
	return withQuery(CheckGraderRoundTrip(first, again, doc, d.Principal, cfg.Standard), query)
}

func unresolvedID(s string) bool { return s == "" || s == "unknown" }

// FollowUpGates proves flow 6 across runs. An earlier run saw its gate PENDING; when a
// later fetch shows it decided by a human who is not the run's agent, that is the proof.
// It never fails tonight's run: an unreadable document is simply tried again next time.
func FollowUpGates(ctx context.Context, cfg Config, d Deps) ([]GateResolution, []FlowProof) {
	resolved := map[string]bool{}
	for _, p := range d.Prior {
		for _, g := range p.GateResolutions {
			resolved[g.RunID] = true
		}
	}
	var out []GateResolution
	var proofs []FlowProof
	looked := 0
	for i := len(d.Prior) - 1; i >= 0 && looked < followUpLimit; i-- {
		p := d.Prior[i]
		if p.RunID == "" || !p.GatePending || resolved[p.RunID] {
			continue
		}
		looked++
		doc, err := d.API.GetRun(ctx, p.BoardID, p.RunID)
		if err != nil {
			continue
		}
		g, ok := runGate(doc, cfg.GateStage, p.RunID)
		dec := ""
		if ok {
			dec = gateDecision(g)
		}
		// Only "pending" awaits a human. "unknown" is not a decision either (the service
		// gives it the same null decision), so both are tried again next time and record
		// nothing; recording a non-passing line would mark the run resolved and stop the
		// follow-up before a human decides.
		if !ok || dec == "pending" || dec == "unknown" {
			continue
		}
		// A missing agent mapping, judge or judge_kind may only mean a lookup was
		// unavailable: distinctness is unproven, so wait for a later fetch.
		if dec != "cancelled" && (unresolvedID(doc.Run.Agent) || unresolvedID(g.Judge) || unresolvedID(g.JudgeKind)) {
			continue
		}
		gr := GateResolution{RunID: p.RunID, GateID: g.ID, Decision: dec, JudgeKind: g.JudgeKind, ObservedAt: d.Now().UTC()}
		switch {
		case dec == "cancelled":
			gr.Detail = "gate was cancelled, so no human decision will follow"
		case g.JudgeKind != "human":
			gr.Detail = "gate decided by judge_kind " + q(g.JudgeKind)
		case g.Judge == doc.Run.Agent:
			gr.Detail = "gate decided by the executing agent"
		default:
			gr.Pass = true
			gr.Detail = "decided by a human principal"
		}
		resolved[p.RunID] = true
		out = append(out, gr)
		if gr.Pass {
			proofs = append(proofs, FlowProof{
				Flow:   6,
				RunRef: "superpipeline:" + p.BoardID + "/" + p.RunID,
				Output: fmt.Sprintf("gate %s seen pending by the run at %s; now %s by %s (judge_kind human)",
					q(g.ID), p.EndedAt.UTC().Format(time.RFC3339), gr.Decision, g.Judge),
				Query: "GET /v1/runs/superpipeline/" + p.BoardID + "/" + p.RunID,
				At:    gr.ObservedAt,
			})
		}
	}
	return out, proofs
}
