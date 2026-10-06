// Package mcp exposes the api operations as MCP tools: get_run, list_run_spans,
// list_run_logs, list_runs, record_verdict and get_transcript. The by-attempt lookup and the
// transcript item route are HTTP-only.
package mcp

import (
	"context"
	"encoding/json"
	"net/http"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/SuperJackfruitLabs/superwitness/internal/api"
	"github.com/SuperJackfruitLabs/superwitness/internal/auth"
	"github.com/SuperJackfruitLabs/superwitness/internal/source"
	"github.com/SuperJackfruitLabs/superwitness/internal/verdicts"
)

type RunArgs struct {
	Source  string `json:"source,omitempty" jsonschema:"the run's source system; this release supports only superpipeline (the default)"`
	BoardID string `json:"board_id" jsonschema:"superpipeline board id, brd_…"`
	RunID   string `json:"run_id" jsonschema:"superpipeline run id, run_…"`
}

type SpansArgs struct {
	Source  string `json:"source,omitempty" jsonschema:"superpipeline (the default)"`
	BoardID string `json:"board_id" jsonschema:"superpipeline board id, brd_…"`
	RunID   string `json:"run_id" jsonschema:"superpipeline run id, run_…"`
	Cursor  string `json:"cursor,omitempty" jsonschema:"next_cursor from the previous page"`
	Limit   int    `json:"limit,omitempty" jsonschema:"page size from 1 to 500; default 100"`
}

// ListRunsArgs are GET /v1/runs's query parameters.
type ListRunsArgs struct {
	Source       string   `json:"source,omitempty" jsonschema:"only runs from this source, e.g. superpipeline"`
	Scope        string   `json:"scope,omitempty" jsonschema:"only runs in this scope, e.g. a board id"`
	Status       []string `json:"status,omitempty" jsonschema:"any of queued, running, waiting, succeeded, failed, cancelled"`
	Executor     string   `json:"executor,omitempty" jsonschema:"only runs this principal executed (prn_…)"`
	Since        string   `json:"since,omitempty" jsonschema:"RFC 3339: runs that started, or were first seen, at or after this"`
	Until        string   `json:"until,omitempty" jsonschema:"RFC 3339: runs that started, or were first seen, before this"`
	NeedsVerdict bool     `json:"needs_verdict,omitempty" jsonschema:"only finished runs that no verdict names yet"`
	Cursor       string   `json:"cursor,omitempty" jsonschema:"next_cursor from the previous page"`
	Limit        int      `json:"limit,omitempty" jsonschema:"page size from 1 to 200; default 50"`
}

type LogsArgs struct {
	Source  string `json:"source,omitempty" jsonschema:"superpipeline (the default)"`
	BoardID string `json:"board_id" jsonschema:"superpipeline board id, brd_…"`
	RunID   string `json:"run_id" jsonschema:"superpipeline run id, run_…"`
	Cursor  string `json:"cursor,omitempty" jsonschema:"next_cursor from the previous page"`
	Level   string `json:"level,omitempty" jsonschema:"debug, info, warn or error; empty for all"`
	Limit   int    `json:"limit,omitempty" jsonschema:"page size from 1 to 500; default 100"`
}

// TranscriptArgs are the transcript route's parameters. An MCP caller is never a browser session,
// so it needs a token granted transcripts:read.
type TranscriptArgs struct {
	BoardID   string `json:"board_id" jsonschema:"superpipeline board id, brd_…"`
	RunID     string `json:"run_id" jsonschema:"superpipeline run id, run_…"`
	AttemptID string `json:"attempt_id,omitempty" jsonschema:"the attempt to read, attempt_…; required when the run has more than one"`
	SeqFrom   *int64 `json:"seq_from,omitempty" jsonschema:"first session seq to read; default the attempt's first"`
	SeqTo     *int64 `json:"seq_to,omitempty" jsonschema:"last session seq to read; default the attempt's last"`
	Cursor    string `json:"cursor,omitempty" jsonschema:"next_cursor from the previous page; send the same seq_from and seq_to with it"`
}

// VerdictArgs is the POST /v1/verdicts body. The judge is always the caller.
type VerdictArgs struct {
	IdempotencyKey string         `json:"idempotency_key" jsonschema:"a key you choose; a retry with the same key returns the original verdict"`
	Kind           string         `json:"kind,omitempty" jsonschema:"grader, review, eval or calibration; defaults to review for humans and grader otherwise"`
	SubjectKind    string         `json:"subject_kind" jsonschema:"run, attempt or eval_case_run"`
	SubjectRef     string         `json:"subject_ref" jsonschema:"superpipeline:<board>/<run>, attempt_<id>, or case:<id>@sha256:<fingerprint>"`
	Standard       string         `json:"standard" jsonschema:"rubric:<id>@<version>, stage:<key> or case:<id>"`
	Value          map[string]any `json:"value" jsonschema:"exactly one of {decision}, {score 0..1}, {label}, {text}"`
	Comment        string         `json:"comment,omitempty" jsonschema:"free text, at most 10000 characters"`
	EvidenceRefs   []any          `json:"evidence_refs,omitempty" jsonschema:"span ids or {session_id, seq_from, seq_to}"`
	Supersedes     string         `json:"supersedes,omitempty" jsonschema:"the id of your earlier verdict this one corrects"`
}

func NewHandler(ops *api.Ops, version string) http.Handler {
	return sdk.NewStreamableHTTPHandler(func(r *http.Request) *sdk.Server {
		caller, _ := auth.PrincipalFrom(r.Context())
		return NewServer(ops, caller, version)
	}, &sdk.StreamableHTTPOptions{Stateless: true})
}

func NewServer(ops *api.Ops, caller auth.Principal, version string) *sdk.Server {
	s := sdk.NewServer(&sdk.Implementation{Name: "superwitness", Version: version}, nil)

	sdk.AddTool(s, &sdk.Tool{Name: "get_run",
		Description: "The RunDocument for one work run: attempts with fingerprints, joined traces, errors, gate and recorded verdicts, cost, and per-source status."},
		func(ctx context.Context, _ *sdk.CallToolRequest, in RunArgs) (*sdk.CallToolResult, any, error) {
			ref, err := runRef(in.Source, in.BoardID, in.RunID)
			if err != nil {
				return toolError(err), nil, nil
			}
			doc, err := ops.GetRun(ctx, ref)
			if err != nil {
				return toolError(err), nil, nil
			}
			return toolJSON(doc), nil, nil
		})

	sdk.AddTool(s, &sdk.Tool{Name: "list_run_spans", Description: "One page of a run's spans, oldest first."},
		func(ctx context.Context, _ *sdk.CallToolRequest, in SpansArgs) (*sdk.CallToolResult, any, error) {
			ref, err := runRef(in.Source, in.BoardID, in.RunID)
			if err != nil {
				return toolError(err), nil, nil
			}
			page, err := ops.ListSpans(ctx, ref, in.Cursor, in.Limit)
			if err != nil {
				return toolError(err), nil, nil
			}
			return toolJSON(page), nil, nil
		})

	sdk.AddTool(s, &sdk.Tool{Name: "list_run_logs", Description: "One page of a run's log lines, matched by run.id and by the run's trace ids, oldest first."},
		func(ctx context.Context, _ *sdk.CallToolRequest, in LogsArgs) (*sdk.CallToolResult, any, error) {
			ref, err := runRef(in.Source, in.BoardID, in.RunID)
			if err != nil {
				return toolError(err), nil, nil
			}
			page, err := ops.ListLogs(ctx, ref, in.Cursor, in.Level, in.Limit)
			if err != nil {
				return toolError(err), nil, nil
			}
			return toolJSON(page), nil, nil
		})

	sdk.AddTool(s, &sdk.Tool{Name: "list_runs",
		Description: "Runs reported to superwitness's run registry, newest first, with per-status counts and each run's latest verdict. Filter by source, scope, status, executor, time, or needs_verdict."},
		func(ctx context.Context, _ *sdk.CallToolRequest, in ListRunsArgs) (*sdk.CallToolResult, any, error) {
			page, err := ops.ListRuns(ctx, api.RunQuery{Source: in.Source, Scope: in.Scope, Status: in.Status,
				Executor: in.Executor, Since: in.Since, Until: in.Until, NeedsVerdict: in.NeedsVerdict,
				Cursor: in.Cursor, Limit: in.Limit})
			if err != nil {
				return toolError(err), nil, nil
			}
			return toolJSON(page), nil, nil
		})

	sdk.AddTool(s, &sdk.Tool{Name: "record_verdict",
		Description: "Record a verdict as the calling principal. Append-only; to correct one, record a new verdict that supersedes it. An agent may not judge a run it executed."},
		func(ctx context.Context, _ *sdk.CallToolRequest, in VerdictArgs) (*sdk.CallToolResult, any, error) {
			req, err := in.request()
			if err != nil {
				return toolError(err), nil, nil
			}
			v, _, err := ops.RecordVerdict(ctx, caller, req)
			if err != nil {
				return toolError(err), nil, nil
			}
			return toolJSON(v), nil, nil
		})

	sdk.AddTool(s, &sdk.Tool{Name: "get_transcript",
		Description: "One page (up to 200 items) of an attempt's session transcript: prompts, messages, reasoning, tool calls with input and output, permissions, state and errors, redacted by the hub. Needs a token granted transcripts:read."},
		func(ctx context.Context, _ *sdk.CallToolRequest, in TranscriptArgs) (*sdk.CallToolResult, any, error) {
			ref, err := runRef("", in.BoardID, in.RunID)
			if err != nil {
				return toolError(err), nil, nil
			}
			body, err := ops.GetTranscript(ctx, api.Caller{Principal: caller}, api.TranscriptRequest{Ref: ref,
				AttemptID: in.AttemptID, SeqFrom: in.SeqFrom, SeqTo: in.SeqTo, Cursor: in.Cursor})
			if err != nil {
				return toolError(err), nil, nil
			}
			return toolJSON(body), nil, nil
		})
	return s
}

func runRef(src, board, run string) (source.RunRef, error) {
	if src != "" && src != string(source.Superpipeline) {
		return source.RunRef{}, &api.APIError{Status: 400, Code: "invalid_source", Message: "source must be superpipeline in this release"}
	}
	ref, err := source.NewSuperpipelineRef(board, run)
	if err != nil {
		return source.RunRef{}, &api.APIError{Status: 400, Code: "invalid_run_ref", Message: err.Error()}
	}
	return ref, nil
}

func (a VerdictArgs) request() (verdicts.Request, error) {
	value, err := json.Marshal(a.Value)
	if err != nil {
		return verdicts.Request{}, &api.APIError{Status: 400, Code: "invalid_value", Message: err.Error()}
	}
	req := verdicts.Request{IdempotencyKey: a.IdempotencyKey, Kind: a.Kind, SubjectKind: verdicts.SubjectKind(a.SubjectKind),
		SubjectRef: a.SubjectRef, Standard: a.Standard, Value: value, Comment: a.Comment}
	if a.EvidenceRefs != nil {
		if req.EvidenceRefs, err = json.Marshal(a.EvidenceRefs); err != nil {
			return verdicts.Request{}, &api.APIError{Status: 400, Code: "invalid_evidence_refs", Message: err.Error()}
		}
	}
	if a.Supersedes != "" {
		sup := a.Supersedes
		req.Supersedes = &sup
	}
	return req, nil
}

func toolJSON(v any) *sdk.CallToolResult {
	b, err := json.Marshal(v)
	if err != nil {
		return toolError(err)
	}
	return &sdk.CallToolResult{Content: []sdk.Content{&sdk.TextContent{Text: string(b)}}, StructuredContent: json.RawMessage(b)}
}

func toolError(err error) *sdk.CallToolResult {
	b, _ := json.Marshal(map[string]any{"error": api.AsAPIError(err)})
	return &sdk.CallToolResult{IsError: true, Content: []sdk.Content{&sdk.TextContent{Text: string(b)}}}
}
