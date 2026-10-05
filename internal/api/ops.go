package api

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/SuperJackfruitLabs/superwitness/internal/join"
	"github.com/SuperJackfruitLabs/superwitness/internal/runs"
	"github.com/SuperJackfruitLabs/superwitness/internal/source"
	"github.com/SuperJackfruitLabs/superwitness/internal/verdicts"
)

const (
	DefaultPageSize = 100
	MaxPageSize     = 500
)

type Ops struct {
	Join       *join.Joiner
	Spans      source.SpanLister
	Logs       source.LogLister
	Attempts   source.AttemptResolver
	Verdicts   *verdicts.Service
	Timeout    time.Duration
	Runs       runs.Store
	RunSources map[string]string // SW_RUN_SOURCES: reporting principal -> its one source
	Rubrics    verdicts.RubricReader
	Now        func() time.Time
}

func (o *Ops) now() time.Time {
	if o.Now != nil {
		return o.Now()
	}
	return time.Now()
}

type SpanPage struct {
	Spans      []source.Span `json:"spans"`
	NextCursor string        `json:"next_cursor,omitempty"`
}

type LogPage struct {
	Logs       []source.LogLine    `json:"logs"`
	NextCursor *string             `json:"next_cursor"` // null on the last page, never omitted
	TraceJoin  source.SourceStatus `json:"trace_join"`  // whether lines were matched by trace id as well as run.id
}

func (o *Ops) timeout() time.Duration {
	if o.Timeout > 0 {
		return o.Timeout
	}
	return join.DefaultTimeout
}

func normLimit(n int) (int, error) {
	switch {
	case n < 0:
		return 0, badRequest("invalid_limit", "limit must be a positive integer")
	case n == 0:
		return DefaultPageSize, nil
	case n > MaxPageSize:
		return MaxPageSize, nil
	}
	return n, nil
}

func (o *Ops) GetRun(ctx context.Context, ref source.RunRef) (join.RunDocument, error) {
	doc, err := o.Join.Build(ctx, ref)
	if errors.Is(err, join.ErrRunNotFound) {
		return join.RunDocument{}, &APIError{Status: http.StatusNotFound, Code: "run_not_found",
			Message: "neither superpipeline nor the hub ledger knows " + ref.String()}
	}
	return doc, err
}

func (o *Ops) ListSpans(ctx context.Context, ref source.RunRef, cur string, limit int) (SpanPage, error) {
	off, err := decodeCursor(cur)
	if err != nil {
		return SpanPage{}, badRequest("invalid_cursor", err.Error())
	}
	n, err := normLimit(limit)
	if err != nil {
		return SpanPage{}, err
	}
	cctx, cancel := context.WithTimeout(ctx, o.timeout())
	defer cancel()
	spans, st := o.Spans.ListSpans(cctx, ref)
	if st != source.StatusOK {
		return SpanPage{}, sourceError(source.Traces, st)
	}
	page := SpanPage{Spans: []source.Span{}}
	if off < len(spans) {
		end := min(off+n, len(spans))
		page.Spans = spans[off:end]
		if end < len(spans) {
			page.NextCursor = encodeCursor(end)
		}
	}
	return page, nil
}

func (o *Ops) ListLogs(ctx context.Context, ref source.RunRef, cur, level string, limit int) (LogPage, error) {
	switch level {
	case "", "debug", "info", "warn", "error":
	default:
		return LogPage{}, badRequest("invalid_level", "level must be one of debug, info, warn, error")
	}
	off, err := decodeCursor(cur)
	if err != nil {
		return LogPage{}, badRequest("invalid_cursor", err.Error())
	}
	n, err := normLimit(limit)
	if err != nil {
		return LogPage{}, err
	}

	tctx, tcancel := context.WithTimeout(ctx, o.timeout())
	spans, tst := o.Spans.ListSpans(tctx, ref)
	tcancel()
	var ids []string
	if tst == source.StatusOK {
		ids = source.TraceIDsOf(spans)
	}

	lctx, lcancel := context.WithTimeout(ctx, o.timeout())
	defer lcancel()
	lines, st := o.Logs.ListLogs(lctx, ref.WithTraceIDs(ids), source.LogQuery{Level: level, Offset: off, Limit: n + 1})
	if st != source.StatusOK {
		return LogPage{}, sourceError(source.Logs, st)
	}
	page := LogPage{Logs: lines, TraceJoin: tst}
	if len(lines) > n {
		next := encodeCursor(off + n)
		page.Logs, page.NextCursor = lines[:n], &next
	}
	if page.Logs == nil {
		page.Logs = []source.LogLine{}
	}
	return page, nil
}

func (o *Ops) ResolveAttempt(ctx context.Context, attemptID string) (source.RunRef, error) {
	if !source.ValidAttemptID(attemptID) {
		return source.RunRef{}, badRequest("invalid_attempt_id", "an attempt id is attempt_<id>")
	}
	cctx, cancel := context.WithTimeout(ctx, o.timeout())
	defer cancel()
	link, st := o.Attempts.ResolveAttempt(cctx, attemptID)
	switch st {
	case source.StatusOK:
	case source.StatusNotFound:
		return source.RunRef{}, &APIError{Status: http.StatusNotFound, Code: "attempt_not_found", Message: "the hub knows no " + attemptID}
	default:
		return source.RunRef{}, sourceError(source.AgentPod, st)
	}
	ref, ok := link.Ref()
	if !ok {
		return source.RunRef{}, &APIError{Status: http.StatusNotFound, Code: "attempt_has_no_run",
			Message: attemptID + " was not dispatched from a superpipeline run"}
	}
	return ref, nil
}
