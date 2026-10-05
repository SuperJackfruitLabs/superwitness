// Package source is one of superwitness's two fixed points: every evidence
// system is an adapter behind Source, and a later run index can slot in behind join
// without changing this interface.
package source

import (
	"context"
	"time"
)

type Name string

const (
	Superpipeline Name = "superpipeline"
	AgentPod      Name = "agentpod"
	Traces        Name = "traces"
	Logs          Name = "logs"
	Errors        Name = "errors"
)

type SourceStatus string

const (
	StatusOK           SourceStatus = "ok"
	StatusUnavailable  SourceStatus = "unavailable"
	StatusTimeout      SourceStatus = "timeout"
	StatusNotFound     SourceStatus = "not_found"
	StatusUnauthorized SourceStatus = "unauthorized"
)

// Source fetches one system's view of a run. It never returns an error: every failure
// is a status, so join can render the rest of the document.
type Source interface {
	Name() Name
	Fetch(ctx context.Context, ref RunRef) (Fragment, SourceStatus)
}

type Pinger interface {
	Ping(ctx context.Context) SourceStatus
}

type SpanLister interface {
	ListSpans(ctx context.Context, ref RunRef) ([]Span, SourceStatus)
}

type LogQuery struct {
	Level  string // "", debug, info, warn, error
	Offset int
	Limit  int
}

type LogLister interface {
	ListLogs(ctx context.Context, ref RunRef, q LogQuery) ([]LogLine, SourceStatus)
}

type AttemptResolver interface {
	ResolveAttempt(ctx context.Context, attemptID string) (AttemptLink, SourceStatus)
}

// Fragment carries exactly one of the typed payloads, plus when and at what version it was read.
type Fragment struct {
	Source    Name
	FetchedAt time.Time
	Version   string // the source's own as_of / sequence, when it has one

	Run    *RunFragment
	Ledger *LedgerFragment
	Traces *TracesFragment
	Logs   *LogsFragment
	Errors *ErrorsFragment
}
