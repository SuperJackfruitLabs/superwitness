package canary

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

const (
	ResultVersion  = 1
	TriggerNightly = "nightly"
	TriggerManual  = "manual"
	// TriggerAlert lines carry only flow 10's proof, written by `canary record-alert`.
	TriggerAlert = "alert"
	FlowCount    = 10
)

// FlowNames are the canary's ten acceptance flows.
var FlowNames = map[int]string{
	1:  "superpipeline run evidence",
	2:  "attempts and fingerprints",
	3:  "hub spans joined to the run",
	4:  "node-agent span in the run's trace",
	5:  "superpipeline Workers span via the OTLP edge",
	6:  "gate verdict, pending then human-decided",
	7:  "grader verdict through MCP",
	8:  "logs and errors linked to the run",
	9:  "no content in telemetry",
	10: "forced failure reaches ntfy",
}

// RunFlows are the checks one run makes, in order. Flow 10 is never a run check.
var RunFlows = []string{"1", "2", "3", "4", "5", "6", "7", "8", "9"}

type Check struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Pass   bool   `json:"pass"`
	Detail string `json:"detail"`
	Query  string `json:"query,omitempty"`
}

func withQuery(c Check, q string) Check {
	c.Query = q
	return c
}

// FlowProof is one flow's evidence: the run reference, the check output, the query, and when.
type FlowProof struct {
	Flow   int       `json:"flow"`
	RunRef string    `json:"run_ref"`
	Output string    `json:"output"`
	Query  string    `json:"query"`
	At     time.Time `json:"at"`
}

// GateResolution records what a later run saw on an earlier run's gate.
type GateResolution struct {
	RunID      string    `json:"run_id"`
	GateID     string    `json:"gate_id"`
	Decision   string    `json:"decision"`
	JudgeKind  string    `json:"judge_kind"`
	Pass       bool      `json:"pass"`
	Detail     string    `json:"detail"`
	ObservedAt time.Time `json:"observed_at"`
}

type Result struct {
	V               int              `json:"v"`
	Trigger         string           `json:"trigger"`
	Forced          bool             `json:"forced,omitempty"`
	StartedAt       time.Time        `json:"started_at"`
	EndedAt         time.Time        `json:"ended_at"`
	BoardID         string           `json:"board_id"`
	CardID          string           `json:"card_id,omitempty"`
	RunID           string           `json:"run_id,omitempty"`
	RunOutcome      string           `json:"run_outcome,omitempty"`
	GatePending     bool             `json:"gate_pending,omitempty"`
	Pass            bool             `json:"pass"`
	Checks          []Check          `json:"checks"`
	Error           string           `json:"error,omitempty"`
	GateResolutions []GateResolution `json:"gate_resolutions,omitempty"`
	Proofs          []FlowProof      `json:"proofs,omitempty"`
}

// Finish sets the regression verdict: all nine run checks, in order, passing.
func (r *Result) Finish(now time.Time) {
	r.EndedAt = now
	if r.Checks == nil {
		r.Checks = []Check{} // the wire form is always an array, never null
	}
	r.Pass = r.Error == "" && len(r.Checks) == len(RunFlows)
	for i, c := range r.Checks {
		if !c.Pass || i >= len(RunFlows) || c.ID != RunFlows[i] {
			r.Pass = false
		}
	}
}

// DeriveProofs turns each passing check into its flow's proof. Flow 6 is excluded: its
// proof needs a human decision that a LATER run observes (FollowUpGates).
func (r *Result) DeriveProofs() {
	if r.RunID == "" {
		return
	}
	ref := "superpipeline:" + r.BoardID + "/" + r.RunID
	for _, c := range r.Checks {
		n, err := strconv.Atoi(c.ID)
		if err != nil || !c.Pass || n == 6 {
			continue
		}
		r.Proofs = append(r.Proofs, FlowProof{Flow: n, RunRef: ref, Output: c.Detail, Query: c.Query, At: r.EndedAt})
	}
}

// Redact replaces every needle value with its label. It runs before a Result is printed,
// stored or turned into proofs, so the canary cannot become the leak flow 9 looks for.
func (r *Result) Redact(needles map[string]string) {
	clean := func(s string) string {
		for label, n := range needles {
			if n != "" {
				s = strings.ReplaceAll(s, n, "<"+label+">")
			}
		}
		return s
	}
	r.Error = clean(r.Error)
	for i := range r.Checks {
		r.Checks[i].Detail = clean(r.Checks[i].Detail)
		r.Checks[i].Query = clean(r.Checks[i].Query)
	}
	for i := range r.GateResolutions {
		r.GateResolutions[i].Detail = clean(r.GateResolutions[i].Detail)
	}
	for i := range r.Proofs {
		r.Proofs[i].Output = clean(r.Proofs[i].Output)
		r.Proofs[i].Query = clean(r.Proofs[i].Query)
	}
}

// OneLine is what reaches stdout, and so the journal: ids and states only, never details.
func OneLine(r Result) string {
	var b strings.Builder
	if r.Pass {
		b.WriteString("canary PASS")
	} else {
		b.WriteString("canary FAIL")
	}
	if r.Forced {
		b.WriteString(" forced")
	}
	if r.RunID != "" {
		fmt.Fprintf(&b, " run=%s", r.RunID)
	}
	for _, c := range r.Checks {
		st := "ok"
		if !c.Pass {
			st = "FAIL"
		}
		fmt.Fprintf(&b, " %s=%s", c.ID, st)
	}
	if n := len(r.Proofs); n > 0 {
		fmt.Fprintf(&b, " proofs=%d", n)
	}
	if r.Error != "" {
		fmt.Fprintf(&b, " error=%q", r.Error)
	}
	return b.String()
}
