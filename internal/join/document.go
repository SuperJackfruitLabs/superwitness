// Package join builds the RunDocument from every source at read time.
package join

import (
	"encoding/json"
	"strconv"
	"time"

	"github.com/SuperJackfruitLabs/superwitness/internal/source"
)

const (
	Unknown      = "unknown"
	TraceJoined  = "joined"
	TracePartial = "partial"
	TraceNone    = "none"
	TraceUnknown = "unknown"
)

type RunDocument struct {
	Run      RunSection                     `json:"run"`
	Attempts []Attempt                      `json:"attempts"`
	Trace    TraceSection                   `json:"trace"`
	Errors   []ErrorItem                    `json:"errors"`
	Verdicts []VerdictView                  `json:"verdicts"`
	Cost     Cost                           `json:"cost"`
	LogCount Count                          `json:"log_count"`
	Sources  map[string]source.SourceStatus `json:"sources"`
	Links    Links                          `json:"links"`
}

type RunSection struct {
	Ref       string   `json:"ref"`
	Card      CardView `json:"card"`
	Stage     string   `json:"stage"`
	Agent     string   `json:"agent"`     // superpipeline's run.agent_principal_id, or "unknown" when null
	AgentRef  string   `json:"agent_ref"` // superpipeline's own agent id (agt_…)
	State     string   `json:"state"`
	StartedAt string   `json:"started_at"`
	EndedAt   *string  `json:"ended_at"` // null: not ended; "unknown": no source could say
}

type CardView struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

type Attempt struct {
	ID          string      `json:"id"`
	Station     string      `json:"station"`
	State       string      `json:"state"`
	SessionID   string      `json:"session_id"`
	SeqFrom     int64       `json:"seq_from"`
	SeqTo       *int64      `json:"seq_to"`
	StartedAt   string      `json:"started_at"`
	EndedAt     *string     `json:"ended_at"`
	Fingerprint Fingerprint `json:"fingerprint"`
	SpanCount   Count       `json:"span_count"`
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

type TraceSection struct {
	TraceIDs []string `json:"trace_ids"`
	Status   string   `json:"status"`
	Sampled  Flag     `json:"sampled"`
}

type ErrorItem struct {
	Service string `json:"service"`
	Message string `json:"message"`
	TraceID string `json:"trace_id"`
	At      string `json:"at"`
}

type VerdictView struct {
	ID         string          `json:"id"`
	Kind       string          `json:"kind"`
	Source     string          `json:"source"`
	Subject    string          `json:"subject"`
	Value      json.RawMessage `json:"value"`
	Judge      string          `json:"judge"`
	JudgeKind  string          `json:"judge_kind"`
	Standard   string          `json:"standard"`
	Status     string          `json:"status,omitempty"` // gates only: pending | resolved | cancelled
	Comment    string          `json:"comment,omitempty"`
	At         string          `json:"at"`
	RunID      string          `json:"run_id,omitempty"`
	Supersedes *string         `json:"supersedes,omitempty"`
}

type Cost struct {
	Status       string   `json:"status"` // reported | unreported | unknown
	InputTokens  *int64   `json:"input_tokens"`
	OutputTokens *int64   `json:"output_tokens"`
	USD          *float64 `json:"usd"`
}

type Links struct {
	Self  string `json:"self"`
	Spans string `json:"spans"`
	Logs  string `json:"logs"`
}

// Count is a number a source supplied, or "unknown" when it could not. Never a silent 0.
type Count struct {
	N     int
	Known bool
}

func KnownCount(n int) Count { return Count{N: n, Known: true} }

func (c Count) MarshalJSON() ([]byte, error) {
	if !c.Known {
		return []byte(`"unknown"`), nil
	}
	return []byte(strconv.Itoa(c.N)), nil
}

func (c *Count) UnmarshalJSON(b []byte) error {
	if string(b) == `"unknown"` {
		*c = Count{}
		return nil
	}
	n, err := strconv.Atoi(string(b))
	if err != nil {
		return err
	}
	*c = KnownCount(n)
	return nil
}

// Flag is true/false, or "unknown".
type Flag struct {
	V     bool
	Known bool
}

func (f Flag) MarshalJSON() ([]byte, error) {
	if !f.Known {
		return []byte(`"unknown"`), nil
	}
	return json.Marshal(f.V)
}

func (f *Flag) UnmarshalJSON(b []byte) error {
	if string(b) == `"unknown"` {
		*f = Flag{}
		return nil
	}
	var v bool
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	*f = Flag{V: v, Known: true}
	return nil
}

func orUnknown(s string) string {
	if s == "" {
		return Unknown
	}
	return s
}

func fmtTime(t time.Time) string {
	if t.IsZero() {
		return Unknown
	}
	return t.UTC().Format(time.RFC3339Nano)
}

func fmtTimePtr(t *time.Time) *string {
	if t == nil {
		return nil
	}
	s := fmtTime(*t)
	return &s
}

func unknownPtr() *string {
	s := Unknown
	return &s
}

func newDocument(ref source.RunRef) RunDocument {
	self := "/v1/runs/superpipeline/" + ref.BoardID + "/" + ref.RunID
	return RunDocument{
		Run: RunSection{Ref: ref.String(), Card: CardView{ID: Unknown, Title: Unknown}, Stage: Unknown, Agent: Unknown,
			AgentRef: Unknown, State: Unknown, StartedAt: Unknown, EndedAt: unknownPtr()},
		Attempts: []Attempt{},
		Trace:    TraceSection{TraceIDs: []string{}, Status: TraceUnknown},
		Errors:   []ErrorItem{},
		Verdicts: []VerdictView{},
		Cost:     Cost{Status: Unknown},
		Sources:  map[string]source.SourceStatus{},
		Links:    Links{Self: self, Spans: self + "/spans", Logs: self + "/logs"},
	}
}
