package source

import "time"

// RunFragment is superpipeline's run evidence response.
type RunFragment struct {
	Run   SPRun    `json:"run"`
	Card  SPCard   `json:"card"`
	Gates []SPGate `json:"gates"`
	Usage SPUsage  `json:"usage"`
	AsOf  string   `json:"as_of"`
}

type SPRun struct {
	ID       string `json:"id"`
	BoardID  string `json:"board_id"`
	CardID   string `json:"card_id"`
	StageKey string `json:"stage_key"`
	AgentID  string `json:"agent_id"`
	// AgentPrincipalID is the executing agent's hub principal (prn_…); "" when superpipeline sends null.
	AgentPrincipalID string     `json:"agent_principal_id"`
	Status           string     `json:"status"`
	Outcome          *string    `json:"outcome"`
	StartedAt        time.Time  `json:"started_at"`
	EndedAt          *time.Time `json:"ended_at"`
}

type SPCard struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	StageKey string `json:"stage_key"`
}

type SPGate struct {
	ID        string  `json:"id"`
	RunID     *string `json:"run_id"`
	StageKey  string  `json:"stage_key"`
	Status    string  `json:"status"`     // pending | resolved | cancelled
	Decision  *string `json:"decision"`   // approved | changes_requested | rejected | null
	DecidedBy *string `json:"decided_by"` // a superpipeline id: usr_…, agt_… or an unlinked hub sub
	// The *_principal_id fields are prn_… ids, or "" when superpipeline sends null (unmapped).
	DecidedByPrincipalID string `json:"decided_by_principal_id"`
	// DecidedByHubSub: the hub auth user id when decided_by_principal_id is null.
	DecidedByHubSub       string     `json:"decided_by_hub_sub"`
	ProducedBy            string     `json:"produced_by"`
	ProducedByPrincipalID string     `json:"produced_by_principal_id"`
	CreatedAt             time.Time  `json:"created_at"`
	ResolvedAt            *time.Time `json:"resolved_at"`
}

type SPUsage struct {
	Status       string   `json:"status"`
	InputTokens  *int64   `json:"input_tokens"`
	OutputTokens *int64   `json:"output_tokens"`
	CostUSD      *float64 `json:"cost_usd"`
}

// LedgerFragment is the hub's run evidence response. BoardID, CardID and Dispatch are
// null when the ledger holds no row for the run.
type LedgerFragment struct {
	ExternalSource string          `json:"external_source"`
	ExternalRunID  string          `json:"external_run_id"`
	BoardID        *string         `json:"board_id"`
	CardID         *string         `json:"card_id"`
	Dispatch       *Dispatch       `json:"dispatch"`
	Attempts       []LedgerAttempt `json:"attempts"`
	AsOf           string          `json:"as_of"`
}

type Dispatch struct {
	Outcome   string    `json:"outcome"`
	Detail    *string   `json:"detail"`
	StationID string    `json:"station_id"`
	UpdatedAt time.Time `json:"updated_at"`
}

type LedgerAttempt struct {
	ID          string      `json:"id"`
	StationID   string      `json:"station_id"`
	SessionID   string      `json:"session_id"`
	State       string      `json:"state"`
	StartSeq    int64       `json:"start_seq"`
	EndSeq      *int64      `json:"end_seq"`
	StartedAt   time.Time   `json:"started_at"`
	EndedAt     *time.Time  `json:"ended_at"`
	Fingerprint Fingerprint `json:"fingerprint"`
	// AgentPrincipalID: the prn_… the station runs as; "" when the hub sends null.
	AgentPrincipalID string `json:"agent_principal_id"`
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

// AttemptLink is the hub's `GET /api/evidence/attempts/:id` response.
type AttemptLink struct {
	ExternalSource *string `json:"external_source"`
	ExternalRunID  *string `json:"external_run_id"`
	BoardID        *string `json:"board_id"`
}

// Ref returns the canonical run an attempt belongs to, or false when it was not dispatched.
func (l AttemptLink) Ref() (RunRef, bool) {
	if l.ExternalSource == nil || *l.ExternalSource != string(Superpipeline) || l.ExternalRunID == nil || l.BoardID == nil {
		return RunRef{}, false
	}
	ref, err := NewSuperpipelineRef(*l.BoardID, *l.ExternalRunID)
	return ref, err == nil
}

type Span struct {
	TraceID      string            `json:"trace_id"`
	SpanID       string            `json:"span_id"`
	ParentSpanID string            `json:"parent_span_id,omitempty"`
	Name         string            `json:"name"`
	Service      string            `json:"service"`
	Start        time.Time         `json:"start"`
	DurationMS   float64           `json:"duration_ms"`
	Attributes   map[string]string `json:"attributes"`
}

type TracesFragment struct {
	TraceIDs []string // ordered by each trace's earliest span
	Spans    []Span
}

type LogLine struct {
	At      time.Time `json:"at"`
	Service string    `json:"service"`
	Level   string    `json:"level"`
	Message string    `json:"message"`
	TraceID string    `json:"trace_id,omitempty"`
	SpanID  string    `json:"span_id,omitempty"`
	RunID   string    `json:"run_id,omitempty"`
}

type LogsFragment struct{ Count int }

type ErrorLine struct {
	Service string
	Message string
	TraceID string
	At      time.Time
}

type ErrorsFragment struct{ Errors []ErrorLine }
