package canary

import (
	"bytes"
	"encoding/json"
	"io"
)

// RunDoc is the canary's own reading of the RunDocument and the spans and
// logs routes. It is
// deliberately not the service's type: a field the service renames should fail the canary.
type RunDoc struct {
	Run      DocRun            `json:"run"`
	Attempts []DocAttempt      `json:"attempts"`
	Trace    DocTrace          `json:"trace"`
	Errors   []DocError        `json:"errors"`
	Verdicts []DocVerdict      `json:"verdicts"`
	Sources  map[string]string `json:"sources"`
}

type DocRun struct {
	Ref   string  `json:"ref"`
	Card  DocCard `json:"card"`
	Stage string  `json:"stage"`
	Agent string  `json:"agent"`
	State string  `json:"state"`
}

type DocCard struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

type DocAttempt struct {
	ID           string          `json:"id"`
	Station      string          `json:"station"`
	State        string          `json:"state"`
	Fingerprint  Fingerprint     `json:"fingerprint"`
	SpanCountRaw json.RawMessage `json:"span_count"`
}

type Fingerprint struct {
	Digest         string `json:"digest"`
	Harness        string `json:"harness"`
	HarnessVersion string `json:"harness_version"`
	Model          string `json:"model"`
	Profile        string `json:"profile"`
	SkillRelease   string `json:"skill_release"`
	ReportedBy     string `json:"reported_by"`
}

// DocTrace.Sampled is raw: the service encodes it as true, false or the string
// "unknown" (a tri-state flag), so a *bool would fail on every unknown document.
type DocTrace struct {
	TraceIDs []string        `json:"trace_ids"`
	Status   string          `json:"status"`
	Sampled  json.RawMessage `json:"sampled"`
}

// SampledState is "true", "false", "unknown", or "" when sampled is absent or null.
func (t DocTrace) SampledState() string {
	s := string(bytes.TrimSpace(t.Sampled))
	switch s {
	case "true", "false":
		return s
	case "", "null":
		return ""
	}
	var str string
	if err := json.Unmarshal(t.Sampled, &str); err == nil {
		return str
	}
	return s
}

type DocError struct {
	Service string `json:"service"`
	Message string `json:"message"`
	TraceID string `json:"trace_id"`
	At      string `json:"at"`
}

type DocVerdict struct {
	ID        string         `json:"id"`
	Kind      string         `json:"kind"`
	Source    string         `json:"source"`
	Value     map[string]any `json:"value"`
	Status    string         `json:"status"`
	Judge     string         `json:"judge"`
	JudgeKind string         `json:"judge_kind"`
	Standard  string         `json:"standard"`
	At        string         `json:"at"`
	RunID     string         `json:"run_id"` // gates only: the run the gate belongs to, "unknown" for a card-level gate
}

// LogLine is one element of GET …/logs.
type LogLine struct {
	Service string `json:"service"`
	Level   string `json:"level"`
	Message string `json:"message"`
	TraceID string `json:"trace_id"`
	RunID   string `json:"run_id"`
	At      string `json:"at"`
}

// SpanCount is false when span_count is "unknown", absent, or not a number.
func (a DocAttempt) SpanCount() (int, bool) {
	var n int
	if err := json.Unmarshal(a.SpanCountRaw, &n); err != nil {
		return 0, false
	}
	return n, true
}

// Source returns sources.<name>, or "missing" when the document omits it.
func (d RunDoc) Source(name string) string {
	if v := d.Sources[name]; v != "" {
		return v
	}
	return "missing"
}

func DecodeRunDoc(r io.Reader) (RunDoc, error) {
	var d RunDoc
	err := json.NewDecoder(r).Decode(&d)
	return d, err
}
