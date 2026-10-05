// Package runs is the run registry. Sources report their runs to superwitness, which keeps one
// row per run so that runs can be listed without asking any product to list its own.
package runs

import (
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/SuperJackfruitLabs/superwitness/internal/config"
)

const (
	WriteScope   = "runs:write"    // the hub scope a reporter's token must carry
	MaxBatch     = 100             // reports in one {"runs": [...]} body
	MaxBodyBytes = 256 << 10       // one POST /v1/runs body
	FutureSkew   = 5 * time.Minute // how far ahead of superwitness's clock reported_at may be
)

var Statuses = []string{"queued", "running", "waiting", "succeeded", "failed", "cancelled"}

var Terminal = map[string]bool{"succeeded": true, "failed": true, "cancelled": true}

func ValidStatus(s string) bool { return slices.Contains(Statuses, s) }

var (
	sourceName = regexp.MustCompile(config.SourceNamePattern)
	executorID = regexp.MustCompile(`^prn_[0-9a-f]{20}$`)
	reportKeys = []string{"source", "external_ref", "scope", "title", "executor", "status", "source_status", "started_at", "ended_at", "reported_at"}
	namedKeys  = []string{"id", "name"}
	// instantShape is RFC 3339 date-time as the published schema asserts it. Go's parser also
	// takes a comma before the fraction and offsets such as +24:00, which the schema refuses.
	instantShape = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}[Tt]\d{2}:\d{2}:\d{2}(\.\d+)?([Zz]|[+-]([01]\d|2[0-3]):[0-5]\d)$`)
)

// Run is one registry row. FirstSeenAt and UpdatedAt are superwitness's own; LatestVerdict is
// filled by a list and never stored.
type Run struct {
	Source        string
	ExternalRef   string
	ScopeID       *string
	ScopeName     *string
	Title         *string
	Executor      *string // the executor's hub principal id, when the source knows it
	ExecutorName  *string
	Status        string
	SourceStatus  string
	StartedAt     *time.Time
	EndedAt       *time.Time
	ReportedAt    time.Time
	FirstSeenAt   time.Time
	UpdatedAt     time.Time
	LatestVerdict *VerdictSummary
}

// SubjectRef is the run as a verdict names it: <source>:<external_ref>.
func (r Run) SubjectRef() string { return r.Source + ":" + r.ExternalRef }

// SortAt is the list's sort key: when the run started, or when superwitness first heard of it.
func (r Run) SortAt() time.Time {
	if r.StartedAt != nil {
		return *r.StartedAt
	}
	return r.FirstSeenAt
}

func (r Run) MarshalJSON() ([]byte, error) {
	type named struct {
		ID   *string `json:"id"`
		Name *string `json:"name"`
	}
	var scope, executor *named
	if r.ScopeID != nil {
		scope = &named{r.ScopeID, r.ScopeName}
	}
	if r.Executor != nil || r.ExecutorName != nil {
		executor = &named{r.Executor, r.ExecutorName}
	}
	return json.Marshal(struct {
		Source        string          `json:"source"`
		ExternalRef   string          `json:"external_ref"`
		Ref           string          `json:"ref"`
		Scope         *named          `json:"scope"`
		Title         *string         `json:"title"`
		Executor      *named          `json:"executor"`
		Status        string          `json:"status"`
		SourceStatus  string          `json:"source_status"`
		StartedAt     *time.Time      `json:"started_at"`
		EndedAt       *time.Time      `json:"ended_at"`
		ReportedAt    time.Time       `json:"reported_at"`
		FirstSeenAt   time.Time       `json:"first_seen_at"`
		UpdatedAt     time.Time       `json:"updated_at"`
		LatestVerdict *VerdictSummary `json:"latest_verdict"`
	}{r.Source, r.ExternalRef, r.SubjectRef(), scope, r.Title, executor, r.Status, r.SourceStatus,
		r.StartedAt, r.EndedAt, r.ReportedAt, r.FirstSeenAt, r.UpdatedAt, r.LatestVerdict})
}

// BodyError is a body that is not one report or {"runs": [...]} of 1 to MaxBatch: 400.
type BodyError struct{ Msg string }

func (e *BodyError) Error() string { return e.Msg }

// ValidationError refuses one report: 422. Index is its position in a batch, -1 for a single report.
type ValidationError struct {
	Index int
	Field string
	Msg   string
}

func (e *ValidationError) Error() string {
	var b strings.Builder
	if e.Index >= 0 {
		fmt.Fprintf(&b, "runs[%d]", e.Index)
	}
	if e.Field != "" {
		if b.Len() > 0 {
			b.WriteByte('.')
		}
		b.WriteString(e.Field)
	}
	if b.Len() > 0 {
		b.WriteString(": ")
	}
	b.WriteString(e.Msg)
	return b.String()
}

// ParseBody decodes one report or {"runs": [...]} and validates every report as a whole: the
// first bad report fails the body. now bounds reported_at (FutureSkew).
func ParseBody(body []byte, now time.Time) ([]Run, bool, error) {
	var top map[string]json.RawMessage
	if err := json.Unmarshal(body, &top); err != nil || top == nil {
		return nil, false, &BodyError{`the body must be one run report or {"runs": [...]}`}
	}
	raw, isBatch := top["runs"]
	if !isBatch {
		r, verr := parseReport(body, now)
		if verr != nil {
			verr.Index = -1
			return nil, false, verr
		}
		return []Run{r}, false, nil
	}
	if len(top) != 1 {
		return nil, true, &BodyError{`a batch is {"runs": [...]} with no other key`}
	}
	var items []json.RawMessage
	if err := json.Unmarshal(raw, &items); err != nil || items == nil {
		return nil, true, &BodyError{"runs must be an array of reports"}
	}
	if len(items) == 0 || len(items) > MaxBatch {
		return nil, true, &BodyError{fmt.Sprintf("runs holds 1 to %d reports, got %d", MaxBatch, len(items))}
	}
	out := make([]Run, 0, len(items))
	for i, it := range items {
		r, verr := parseReport(it, now)
		if verr != nil {
			verr.Index = i
			return nil, true, verr
		}
		out = append(out, r)
	}
	return out, true, nil
}

func parseReport(raw json.RawMessage, now time.Time) (Run, *ValidationError) {
	o, verr := object(raw, "", reportKeys)
	if verr != nil {
		return Run{}, verr
	}
	var r Run
	src, verr := o.text("source", true, 1, 32)
	if verr != nil {
		return Run{}, verr
	}
	if !sourceName.MatchString(*src) {
		return Run{}, invalid("source", "lowercase letters, digits and -, starting with a letter, at most 32; got %q", *src)
	}
	r.Source = *src
	ref, verr := o.text("external_ref", true, 1, 256)
	if verr != nil {
		return Run{}, verr
	}
	r.ExternalRef = *ref
	if raw, ok := o.m["scope"]; ok {
		s, verr := object(raw, "scope", namedKeys)
		if verr != nil {
			return Run{}, verr
		}
		if r.ScopeID, verr = s.text("id", true, 1, 128); verr != nil {
			return Run{}, verr
		}
		if r.ScopeName, verr = s.text("name", false, 1, 200); verr != nil {
			return Run{}, verr
		}
	}
	if r.Title, verr = o.text("title", false, 1, 200); verr != nil {
		return Run{}, verr
	}
	if raw, ok := o.m["executor"]; ok {
		e, verr := object(raw, "executor", namedKeys)
		if verr != nil {
			return Run{}, verr
		}
		if r.ExecutorName, verr = e.text("name", true, 1, 200); verr != nil {
			return Run{}, verr
		}
		if r.Executor, verr = e.text("id", false, 1, 24); verr != nil {
			return Run{}, verr
		}
		if r.Executor != nil && !executorID.MatchString(*r.Executor) {
			return Run{}, invalid("executor.id", "a hub principal id, prn_ and 20 lowercase hex digits; got %q", *r.Executor)
		}
	}
	st, verr := o.text("status", true, 1, 16)
	if verr != nil {
		return Run{}, verr
	}
	if !ValidStatus(*st) {
		return Run{}, invalid("status", "one of queued, running, waiting, succeeded, failed, cancelled; got %q", *st)
	}
	r.Status = *st
	ss, verr := o.text("source_status", true, 1, 64)
	if verr != nil {
		return Run{}, verr
	}
	r.SourceStatus = *ss
	if r.StartedAt, verr = o.instant("started_at", false, true); verr != nil {
		return Run{}, verr
	}
	if r.EndedAt, verr = o.instant("ended_at", false, true); verr != nil {
		return Run{}, verr
	}
	at, verr := o.instant("reported_at", true, false)
	if verr != nil {
		return Run{}, verr
	}
	if at.After(now.Add(FutureSkew)) {
		return Run{}, invalid("reported_at", "%s is more than %s ahead of superwitness's clock", at.Format(time.RFC3339Nano), FutureSkew)
	}
	r.ReportedAt = *at
	return r, nil
}

func invalid(field, format string, args ...any) *ValidationError {
	return &ValidationError{Field: field, Msg: fmt.Sprintf(format, args...)}
}

// obj is one JSON object of a report, with its path for messages.
type obj struct {
	path string
	m    map[string]json.RawMessage
}

// object reads raw as a JSON object whose keys are all in keys, spelled exactly: encoding/json
// alone would match "Source" to source, which the published schema refuses.
func object(raw json.RawMessage, path string, keys []string) (obj, *ValidationError) {
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil || m == nil {
		return obj{}, invalid(path, "must be a JSON object")
	}
	for k := range m {
		if !slices.Contains(keys, k) {
			return obj{}, invalid(path, "unknown field %q", k)
		}
	}
	return obj{path, m}, nil
}

func (o obj) name(key string) string {
	if o.path == "" {
		return key
	}
	return o.path + "." + key
}

func isNull(raw json.RawMessage) bool { return bytes.Equal(bytes.TrimSpace(raw), []byte("null")) }

// text reads a string of minN to maxN characters (code points, as JSON Schema counts them). It
// is nil when the key is absent; null is never a string. NUL is refused because Postgres text
// cannot hold it.
func (o obj) text(key string, required bool, minN, maxN int) (*string, *ValidationError) {
	raw, ok := o.m[key]
	if !ok {
		if required {
			return nil, invalid(o.name(key), "required")
		}
		return nil, nil
	}
	var s string
	if isNull(raw) || json.Unmarshal(raw, &s) != nil {
		return nil, invalid(o.name(key), "must be a string; leave the field out rather than send null")
	}
	if n := utf8.RuneCountInString(s); n < minN || n > maxN {
		return nil, invalid(o.name(key), "%d to %d characters, got %d", minN, maxN, n)
	}
	if strings.ContainsRune(s, 0) {
		return nil, invalid(o.name(key), "must not contain NUL")
	}
	return &s, nil
}

// instant reads an RFC 3339 timestamp, truncated to the microsecond, which is all Postgres keeps.
func (o obj) instant(key string, required, nullable bool) (*time.Time, *ValidationError) {
	raw, ok := o.m[key]
	if !ok {
		if required {
			return nil, invalid(o.name(key), "required")
		}
		return nil, nil
	}
	if isNull(raw) && nullable {
		return nil, nil
	}
	var s string
	if isNull(raw) || json.Unmarshal(raw, &s) != nil || utf8.RuneCountInString(s) > 64 {
		return nil, invalid(o.name(key), "must be an RFC 3339 timestamp string")
	}
	// RFC 3339 allows a lowercase t and z; Go's parser wants them uppercase.
	t, err := time.Parse(time.RFC3339Nano, strings.ToUpper(s))
	if err != nil || !instantShape.MatchString(s) {
		return nil, invalid(o.name(key), "an RFC 3339 timestamp such as 2026-10-06T10:00:00Z; got %q", s)
	}
	t = t.UTC().Truncate(time.Microsecond)
	// A zone offset can push the instant outside the years JSON can encode; stored, it would
	// make every listing fail.
	if y := t.Year(); y < 0 || y > 9999 {
		return nil, invalid(o.name(key), "the year in UTC must be 0 to 9999; got %q", s)
	}
	return &t, nil
}
