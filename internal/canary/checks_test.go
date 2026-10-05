package canary

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

// testNonce is what newNonce makes from zeroRand after the markers have used 20 bytes.
const testNonce = "swcerr-00000000"

func goodFingerprint() Fingerprint {
	fp := Fingerprint{Harness: "hermes", HarnessVersion: "0.9.1", Model: "claude-x", Profile: "coder",
		SkillRelease: "2026.10.01", ReportedBy: "harness"}
	fp.Digest = FingerprintDigest(fp)
	return fp
}

func goodDoc() RunDoc {
	return RunDoc{
		Run: DocRun{Ref: "superpipeline:brd_c/run_1", Card: DocCard{ID: "crd_1", Title: "superwitness canary"},
			Stage: "work", Agent: "prn_exec01", State: "completed"},
		Attempts: []DocAttempt{{ID: "attempt_1", Station: "station_g", State: "ended",
			Fingerprint: goodFingerprint(), SpanCountRaw: json.RawMessage(`12`)}},
		Trace: DocTrace{TraceIDs: []string{"t1"}, Status: "joined"},
		Errors: []DocError{{Service: "superwitness-canary", TraceID: "t1",
			Message: "superwitness canary injected error " + testNonce + " (synthetic; safe to ignore)"}},
		Verdicts: []DocVerdict{{ID: "gate_1", Kind: "gate", Source: "superpipeline", Value: map[string]any{"decision": nil},
			Status: "pending", Judge: "unknown", JudgeKind: "unknown", Standard: "stage:review"}},
		Sources: map[string]string{"superpipeline": "ok", "agentpod": "ok", "traces": "ok", "logs": "ok",
			"errors": "ok", "verdicts": "ok"},
	}
}

func gradedDoc() RunDoc {
	d := goodDoc()
	d.Verdicts = append(d.Verdicts, DocVerdict{ID: "vrd_1", Kind: "grader", Source: "superwitness",
		Value: map[string]any{"score": 1.0}, Judge: "prn_canary", JudgeKind: "grader",
		Standard: "rubric:superwitness-canary@1"})
	return d
}

func goodSpans() []Span {
	return []Span{
		{TraceID: "t1", Service: "agentpod-hub", Operation: "dispatch", Tags: map[string]string{"run.id": "run_1"}},
		{TraceID: "t1", Service: "agentpod-hub", Operation: "attempt", Tags: map[string]string{"attempt.id": "attempt_1"}},
		{TraceID: "t1", Service: "agentpod-hub", Operation: "turn", Tags: map[string]string{"attempt.id": "attempt_1"}},
		{TraceID: "t1", Service: "agentpod-hub", Operation: "tool_call", Tags: map[string]string{"attempt.id": "attempt_1", "tool.kind": "edit"}},
		{TraceID: "t1", Service: "agentpod-node-agent", Operation: "acp session/prompt", Tags: map[string]string{"attempt.id": "attempt_1"}},
	}
}

func workersSpans() []Span {
	return []Span{{TraceID: "t3", Service: "superpipeline-api", Operation: "POST /v1/boards/:id/runs/:runId/activities",
		Tags: map[string]string{"run.id": "run_1"}}}
}

func goodLogs() []LogLine {
	return []LogLine{
		{Service: "agentpod-hub", Level: "info", Message: "bridge claimed the card", TraceID: "t1", RunID: "run_1"},
		{Service: "superwitness-canary", Level: "error", Message: "injected", TraceID: "t1", RunID: "run_1"},
	}
}

func TestFingerprintDigestIsC3(t *testing.T) {
	fp := Fingerprint{Harness: "hermes", HarnessVersion: "0.9.1", Model: "a<b&c", Profile: "coder", SkillRelease: "unknown", ReportedBy: "hub"}
	canonical := `{"harness":"hermes","harness_version":"0.9.1","model":"a<b&c","profile":"coder","skill_release":"unknown"}`
	sum := sha256.Sum256([]byte(canonical))
	if got, want := FingerprintDigest(fp), "sha256:"+hex.EncodeToString(sum[:]); got != want {
		t.Fatalf("digest %s, want %s (reported_by must not count; < and & must not be escaped)", got, want)
	}
}

func TestCheckRunEvidence(t *testing.T) {
	cases := map[string]struct {
		mutate func(*RunDoc)
		pass   bool
		detail string
	}{
		"healthy":            {func(*RunDoc) {}, true, "1 gate(s)"},
		"superpipeline down": {func(d *RunDoc) { d.Sources["superpipeline"] = "unavailable" }, false, "sources.superpipeline is unavailable"},
		"wrong ref":          {func(d *RunDoc) { d.Run.Ref = "superpipeline:brd_c/run_9" }, false, "run.ref"},
		"card missing":       {func(d *RunDoc) { d.Run.Card = DocCard{} }, false, "run.card.id is missing"},
		"wrong stage":        {func(d *RunDoc) { d.Run.Stage = "review" }, false, "run.stage"},
		"no gates":           {func(d *RunDoc) { d.Verdicts = nil }, false, "no gate"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			d := goodDoc()
			tc.mutate(&d)
			c := CheckRunEvidence(d, "brd_c", "run_1", "crd_1", "work")
			if c.ID != "1" || c.Pass != tc.pass || !strings.Contains(c.Detail, tc.detail) {
				t.Fatalf("got %+v", c)
			}
		})
	}
}

func TestCheckAttempts(t *testing.T) {
	cases := map[string]struct {
		mutate func(*RunDoc)
		pass   bool
		detail string
	}{
		"healthy":           {func(*RunDoc) {}, true, "1 attempt(s)"},
		"no attempts":       {func(d *RunDoc) { d.Attempts = nil }, false, "no attempts"},
		"digest unknown":    {func(d *RunDoc) { d.Attempts[0].Fingerprint.Digest = "unknown" }, false, `"unknown"`},
		"digest mismatched": {func(d *RunDoc) { d.Attempts[0].Fingerprint.Model = "other" }, false, "does not match"},
		"wrong harness": {func(d *RunDoc) {
			fp := d.Attempts[0].Fingerprint
			fp.Harness = "codex"
			fp.Digest = FingerprintDigest(fp)
			d.Attempts[0].Fingerprint = fp
		}, false, `expected "hermes"`},
		"agentpod down": {func(d *RunDoc) { d.Sources["agentpod"] = "timeout" }, false, "sources.agentpod is timeout"},
		"unknown field noted": {func(d *RunDoc) {
			fp := d.Attempts[0].Fingerprint
			fp.Model = "unknown"
			fp.Digest = FingerprintDigest(fp)
			d.Attempts[0].Fingerprint = fp
		}, true, "attempt_1.model"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			d := goodDoc()
			tc.mutate(&d)
			c := CheckAttempts(d, "hermes")
			if c.ID != "2" || c.Pass != tc.pass || !strings.Contains(c.Detail, tc.detail) {
				t.Fatalf("got %+v", c)
			}
		})
	}
}

func without(spans []Span, op string) []Span {
	var out []Span
	for _, s := range spans {
		if s.Operation != op {
			out = append(out, s)
		}
	}
	return out
}

func TestCheckHubSpansJoined(t *testing.T) {
	moved := goodSpans()
	moved[2].Service = "superpipeline-api"
	otherRun := goodSpans()
	otherRun[0].Tags = map[string]string{"run.id": "run_9"}
	cases := map[string]struct {
		spans  []Span
		mutate func(*RunDoc)
		pass   bool
		detail string
	}{
		"healthy":                 {goodSpans(), func(*RunDoc) {}, true, "1 tool_call"},
		"partial trace":           {goodSpans(), func(d *RunDoc) { d.Trace.Status = "partial" }, true, "partial"},
		"no trace":                {goodSpans(), func(d *RunDoc) { d.Trace = DocTrace{Status: "none"} }, false, "trace.status is none"},
		"joined, no ids":          {goodSpans(), func(d *RunDoc) { d.Trace.TraceIDs = nil }, false, "0 trace id(s)"},
		"traces timeout":          {goodSpans(), func(d *RunDoc) { d.Sources["traces"] = "timeout" }, false, "sources.traces is timeout"},
		"no dispatch":             {without(goodSpans(), "dispatch"), func(*RunDoc) {}, false, "no dispatch span"},
		"dispatch for another":    {otherRun, func(*RunDoc) {}, false, "no dispatch span"},
		"no attempt span":         {without(goodSpans(), "attempt"), func(*RunDoc) {}, false, "no attempt span for attempt_1"},
		"no turn span":            {without(goodSpans(), "turn"), func(*RunDoc) {}, false, "no turn span for attempt_1"},
		"turn from wrong service": {moved, func(*RunDoc) {}, false, "no turn span"},
		"span_count unknown":      {goodSpans(), func(d *RunDoc) { d.Attempts[0].SpanCountRaw = json.RawMessage(`"unknown"`) }, false, "span_count"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			d := goodDoc()
			tc.mutate(&d)
			c := CheckHubSpansJoined(d, "run_1", tc.spans)
			if c.ID != "3" || c.Pass != tc.pass || !strings.Contains(c.Detail, tc.detail) {
				t.Fatalf("got %+v", c)
			}
		})
	}
}

func TestCheckNodeAgentSpan(t *testing.T) {
	if c := CheckNodeAgentSpan(goodSpans()); c.ID != "4" || !c.Pass || !strings.Contains(c.Detail, "1 agentpod-node-agent span") {
		t.Fatalf("got %+v", c)
	}
	if c := CheckNodeAgentSpan(without(goodSpans(), "acp session/prompt")); c.Pass {
		t.Fatalf("passed without a node-agent span: %+v", c)
	}
}

func TestCheckWorkersSpan(t *testing.T) {
	wrongRun := workersSpans()
	wrongRun[0].Tags = map[string]string{"run.id": "run_9"}
	cases := map[string]struct {
		trace, searched []Span
		err             error
		pass            bool
		detail          string
	}{
		"found by search":         {goodSpans(), workersSpans(), nil, true, "by tag search: 1"},
		"found in joined trace":   {append(goodSpans(), workersSpans()...), nil, nil, true, "in the joined trace: 1"},
		"absent":                  {goodSpans(), nil, nil, false, "no superpipeline-api span with run.id run_1"},
		"other run's span":        {goodSpans(), wrongRun, nil, false, "no superpipeline-api span"},
		"search failed, absent":   {goodSpans(), nil, errors.New("HTTP 502"), false, "search failed"},
		"search failed, in trace": {append(goodSpans(), workersSpans()...), nil, errors.New("HTTP 502"), true, "in the joined trace: 1"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			c := CheckWorkersSpan("run_1", tc.trace, tc.searched, tc.err)
			if c.ID != "5" || c.Pass != tc.pass || !strings.Contains(c.Detail, tc.detail) {
				t.Fatalf("got %+v", c)
			}
		})
	}
}

func TestCheckGateShown(t *testing.T) {
	cases := map[string]struct {
		mutate        func(*RunDoc)
		pass, pending bool
	}{
		"pending": {func(*RunDoc) {}, true, true},
		"approved by human": {func(d *RunDoc) {
			d.Verdicts[0].Value = map[string]any{"decision": "approved"}
			d.Verdicts[0].Status = "resolved"
			d.Verdicts[0].JudgeKind = "human"
		}, true, false},
		"decided by agent": {func(d *RunDoc) {
			d.Verdicts[0].Value = map[string]any{"decision": "approved"}
			d.Verdicts[0].JudgeKind = "agent"
		}, false, false},
		"missing":            {func(d *RunDoc) { d.Verdicts = nil }, false, false},
		"other stage":        {func(d *RunDoc) { d.Verdicts[0].Standard = "stage:work" }, false, false},
		"not superpipeline":  {func(d *RunDoc) { d.Verdicts[0].Source = "superwitness" }, false, false},
		"cancelled":          {func(d *RunDoc) { d.Verdicts[0].Status = "cancelled" }, false, false},
		"unknown status":     {func(d *RunDoc) { d.Verdicts[0].Status = "unknown" }, false, false},
		"empty status":       {func(d *RunDoc) { d.Verdicts[0].Status = "" }, false, false},
		"another run's gate": {func(d *RunDoc) { d.Verdicts[0].RunID = "run_0" }, false, false},
		"this run's gate":    {func(d *RunDoc) { d.Verdicts[0].RunID = "run_1" }, true, true},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			d := goodDoc()
			tc.mutate(&d)
			c, pending := CheckGateShown(d, "review", "run_1")
			if c.ID != "6" || c.Pass != tc.pass || pending != tc.pending {
				t.Fatalf("got %+v pending=%v", c, pending)
			}
		})
	}
}

func TestCheckGraderRoundTrip(t *testing.T) {
	const std = "rubric:superwitness-canary@1"
	v := Verdict{ID: "vrd_1", Judge: "prn_canary", JudgeKind: "grader"}
	cases := map[string]struct {
		first, again Verdict
		principal    string
		mutate       func(*RunDoc)
		pass         bool
		detail       string
	}{
		"healthy":            {v, v, "prn_canary", func(*RunDoc) {}, true, "read back"},
		"no id":              {Verdict{}, Verdict{}, "prn_canary", func(*RunDoc) {}, false, "no id"},
		"not idempotent":     {v, Verdict{ID: "vrd_2"}, "prn_canary", func(*RunDoc) {}, false, "idempotency"},
		"duplicated on read": {v, v, "prn_canary", func(d *RunDoc) { d.Verdicts = append(d.Verdicts, d.Verdicts[1]) }, false, "2 copies"},
		"grader is agent":    {v, v, "prn_exec01", func(*RunDoc) {}, false, "executing agent"},
		"agent unknown":      {v, v, "prn_canary", func(d *RunDoc) { d.Run.Agent = "unknown" }, false, "run.agent is unknown"},
		"not on document":    {v, v, "prn_canary", func(d *RunDoc) { d.Verdicts = d.Verdicts[:1] }, false, "not on the run document"},
		"kind from claim":    {v, v, "prn_canary", func(d *RunDoc) { d.Verdicts[1].JudgeKind = "agent" }, false, "expected grader"},
		"wrong judge":        {v, v, "prn_canary", func(d *RunDoc) { d.Verdicts[1].Judge = "prn_other" }, false, "verdict judge is prn_other"},
		"verdicts down":      {v, v, "prn_canary", func(d *RunDoc) { d.Sources["verdicts"] = "unavailable" }, false, "sources.verdicts"},
		"matched without id": {v, v, "prn_canary", func(d *RunDoc) { d.Verdicts[1].ID = "" }, true, "read back"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			d := gradedDoc()
			tc.mutate(&d)
			c := CheckGraderRoundTrip(tc.first, tc.again, d, tc.principal, std)
			if c.ID != "7" || c.Pass != tc.pass || !strings.Contains(c.Detail, tc.detail) {
				t.Fatalf("got %+v", c)
			}
		})
	}
}

func TestCheckLinkedLogsAndErrors(t *testing.T) {
	onlyCanary := goodLogs()[1:]
	unlinked := []LogLine{{Service: "agentpod-hub", RunID: "run_9", TraceID: "t9"}}
	byTrace := []LogLine{{Service: "superpipeline-api", TraceID: "t1"}}
	cases := map[string]struct {
		lines  []LogLine
		err    error
		mutate func(*RunDoc)
		pass   bool
		detail string
	}{
		"healthy":                {goodLogs(), nil, func(*RunDoc) {}, true, "injected error " + testNonce},
		"linked by trace id":     {byTrace, nil, func(*RunDoc) {}, true, "1 linked"},
		"logs API failed":        {nil, errors.New("HTTP 503"), func(*RunDoc) {}, false, "HTTP 503"},
		"only the canary's line": {onlyCanary, nil, func(*RunDoc) {}, false, "product line"},
		"unlinked lines":         {unlinked, nil, func(*RunDoc) {}, false, "product line"},
		"errors source down":     {goodLogs(), nil, func(d *RunDoc) { d.Sources["errors"] = "timeout" }, false, "sources.errors is timeout"},
		"injected error absent":  {goodLogs(), nil, func(d *RunDoc) { d.Errors = nil }, false, "not in errors"},
		"other service's error":  {goodLogs(), nil, func(d *RunDoc) { d.Errors[0].Service = "agentpod-hub" }, false, "not in errors"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			d := goodDoc()
			tc.mutate(&d)
			c := CheckLinkedLogsAndErrors(d, tc.lines, tc.err, "run_1", testNonce)
			if c.ID != "8" || c.Pass != tc.pass || !strings.Contains(c.Detail, tc.detail) {
				t.Fatalf("got %+v", c)
			}
		})
	}
}

func TestGateDecisionAndStatusDetail(t *testing.T) {
	for status, want := range map[string]string{"pending": "pending", "cancelled": "cancelled", "unknown": "unknown", "": "unknown", "weird": "unknown"} {
		g := DocVerdict{Value: map[string]any{"decision": nil}, Status: status}
		if got := gateDecision(g); got != want {
			t.Errorf("status %q: got %q want %q", status, got, want)
		}
	}
	if got := gateDecision(DocVerdict{Value: map[string]any{"decision": "approved"}, Status: "resolved"}); got != "approved" {
		t.Errorf("got %q", got)
	}
	for _, st := range []string{"cancelled", "unknown"} {
		d := goodDoc()
		d.Verdicts[0].Status = st
		c, pending := CheckGateShown(d, "review", "run_1")
		if c.Pass || pending || !strings.Contains(c.Detail, st) {
			t.Errorf("%s: got %+v pending=%v", st, c, pending)
		}
	}
}

func TestFindGateChoosesThisRunsGateElseTheLatest(t *testing.T) {
	gate := func(id, runID, at string) DocVerdict {
		return DocVerdict{ID: id, Kind: "gate", Source: "superpipeline", Standard: "stage:review", RunID: runID, At: at,
			Value: map[string]any{"decision": nil}, Status: "pending"}
	}
	cases := map[string]struct {
		gates []DocVerdict
		want  string
	}{
		"run_id match beats a later gate": {[]DocVerdict{
			gate("g_mine", "run_1", "2026-10-04T10:00:00Z"), gate("g_later", "unknown", "2026-10-04T11:00:00Z")}, "g_mine"},
		"no match, latest wins": {[]DocVerdict{
			gate("g_old", "run_0", "2026-10-04T10:00:00Z"), gate("g_new", "unknown", "2026-10-04T11:00:00Z"),
			gate("g_mid", "run_9", "2026-10-04T10:30:00Z")}, "g_new"},
		"latest of this run's gates": {[]DocVerdict{
			gate("g_first", "run_1", "2026-10-04T10:00:00Z"), gate("g_second", "run_1", "2026-10-04T10:10:00.5Z"),
			gate("g_other", "run_0", "2026-10-04T12:00:00Z")}, "g_second"},
		"unknown at sorts oldest": {[]DocVerdict{
			gate("g_dated", "unknown", "2026-10-04T10:00:00Z"), gate("g_undated", "unknown", "unknown")}, "g_dated"},
		"ties keep document order": {[]DocVerdict{
			gate("g_a", "unknown", "unknown"), gate("g_b", "unknown", "")}, "g_a"},
		"other stages and sources ignored": {[]DocVerdict{
			gate("g_mine", "run_1", "2026-10-04T10:00:00Z"),
			{ID: "g_stage", Kind: "gate", Source: "superpipeline", Standard: "stage:work", RunID: "run_1", At: "2026-10-04T12:00:00Z"},
			{ID: "g_owned", Kind: "gate", Source: "superwitness", Standard: "stage:review", RunID: "run_1", At: "2026-10-04T12:00:00Z"}}, "g_mine"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			g, ok := findGate(RunDoc{Verdicts: tc.gates}, "review", "run_1")
			if !ok || g.ID != tc.want {
				t.Fatalf("got %q ok=%v, want %q", g.ID, ok, tc.want)
			}
		})
	}
	if _, ok := findGate(RunDoc{}, "review", "run_1"); ok {
		t.Error("found a gate in an empty document")
	}
}

func TestCheckGateShownNamesTheLatestGateWhenNoneIsTiedToTheRun(t *testing.T) {
	d := goodDoc()
	d.Verdicts[0].RunID = "run_0"
	c, pending := CheckGateShown(d, "review", "run_1")
	if c.Pass || pending || !strings.Contains(c.Detail, "no gate for stage:review tied to run run_1") ||
		!strings.Contains(c.Detail, "latest seen: gate_1 (run run_0") {
		t.Fatalf("got %+v pending=%v", c, pending)
	}
}
