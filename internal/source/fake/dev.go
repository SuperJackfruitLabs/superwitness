package fake

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/SuperJackfruitLabs/superwitness/internal/auth"
	"github.com/SuperJackfruitLabs/superwitness/internal/contracts"
	"github.com/SuperJackfruitLabs/superwitness/internal/source"
)

const (
	DevBoard    = "brd_01"
	DevRun      = "run_01"
	DevAttempt  = "attempt_01"
	DevHuman    = "prn_human01"
	DevExecutor = "prn_agent01"
	DevGrader   = "prn_grader01"
	DevHuman2   = "prn_human02" // gate_02's decider, known only by hub sub in the superpipeline fixture
)

// Dev serves run superpipeline:brd_01/run_01 from the contract fixtures, whose principal
// fields name DevExecutor as the agent and DevHuman as gate_01's decider.
type Dev struct {
	SP, AP, Traces, Logs, Errors *Source

	spans      []source.Span
	lines      []source.LogLine
	principals map[string]auth.PrincipalRecord // keyed by prn_… id or hub auth user id
}

func NewDev() (*Dev, error) {
	var run source.RunFragment
	if err := json.Unmarshal(contracts.SuperpipelineEvidence, &run); err != nil {
		return nil, fmt.Errorf("superpipeline fixture: %w", err)
	}
	var led source.LedgerFragment
	if err := json.Unmarshal(contracts.HubEvidenceRun, &led); err != nil {
		return nil, fmt.Errorf("hub fixture: %w", err)
	}

	d := &Dev{
		spans: devSpans(),
		lines: devLines(),
		principals: map[string]auth.PrincipalRecord{
			DevHuman:       {ID: DevHuman, Kind: auth.KindHuman},
			DevExecutor:    {ID: DevExecutor, Kind: auth.KindAgent},
			DevGrader:      {ID: DevGrader, Kind: auth.KindAgent},
			DevHuman2:      {ID: DevHuman2, Kind: auth.KindHuman},
			"hubuser_7f3a": {ID: DevHuman2, Kind: auth.KindHuman}, // gate_02's decided_by_hub_sub
			"hubuser_01":   {ID: DevHuman, Kind: auth.KindHuman},  // DevHuman's hub account, as a browser sign-in names it
		},
	}

	var errs []source.ErrorLine
	for _, l := range d.lines {
		if l.Level == "error" {
			errs = append(errs, source.ErrorLine{Service: l.Service, Message: l.Message, TraceID: l.TraceID, At: l.At})
		}
	}
	d.SP = &Source{SourceName: source.Superpipeline, Match: d.isDev, Fragment: source.Fragment{Version: run.AsOf, Run: &run}}
	d.AP = &Source{SourceName: source.AgentPod, Match: d.isDev, Fragment: source.Fragment{Version: led.AsOf, Ledger: &led}}
	d.Traces = &Source{SourceName: source.Traces, Match: d.isDev,
		Fragment: source.Fragment{Traces: &source.TracesFragment{TraceIDs: source.TraceIDsOf(d.spans), Spans: d.spans}},
		Miss:     &source.Fragment{Traces: &source.TracesFragment{TraceIDs: []string{}}}}
	d.Logs = &Source{SourceName: source.Logs, Match: d.isDev,
		Fragment: source.Fragment{Logs: &source.LogsFragment{Count: len(d.lines)}},
		Miss:     &source.Fragment{Logs: &source.LogsFragment{}}}
	d.Errors = &Source{SourceName: source.Errors, Match: d.isDev,
		Fragment: source.Fragment{Errors: &source.ErrorsFragment{Errors: errs}},
		Miss:     &source.Fragment{Errors: &source.ErrorsFragment{}}}
	return d, nil
}

func (d *Dev) isDev(r source.RunRef) bool { return r.BoardID == DevBoard && r.RunID == DevRun }

func (d *Dev) ListSpans(_ context.Context, ref source.RunRef) ([]source.Span, source.SourceStatus) {
	if !d.isDev(ref) {
		return []source.Span{}, source.StatusOK
	}
	return append([]source.Span(nil), d.spans...), source.StatusOK
}

func (d *Dev) ListLogs(_ context.Context, ref source.RunRef, q source.LogQuery) ([]source.LogLine, source.SourceStatus) {
	out := []source.LogLine{}
	if !d.isDev(ref) {
		return out, source.StatusOK
	}
	for _, l := range d.lines {
		if q.Level == "" || l.Level == q.Level {
			out = append(out, l)
		}
	}
	if q.Offset >= len(out) {
		return []source.LogLine{}, source.StatusOK
	}
	out = out[q.Offset:]
	if q.Limit > 0 && len(out) > q.Limit {
		out = out[:q.Limit]
	}
	return out, source.StatusOK
}

func (d *Dev) ResolveAttempt(_ context.Context, attemptID string) (source.AttemptLink, source.SourceStatus) {
	if attemptID != DevAttempt {
		return source.AttemptLink{}, source.StatusNotFound
	}
	src, run, board := string(source.Superpipeline), DevRun, DevBoard
	return source.AttemptLink{ExternalSource: &src, ExternalRunID: &run, BoardID: &board}, source.StatusOK
}

func (d *Dev) Lookup(_ context.Context, idOrHubSub string) (auth.PrincipalRecord, error) {
	p, ok := d.principals[idOrHubSub]
	if !ok {
		return auth.PrincipalRecord{}, auth.ErrPrincipalNotFound
	}
	return p, nil
}

func (d *Dev) Ping(context.Context) source.SourceStatus { return source.StatusOK }

func devSpans() []source.Span {
	t0 := time.Date(2026, 10, 4, 10, 0, 0, 0, time.UTC)
	hub, workers := contracts.TraceHub, contracts.TraceWorkers
	return []source.Span{
		{TraceID: hub, SpanID: "00f067aa0ba902b7", Name: "dispatch", Service: "agentpod-hub", Start: t0, DurationMS: 300000,
			Attributes: map[string]string{"run.id": DevRun, "board.id": DevBoard, "card.id": "crd_01", "external.source": "superpipeline"}},
		{TraceID: hub, SpanID: "00f067aa0ba902b8", ParentSpanID: "00f067aa0ba902b7", Name: "attempt", Service: "agentpod-hub",
			Start: t0.Add(2 * time.Second), DurationMS: 296000,
			Attributes: map[string]string{"attempt.id": DevAttempt, "station.id": "stn_01", "run.id": DevRun,
				"harness.name": "hermes", "acp.session_id": "acps_01", "acp.seq_from": "1"}},
		{TraceID: hub, SpanID: "00f067aa0ba902b9", ParentSpanID: "00f067aa0ba902b8", Name: "turn", Service: "agentpod-hub",
			Start: t0.Add(3 * time.Second), DurationMS: 290000,
			Attributes: map[string]string{"attempt.id": DevAttempt, "acp.seq_from": "1", "acp.seq_to": "9"}},
		{TraceID: workers, SpanID: "10f067aa0ba902b7", Name: "POST /v1/boards/:id/runs/:runId/heartbeat",
			Service: "superpipeline-api", Start: t0.Add(time.Minute), DurationMS: 12,
			Attributes: map[string]string{"run.id": DevRun}},
	}
}

func devLines() []source.LogLine {
	t0 := time.Date(2026, 10, 4, 10, 0, 0, 0, time.UTC)
	return []source.LogLine{
		{At: t0.Add(2 * time.Second), Service: "agentpod-hub", Level: "info", Message: "attempt opened",
			TraceID: contracts.TraceHub, SpanID: "00f067aa0ba902b8", RunID: DevRun},
		{At: t0.Add(2 * time.Minute), Service: "agentpod-hub", Level: "error", Message: "bridge heartbeat failed: 502 from superpipeline",
			TraceID: contracts.TraceHub, SpanID: "00f067aa0ba902b7"},
	}
}
