---
title: MCP tools
description: The MCP server for agents, with tools for runs, spans, logs, transcripts, the run registry and verdicts; the by-attempt lookup stays HTTP-only.
---

superwitness serves an MCP server at `/mcp` on its own address, over MCP's streamable HTTP
transport. It is stateless: every request stands alone, with no session to open or keep. It
takes the same bearer token as the [HTTP API](/build/api/#authentication), and every tool acts
as the principal that token names.

Point an MCP client at it with the token as a header, for example:

```json
{
  "url": "https://superwitness.example.internal/mcp",
  "headers": { "Authorization": "Bearer <token>" }
}
```

The server names itself `superwitness`, with the running release as its version. It has six
tools, thin layers over the HTTP operations for runs, spans, logs, transcripts, the run registry
and verdicts. Five routes are HTTP-only and have no tool: `GET /v1/runs/by-attempt/{attempt}`,
`POST /v1/runs`, the two rubric reads and the transcript step route.

## The tools

### `get_run`

The RunDocument for one work run: attempts with fingerprints, joined traces, errors, gate and
recorded verdicts, cost, and per-source status. The same as
`GET /v1/runs/superpipeline/{board}/{run}`.

| Argument | Required | Description |
|---|---|---|
| `source` | no | the run's source system; only `superpipeline`, which is the default |
| `board_id` | yes | superpipeline board id, `brd_…` |
| `run_id` | yes | superpipeline run id, `run_…` |

Result: the [run document](/build/contracts/#the-run-document).

### `list_run_spans`

One page of a run's spans, oldest first. The same as
`GET /v1/runs/superpipeline/{board}/{run}/spans`.

| Argument | Required | Description |
|---|---|---|
| `source` | no | `superpipeline`, the default |
| `board_id` | yes | superpipeline board id, `brd_…` |
| `run_id` | yes | superpipeline run id, `run_…` |
| `cursor` | no | `next_cursor` from the previous page |
| `limit` | no | page size from 1 to 500; default 100 |

Result: `{"spans": [...], "next_cursor": "…"}`, with `next_cursor` left out on the last page.

### `list_run_logs`

One page of a run's log lines, matched by `run.id` and by the run's trace ids, oldest first. The
same as `GET /v1/runs/superpipeline/{board}/{run}/logs`.

| Argument | Required | Description |
|---|---|---|
| `source` | no | `superpipeline`, the default |
| `board_id` | yes | superpipeline board id, `brd_…` |
| `run_id` | yes | superpipeline run id, `run_…` |
| `cursor` | no | `next_cursor` from the previous page |
| `level` | no | `debug`, `info`, `warn` or `error`; empty for all |
| `limit` | no | page size from 1 to 500; default 100 |

Result: `{"logs": [...], "next_cursor": "…" or null, "trace_join": "<status>"}`.

### `list_runs`

Runs reported to the run registry, newest first, with per-status counts and each run's latest
verdict. The same as `GET /v1/runs` ([Run registry API](/build/run-registry/#listing-runs)).

| Argument | Required | Description |
|---|---|---|
| `source` | no | only runs from this source |
| `scope` | no | only runs in this scope id |
| `status` | no | a list of statuses |
| `executor` | no | only runs this principal executed |
| `since`, `until` | no | RFC 3339 bounds on the run's start, or first sighting |
| `needs_verdict` | no | `true` for finished runs no verdict names yet |
| `cursor` | no | `next_cursor` from the previous page |
| `limit` | no | page size from 1 to 200; default 50 |

### `record_verdict`

Record a verdict as the calling principal. Append-only; to correct one, record a new verdict
that supersedes it. The same as `POST /v1/verdicts`; [Verdicts](/use/verdicts/) has the rules.

| Argument | Required | Description |
|---|---|---|
| `idempotency_key` | yes | a key you choose; a retry with the same key returns the original verdict |
| `kind` | no | `grader`, `review`, `eval` or `calibration`; defaults to `review` for humans and `grader` otherwise |
| `subject_kind` | yes | `run`, `attempt` or `eval_case_run` |
| `subject_ref` | yes | `superpipeline:<board>/<run>`, `attempt_<id>`, or `case:<id>@sha256:<fingerprint>` |
| `standard` | yes | `rubric:<id>@<version>`, `stage:<key>` or `case:<id>` |
| `value` | yes | exactly one of `{decision}`, `{score 0..1}`, `{label}`, `{text}` |
| `comment` | no | free text, at most 10000 characters |
| `evidence_refs` | no | span ids or `{session_id, seq_from, seq_to}` |
| `supersedes` | no | the id of your earlier verdict this one corrects |

Result: the [verdict](/build/contracts/#verdicts), whether it was just recorded or returned
for a retry. Unlike the HTTP body, the tool has no `judge` or `judge_kind` argument: the judge
is always the caller.

### `get_transcript`

One page (up to 200 steps) of an attempt's transcript, redacted by AgentPod. The same as
`GET /v1/runs/superpipeline/{board}/{run}/transcript`. The token must be granted
`transcripts:read`; an MCP call is never a browser session.

| Argument | Required | Description |
|---|---|---|
| `board_id` | yes | superpipeline board id, `brd_…` |
| `run_id` | yes | superpipeline run id, `run_…` |
| `attempt_id` | when the run has several attempts | the attempt to read, `attempt_…` |
| `seq_from` | no | first session seq; default the attempt's first |
| `seq_to` | no | last session seq; default the attempt's last |
| `cursor` | no | `next_cursor` from the previous page |

Result: the transcript page ([Transcripts](/use/transcripts/#the-api)).

## Results and errors

A tool's result is the same JSON the HTTP route returns, twice over: as text content, and as
structured content.

A refusal is a tool result marked as an error (`isError`), whose text content is the same error
object the HTTP API returns, with the same [codes](/build/api/#every-code):

```json
{"error":{"code":"run_not_found","message":"neither superpipeline nor the hub ledger knows superpipeline:brd_01/run_02"}}
```

There is one code the HTTP API has no use for: `invalid_source`, when `source` is anything but
`superpipeline`.

Each tool's input schema is published with it, with the descriptions above. Arguments are checked
against it before the tool runs: a missing required argument, or one the schema does not list, is
also an error result, whose text is a plain validation message rather than an error object.
