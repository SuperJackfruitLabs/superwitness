package canary

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"
)

// service.name values each product emits, plus the canary's own for its injected error line.
const (
	hubService       = "agentpod-hub"
	nodeAgentService = "agentpod-node-agent"
	workersService   = "superpipeline-api"
	canaryService    = "superwitness-canary"
)

// Span is one span as the canary needs it, whatever API it came from.
type Span struct {
	TraceID   string
	Service   string
	Operation string
	Tags      map[string]string
}

// Verdict is what record_verdict returns: the stored verdict, as the server recorded it.
type Verdict struct {
	ID        string `json:"id"`
	Judge     string `json:"judge"`
	JudgeKind string `json:"judge_kind"`
}

var digestRE = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)

func q(s string) string {
	if s == "" {
		return "missing"
	}
	return s
}

func check(flow int) Check {
	return Check{ID: fmt.Sprint(flow), Name: FlowNames[flow]}
}

// FingerprintDigest is the attempt fingerprint digest: sha256 over the five fields as sorted, compact JSON.
// HTML escaping is off so the bytes match JSON.stringify in the hub.
func FingerprintDigest(f Fingerprint) string {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(map[string]string{
		"harness": f.Harness, "harness_version": f.HarnessVersion, "model": f.Model,
		"profile": f.Profile, "skill_release": f.SkillRelease,
	})
	sum := sha256.Sum256(bytes.TrimRight(buf.Bytes(), "\n"))
	return "sha256:" + hex.EncodeToString(sum[:])
}

func countGates(doc RunDoc) int {
	n := 0
	for _, v := range doc.Verdicts {
		if v.Kind == "gate" && v.Source == "superpipeline" {
			n++
		}
	}
	return n
}

// CheckRunEvidence is flow 1: the run, card, stage and gates from superpipeline's run evidence route.
func CheckRunEvidence(doc RunDoc, boardID, runID, cardID, workStage string) Check {
	c := check(1)
	wantRef := "superpipeline:" + boardID + "/" + runID
	switch {
	case doc.Source("superpipeline") != "ok":
		c.Detail = "sources.superpipeline is " + doc.Source("superpipeline")
	case doc.Run.Ref != wantRef:
		c.Detail = fmt.Sprintf("run.ref is %s, expected %s", q(doc.Run.Ref), wantRef)
	case doc.Run.Card.ID != cardID:
		c.Detail = fmt.Sprintf("run.card.id is %s, expected %s", q(doc.Run.Card.ID), cardID)
	case doc.Run.Stage != workStage:
		c.Detail = fmt.Sprintf("run.stage is %s, expected %s", q(doc.Run.Stage), workStage)
	case countGates(doc) == 0:
		c.Detail = "no gate from superpipeline on the document"
	default:
		c.Pass = true
		c.Detail = fmt.Sprintf("run %s, card %s, stage %s, %d gate(s); sources.superpipeline ok", runID, cardID, workStage, countGates(doc))
	}
	return c
}

// CheckAttempts is flow 2.
func CheckAttempts(doc RunDoc, expectHarness string) Check {
	c := check(2)
	if s := doc.Source("agentpod"); s != "ok" {
		c.Detail = "sources.agentpod is " + s
		return c
	}
	if len(doc.Attempts) == 0 {
		c.Detail = "the document lists no attempts"
		return c
	}
	var unknown []string
	for _, a := range doc.Attempts {
		fp := a.Fingerprint
		if !digestRE.MatchString(fp.Digest) {
			c.Detail = fmt.Sprintf("attempt %s: fingerprint digest is %q", a.ID, fp.Digest)
			return c
		}
		if got := FingerprintDigest(fp); got != fp.Digest {
			c.Detail = fmt.Sprintf("attempt %s: digest %s does not match its fields (recomputed %s)", a.ID, fp.Digest, got)
			return c
		}
		if expectHarness != "" && fp.Harness != expectHarness {
			c.Detail = fmt.Sprintf("attempt %s: harness %q, expected %q", a.ID, fp.Harness, expectHarness)
			return c
		}
		for name, v := range map[string]string{"harness_version": fp.HarnessVersion, "model": fp.Model,
			"profile": fp.Profile, "skill_release": fp.SkillRelease} {
			if v == "unknown" {
				unknown = append(unknown, a.ID+"."+name)
			}
		}
	}
	sort.Strings(unknown)
	c.Pass = true
	c.Detail = fmt.Sprintf("%d attempt(s) fingerprinted; digests recompute; sources.agentpod ok", len(doc.Attempts))
	if len(unknown) > 0 {
		c.Detail += "; unknown fields: " + strings.Join(unknown, ",")
	}
	return c
}

// CheckHubSpansJoined is flow 3: the trace is joined, and the hub's dispatch, attempt and
// turn spans are in it.
func CheckHubSpansJoined(doc RunDoc, runID string, spans []Span) Check {
	c := check(3)
	if s := doc.Source("traces"); s != "ok" {
		c.Detail = "sources.traces is " + s
		return c
	}
	if st := doc.Trace.Status; (st != "joined" && st != "partial") || len(doc.Trace.TraceIDs) == 0 {
		c.Detail = fmt.Sprintf("trace.status is %s with %d trace id(s)", q(doc.Trace.Status), len(doc.Trace.TraceIDs))
		return c
	}
	if len(doc.Attempts) == 0 {
		c.Detail = "no attempts to match spans against"
		return c
	}
	has := func(op, key, val string) bool {
		for _, s := range spans {
			if s.Service == hubService && s.Operation == op && s.Tags[key] == val {
				return true
			}
		}
		return false
	}
	if !has("dispatch", "run.id", runID) {
		c.Detail = "no dispatch span from agentpod-hub with run.id " + runID
		return c
	}
	for _, a := range doc.Attempts {
		if !has("attempt", "attempt.id", a.ID) {
			c.Detail = "no attempt span for " + a.ID
			return c
		}
		if !has("turn", "attempt.id", a.ID) {
			c.Detail = "no turn span for " + a.ID
			return c
		}
		if n, ok := a.SpanCount(); !ok || n == 0 {
			c.Detail = fmt.Sprintf("attempt %s: span_count is %s", a.ID, q(string(a.SpanCountRaw)))
			return c
		}
	}
	tools := 0
	for _, s := range spans {
		if s.Service == hubService && s.Operation == "tool_call" {
			tools++
		}
	}
	c.Pass = true
	c.Detail = fmt.Sprintf("trace.status %s; dispatch, attempt and turn spans present for %d attempt(s); %d tool_call span(s)",
		doc.Trace.Status, len(doc.Attempts), tools)
	return c
}

// CheckNodeAgentSpan is flow 4: node-agent continued the trace the hub started.
func CheckNodeAgentSpan(spans []Span) Check {
	c := check(4)
	n := 0
	ops := map[string]bool{}
	for _, s := range spans {
		if s.Service == nodeAgentService {
			n++
			ops[s.Operation] = true
		}
	}
	if n == 0 {
		c.Detail = "no agentpod-node-agent span in the run's trace(s)"
		return c
	}
	names := make([]string, 0, len(ops))
	for op := range ops {
		names = append(names, op)
	}
	sort.Strings(names)
	c.Pass = true
	c.Detail = fmt.Sprintf("%d agentpod-node-agent span(s) in the run's trace: %s", n, strings.Join(names, ", "))
	return c
}

// CheckWorkersSpan is flow 5: a superpipeline-api span carrying run.id. It may sit in the
// joined trace (Workers continued traceparent) or only be found by a tag search (Workers
// started their own trace).
func CheckWorkersSpan(runID string, traceSpans, searched []Span, searchErr error) Check {
	c := check(5)
	count := func(spans []Span) int {
		n := 0
		for _, s := range spans {
			if s.Service == workersService && s.Tags["run.id"] == runID {
				n++
			}
		}
		return n
	}
	inTrace, bySearch := count(traceSpans), count(searched)
	if inTrace+bySearch == 0 {
		if searchErr != nil {
			c.Detail = "none in the joined trace, and the VictoriaTraces search failed: " + searchErr.Error()
			return c
		}
		c.Detail = "no superpipeline-api span with run.id " + runID
		return c
	}
	c.Pass = true
	c.Detail = fmt.Sprintf("superpipeline-api spans with run.id %s: in the joined trace: %d; by tag search: %d", runID, inTrace, bySearch)
	return c
}

// findGate picks the superpipeline gate for stage on this run. The service
// shows every gate on the card, including card-level gates (run_id "unknown") and older
// gates for the same stage, so a gate whose run_id is runID beats any other; among equals
// the latest by at wins (an unparsable or "unknown" at sorts oldest; ties keep document order).
func findGate(doc RunDoc, stage, runID string) (DocVerdict, bool) {
	var best DocVerdict
	var bestAt time.Time
	bestMatch, found := false, false
	for _, v := range doc.Verdicts {
		if v.Kind != "gate" || v.Source != "superpipeline" || v.Standard != "stage:"+stage {
			continue
		}
		match := runID != "" && v.RunID == runID
		at, err := time.Parse(time.RFC3339Nano, v.At)
		if err != nil {
			at = time.Time{}
		}
		if !found || (match && !bestMatch) || (match == bestMatch && at.After(bestAt)) {
			best, bestAt, bestMatch, found = v, at, match, true
		}
	}
	return best, found
}

// runGate is the gate tied to runID: the latest gate whose run_id is runID,
// or, because gates opened by advanceCard may carry no run_id, the card's
// ONLY gate for the stage when that gate's run_id is unknown. Anything else is another
// run's gate, or ambiguous, and never stands for this run.
func runGate(doc RunDoc, stage, runID string) (DocVerdict, bool) {
	g, ok := findGate(doc, stage, runID)
	if !ok || (runID != "" && g.RunID == runID) {
		return g, ok
	}
	n := 0
	for _, v := range doc.Verdicts {
		if v.Kind == "gate" && v.Source == "superpipeline" && v.Standard == "stage:"+stage {
			n++
		}
	}
	if n == 1 && unresolvedID(g.RunID) {
		return g, true
	}
	return DocVerdict{}, false
}

// gateDecision is the one reading of a superpipeline gate verdict, shared by CheckGateShown
// and FollowUpGates. It returns value.decision when that is a non-empty string.
// Otherwise it looks at status, because the service (join.gateViews) gives cancelled and
// unknown gates the same {"decision":null} as a pending one: "cancelled" for status
// "cancelled", "pending" ONLY for status "pending", and "unknown" for anything else
// (including "unknown" and empty). Callers must treat only "pending" as awaiting a human.
func gateDecision(v DocVerdict) string {
	if s, ok := v.Value["decision"].(string); ok && s != "" {
		return s
	}
	switch v.Status {
	case "pending":
		return "pending"
	case "cancelled":
		return "cancelled"
	}
	return "unknown"
}

// CheckGateShown is the first half of flow 6. The bool reports that the gate was seen
// PENDING, which is what lets a later run credit the human decision (FollowUpGates).
func CheckGateShown(doc RunDoc, stage, runID string) (Check, bool) {
	c := check(6)
	g, ok := runGate(doc, stage, runID)
	if !ok {
		c.Detail = fmt.Sprintf("no gate for stage:%s tied to run %s (sources.superpipeline is %s)", stage, runID, doc.Source("superpipeline"))
		if latest, seen := findGate(doc, stage, runID); seen {
			c.Detail += fmt.Sprintf("; latest seen: %s (run %s, status %s)", q(latest.ID), q(latest.RunID), q(latest.Status))
		}
		return c, false
	}
	dec := gateDecision(g)
	if dec == "cancelled" || dec == "unknown" {
		c.Detail = fmt.Sprintf("gate %s (stage:%s) has status %s, not pending or resolved", q(g.ID), stage, q(g.Status))
		return c, false
	}
	if dec != "pending" && g.JudgeKind == "agent" {
		c.Detail = "gate shows decision " + dec + " by an agent judge"
		return c, false
	}
	c.Pass = true
	c.Detail = fmt.Sprintf("gate %s (stage:%s) shown, decision %s", q(g.ID), stage, dec)
	return c, dec == "pending"
}

// CheckGraderRoundTrip is flow 7.
func CheckGraderRoundTrip(first, again Verdict, doc RunDoc, principal, standard string) Check {
	c := check(7)
	switch {
	case first.ID == "":
		c.Detail = "record_verdict returned no id"
		return c
	case again.ID != first.ID:
		c.Detail = fmt.Sprintf("retry with the same idempotency key returned %s, first returned %s", q(again.ID), first.ID)
		return c
	case doc.Run.Agent == "" || doc.Run.Agent == "unknown":
		c.Detail = "run.agent is unknown, so grader/agent distinctness cannot be shown"
		return c
	case principal == "":
		c.Detail = "the canary does not know its own principal"
		return c
	case principal == doc.Run.Agent:
		c.Detail = "the grader principal is the executing agent"
		return c
	}
	if s := doc.Source("verdicts"); s != "ok" {
		c.Detail = "sources.verdicts is " + s
		return c
	}
	var match []DocVerdict
	for _, v := range doc.Verdicts {
		if v.Kind == "grader" && (v.ID == first.ID || (v.ID == "" && v.Judge == principal && v.Standard == standard)) {
			match = append(match, v)
		}
	}
	if len(match) == 0 {
		c.Detail = "verdict " + first.ID + " not on the run document read back over MCP"
		return c
	}
	if len(match) > 1 {
		c.Detail = fmt.Sprintf("verdict %s appears %d copies on the run document; the retry duplicated it", first.ID, len(match))
		return c
	}
	v := match[0]
	switch {
	case v.Judge != principal:
		c.Detail = fmt.Sprintf("verdict judge is %s, expected the canary %s", q(v.Judge), principal)
	case v.JudgeKind != "grader":
		c.Detail = "verdict judge_kind is " + q(v.JudgeKind) + ", expected grader"
	case v.Standard != standard:
		c.Detail = "verdict standard is " + q(v.Standard)
	default:
		c.Pass = true
		c.Detail = "verdict " + first.ID + " read back over MCP once; retry returned the same id; judge distinct from agent " + doc.Run.Agent
	}
	return c
}

// CheckLinkedLogsAndErrors is flow 8. It counts lines; it never copies a log message into
// its output, because a message is exactly where leaked content would be.
func CheckLinkedLogsAndErrors(doc RunDoc, lines []LogLine, logsErr error, runID, nonce string) Check {
	c := check(8)
	if logsErr != nil {
		c.Detail = "GET …/logs: " + logsErr.Error()
		return c
	}
	traces := map[string]bool{}
	for _, id := range doc.Trace.TraceIDs {
		traces[id] = true
	}
	linked := 0
	for _, l := range lines {
		if l.Service == canaryService {
			continue
		}
		if l.RunID == runID || (l.TraceID != "" && traces[l.TraceID]) {
			linked++
		}
	}
	if linked == 0 {
		c.Detail = fmt.Sprintf("none of %d line(s) from …/logs is a product line carrying run.id %s or the run's trace id", len(lines), runID)
		return c
	}
	if s := doc.Source("errors"); s != "ok" {
		c.Detail = fmt.Sprintf("%d linked product log line(s), but sources.errors is %s", linked, s)
		return c
	}
	for _, e := range doc.Errors {
		if e.Service == canaryService && strings.Contains(e.Message, nonce) {
			c.Pass = true
			c.Detail = fmt.Sprintf("%d linked product log line(s) from …/logs; injected error %s shown in errors", linked, nonce)
			return c
		}
	}
	c.Detail = fmt.Sprintf("%d linked product log line(s), but the injected error %s is not in errors (%d error(s) shown)", linked, nonce, len(doc.Errors))
	return c
}
