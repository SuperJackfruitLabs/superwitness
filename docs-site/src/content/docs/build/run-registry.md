---
title: Run registry API
description: How a source reports its runs to superwitness, and how runs are listed back.
---

superwitness keeps its own list of runs. Each product that runs work, a **source**, reports its
runs to superwitness as they start, change and finish, the same way it sends telemetry to an
OTLP endpoint. superwitness never asks a product to list its runs. A source that has not
reported a run leaves it out of the list, but the run's own page still works by its link.

## Reporting runs

`POST /v1/runs` takes one report, or a batch of 1 to 100:

```json
{ "source": "superpipeline", "external_ref": "brd_01/run_01",
  "scope": {"id": "brd_01", "name": "Press"}, "title": "Draft the release note",
  "executor": {"id": "prn_…", "name": "drafter"},
  "status": "running", "source_status": "in_progress",
  "started_at": "2026-10-06T09:00:00Z", "ended_at": null, "reported_at": "2026-10-06T09:00:01Z" }
```

```json
{ "runs": [ { … }, { … } ] }
```

| Field | Required | Rule |
|---|---|---|
| `source` | yes | lowercase letters, digits and `-`, starting with a letter, at most 32 |
| `external_ref` | yes | the source's own reference for the run, 1 to 256 characters |
| `scope` | no | `{"id", "name"}`: where the run belongs, such as a board. `id` is required, 1 to 128 characters; `name`, 1 to 200, may be left out. Leave `scope` out rather than send `null` |
| `title` | no | 1 to 200 characters. Leave it out rather than send `null` or `""` |
| `executor` | no | `{"id", "name"}`: who ran it. `name` is required, 1 to 200 characters; `id`, when known, is a hub principal id (`prn_` and 20 lowercase hex digits). Leave `executor` out rather than send `null` |
| `status` | yes | `queued`, `running`, `waiting`, `succeeded`, `failed` or `cancelled` |
| `source_status` | yes | the source's own word for the state, 1 to 64 characters |
| `started_at`, `ended_at` | no | RFC 3339 timestamps, or `null` when not yet known |
| `reported_at` | yes | RFC 3339: when the source produced this report |

Unknown fields are refused, and field names are matched exactly. Timestamps are RFC 3339
`date-time` strings of at most 64 characters, kept to the microsecond. The format is strict: the fraction separator is a dot, not a comma, and an offset hour is at most 23. The body may be at most
256 KiB.

### Who may report

A reporter is an AgentPod **service** principal whose token carries the `runs:write` scope and
superwitness's audience (`SW_PUBLIC_URL`). superwitness also binds each reporter to one source
with `SW_RUN_SOURCES`, for example `SW_RUN_SOURCES=prn_reporter01=superpipeline`; a report for
any other source is refused. Session cookies are never accepted here.

### Newer reports win

A report updates its run only when its `reported_at` is **strictly newer** than the one stored.
An equal or older report changes nothing and answers `"applied": false`, so retries and
out-of-order deliveries cannot move a run backwards. A `reported_at` more than 5 minutes ahead
of superwitness's clock is refused, so that one report from a fast clock cannot freeze a run.

### The answer

```json
{"results": [{"external_ref": "brd_01/run_01", "applied": true}]}
```

One result per report, in the order sent. A batch is checked as a whole: if one report is
invalid, nothing is written, and the error names its position with `index`. A single report's errors carry no `index`; a batch from a reporter that is not bound to the source gets `index` 0:

```json
{"error": {"code": "invalid_report", "message": "runs[3].status: one of queued, …", "index": 3}}
```

| Status | Code | When | Retry? |
|---|---|---|---|
| 400 | `invalid_json` | the body is not one report or `{"runs": [...]}` of 1 to 100 | no |
| 401 | `unauthenticated` | no token, or one not valid for this service | no |
| 403 | `service_principal_required` | the token's principal is not a service | no |
| 403 | `insufficient_scope` | the token lacks `runs:write` | no |
| 403 | `source_not_allowed` | the reporter is not bound to this report's source | no |
| 413 | `body_too_large` | the body is over 256 KiB | no |
| 422 | `invalid_report` | a report breaks a rule above | no |
| 503 | `store_unavailable` | superwitness's database is unavailable; `Retry-After` is set | yes |

A reporter should retry 5xx answers and network errors with backoff, and set any other 4xx
aside: sending it again will get the same answer.

### The schema

The body is described by a JSON Schema (draft 2020-12), published at
[`/schemas/run-report.schema.json`](/schemas/run-report.schema.json) and kept in the
repository at `internal/contracts/run-report.schema.json`. superwitness's own tests run the same
bodies through the schema and through its handler and require the same answer from both.

superwitness keeps four rules beyond the schema: `reported_at` may be at most 5 minutes ahead
of its clock, no string may contain NUL (`\u0000`), a timestamp may not be a leap second, and a
timestamp's year in UTC must be 0 to 9999.

## Listing runs

`GET /v1/runs` lists runs newest first: by `started_at`, or by when superwitness first heard of
the run when it has not started, then by `source` and `external_ref`.

| Query | Meaning |
|---|---|
| `source` | only runs from this source |
| `scope` | only runs in this scope id |
| `status` | only runs with this status; repeat it for several |
| `executor` | only runs this principal executed |
| `since`, `until` | RFC 3339; runs whose start (or first sighting) is at or after `since` and before `until` |
| `needs_verdict` | `true`: finished runs (`succeeded`, `failed`, `cancelled`) that no verdict names yet |
| `cursor` | `next_cursor` from the previous page |
| `limit` | page size, 1 or more (0 or less is 400 `invalid_limit`); default 50, and anything above 200 is 200 |

```json
{
  "runs": [
    { "source": "superpipeline", "external_ref": "brd_01/run_01", "ref": "superpipeline:brd_01/run_01",
      "scope": {"id": "brd_01", "name": "Press"}, "title": "Draft the release note",
      "executor": {"id": "prn_…", "name": "drafter"}, "status": "succeeded", "source_status": "done",
      "started_at": "2026-10-06T09:00:00Z", "ended_at": "2026-10-06T09:05:00Z",
      "reported_at": "2026-10-06T09:05:01Z", "first_seen_at": "2026-10-06T09:00:02Z", "updated_at": "2026-10-06T09:05:02Z",
      "latest_verdict": { "id": "vrd_01", "judge": "prn_human01", "judge_kind": "human",
        "standard": "rubric:press@1", "value": {"decision": "pass"}, "created_at": "2026-10-06T09:30:00Z" } }
  ],
  "next_cursor": null,
  "counts": {"queued": 0, "running": 2, "waiting": 1, "succeeded": 14, "failed": 3, "cancelled": 0}
}
```

- `ref` is the run as a verdict names it, `<source>:<external_ref>`: record a verdict with
  `"subject_kind": "run"` and this as `subject_ref`.
- `latest_verdict` is the newest verdict on the run that no other verdict supersedes, or `null`.
  Verdicts on the run's attempts do not count.
- `counts` totals every status for the same filter without `status`, so a view can show its tabs.
- `next_cursor` is `null` on the last page. A cursor stays valid while new runs arrive: the next
  page continues where the last one ended.

Errors: 400 `invalid_source`, `invalid_status`, `invalid_time`, `invalid_cursor`, `invalid_limit`
or `invalid_needs_verdict`, and 503 `store_unavailable`. Any principal whose token carries
superwitness's audience may list runs, as with every other read.

The MCP tool `list_runs` takes the same filters as arguments (`status` as a list) and answers the
same body ([MCP tools](/build/mcp/)).
