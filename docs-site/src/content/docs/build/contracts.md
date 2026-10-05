---
title: Contracts
description: The run document and verdict shapes superwitness returns, and the recorded payloads that pin what it reads from other products.
---

This page lists every field superwitness returns in a run document and a verdict, and the
payloads it reads from other products. [Concepts](/concepts/) explains what the fields mean;
this page is the exact shape.

Two rules hold throughout:

- **Nothing a source could not supply is shown as 0 or left out.** A text field reads
  `"unknown"`, a count reads `"unknown"`, and a list the source would have filled comes back
  empty, with `sources` saying why.
- **Do not rely on key order.** Objects keep the order shown here; the keys of `sources` and of
  span `attributes` come back sorted.

## The run document

What `GET /v1/runs/superpipeline/{board}/{run}` and the `get_run` tool return. Every top-level key
is always present:

| Key | Type |
|---|---|
| `run` | object, below |
| `attempts` | array, possibly empty |
| `trace` | object |
| `errors` | array, possibly empty |
| `verdicts` | array, possibly empty |
| `cost` | object |
| `log_count` | number, or `"unknown"` |
| `sources` | object: source name to status |
| `links` | object |

### `run`

| Field | Value |
|---|---|
| `ref` | `superpipeline:<board>/<run>` |
| `card` | `{"id", "title"}`, each a string or `"unknown"` |
| `stage` | the run's stage key, or `"unknown"` |
| `agent` | the executing agent's principal id (`prn_…`), or `"unknown"` |
| `agent_ref` | superpipeline's own id for the agent, or `"unknown"` |
| `state` | the run's outcome when superpipeline reports one, else its status; `"unknown"` when neither is known |
| `started_at` | RFC 3339 time in UTC, or `"unknown"` |
| `ended_at` | RFC 3339 time; `null` while the run has not ended; `"unknown"` when no source could say |

When superpipeline does not answer, `card.id`, `state`, `agent` and `started_at` come from the
AgentPod ledger where it has them: the card id, the dispatch outcome, the first attempt's agent
principal, and the earliest attempt's start.

### `attempts[]`

| Field | Value |
|---|---|
| `id` | `attempt_<id>` |
| `station` | the AgentPod station, or `"unknown"` |
| `state` | the attempt's state as AgentPod reports it, or `"unknown"` |
| `session_id` | the session the attempt ran in, or `"unknown"` |
| `seq_from` | number: the first event of the session the attempt covers |
| `seq_to` | number, or `null` while the attempt is open |
| `started_at` | RFC 3339 time, or `"unknown"` |
| `ended_at` | RFC 3339 time, or `null` |
| `fingerprint` | `digest`, `harness`, `harness_version`, `model`, `profile`, `skill_release`, `reported_by`: each a string or `"unknown"` |
| `span_count` | number of spans whose `attempt.id` is this attempt's id, or `"unknown"` when the traces source did not answer |

### `trace`

| Field | Value |
|---|---|
| `trace_ids` | array of trace ids, ordered by each trace's earliest span; empty when none were found or the traces source did not answer |
| `status` | `joined`, `partial`, `none` or `unknown` ([Concepts](/concepts/#trace-status)) |
| `sampled` | `true` when spans were found, otherwise `"unknown"` |

### `errors[]`

Up to 50 error-level log lines, oldest first:

| Field | Value |
|---|---|
| `service` | the service name, or `"unknown"` |
| `message` | the log message |
| `trace_id` | the line's trace id, or `""` |
| `at` | RFC 3339 time, or `"unknown"` |

### `verdicts[]`

Gates from superpipeline first, in superpipeline's order, then verdicts recorded in superwitness,
oldest first. Of each recorded chain, only the latest verdict appears.

| Field | Gate | Recorded verdict |
|---|---|---|
| `id` | superpipeline's gate id | `vrd_…` |
| `kind` | `gate` | `grader`, `review`, `eval` or `calibration` |
| `source` | `superpipeline` | `superwitness` |
| `subject` | the run's ref | the verdict's `subject_ref`: the run's ref, or an attempt id |
| `value` | `{"decision": "<superpipeline's decision>"}`, or `{"decision": null}` when not decided | as recorded |
| `judge` | the decider's principal id; superpipeline's own id for a decider it has not mapped; or `"unknown"` | the principal that recorded it |
| `judge_kind` | `human`, `agent`, `grader` or `unknown` | `human`, `agent` or `grader` |
| `standard` | `stage:<key>` | as recorded |
| `status` | `pending`, `resolved`, `cancelled` or `unknown` | left out |
| `comment` | left out | the comment; left out when empty |
| `at` | when it was resolved or cancelled, else when it was created; `"unknown"` if a resolved gate has no time | when it was recorded |
| `run_id` | the run the gate was raised on, or `"unknown"` | left out |
| `supersedes` | left out | the verdict it corrects; left out when none |

A gate superpipeline reports as resolved but without a decision shows as `pending`.

### `cost`

| Field | Value |
|---|---|
| `status` | `reported`, `unreported` or `unknown` |
| `input_tokens`, `output_tokens` | number, or `null` |
| `usd` | number, or `null` |

The three numbers can be non-null only when `status` is `reported`.

### `sources`

One key for each source, always all six: `superpipeline`, `agentpod`, `traces`, `logs`, `errors`
and `verdicts`. Each value is `ok`, `unavailable`, `timeout`, `not_found` or `unauthorized`.

### `links`

`self`, `spans` and `logs`: the paths of this document and of its
[paged spans and logs](/use/read-a-run/#spans-one-page-at-a-time).

### Example

The development run that fake mode serves, shortened:

```json
{
  "run": {"ref": "superpipeline:brd_01/run_01", "card": {"id": "crd_01", "title": "Write the release note"},
          "stage": "draft", "agent": "prn_agent01", "agent_ref": "agt_01", "state": "completed",
          "started_at": "2026-10-04T10:00:00Z", "ended_at": "2026-10-04T10:05:00Z"},
  "attempts": [{"id": "attempt_01", "station": "stn_01", "state": "completed", "session_id": "acps_01",
                "seq_from": 1, "seq_to": 9, "started_at": "2026-10-04T10:00:02Z", "ended_at": "2026-10-04T10:04:58Z",
                "fingerprint": {"digest": "sha256:c1a8…3426", "harness": "hermes", "harness_version": "unknown",
                                "model": "unknown", "profile": "default", "skill_release": "none", "reported_by": "hub"},
                "span_count": 2}],
  "trace": {"trace_ids": ["4bf92f3577b34da6a3ce929d0e0e4736", "5bf92f3577b34da6a3ce929d0e0e4737"],
            "status": "partial", "sampled": true},
  "errors": [{"service": "agentpod-hub", "message": "bridge heartbeat failed: 502 from superpipeline",
              "trace_id": "4bf92f3577b34da6a3ce929d0e0e4736", "at": "2026-10-04T10:02:00Z"}],
  "verdicts": [{"id": "gate_01", "kind": "gate", "source": "superpipeline", "subject": "superpipeline:brd_01/run_01",
                "value": {"decision": "rejected"}, "judge": "prn_human01", "judge_kind": "human",
                "standard": "stage:draft", "status": "resolved", "at": "2026-10-04T10:20:00Z", "run_id": "run_01"}],
  "cost": {"status": "unreported", "input_tokens": null, "output_tokens": null, "usd": null},
  "log_count": 2,
  "sources": {"agentpod": "ok", "errors": "ok", "logs": "ok", "superpipeline": "ok", "traces": "ok", "verdicts": "ok"},
  "links": {"self": "/v1/runs/superpipeline/brd_01/run_01",
            "spans": "/v1/runs/superpipeline/brd_01/run_01/spans",
            "logs": "/v1/runs/superpipeline/brd_01/run_01/logs"}
}
```

## Verdicts

What `POST /v1/verdicts` and the `record_verdict` tool return:

| Field | Value |
|---|---|
| `id` | `vrd_` and 20 hex characters |
| `idempotency_key` | as sent |
| `kind` | `grader`, `review`, `eval` or `calibration` |
| `subject_kind` | `run`, `attempt` or `eval_case_run` |
| `subject_ref` | `superpipeline:<board>/<run>`, `attempt_<id>` or `case:<id>@sha256:<64 hex>` |
| `judge` | the caller's principal id |
| `judge_kind` | `human`, `agent` or `grader`, from the caller's principal kind |
| `standard` | `rubric:<id>@<version>`, `stage:<key>` or `case:<id>` |
| `value` | an object with exactly one of `decision`, `score`, `label` or `text` |
| `comment` | a string, `""` when none |
| `evidence_refs` | an array of span ids and `{session_id, seq_from, seq_to}` ranges, `[]` when none |
| `supersedes` | the id of the verdict it corrects, or `null` |
| `created_at` | RFC 3339 time in UTC |

The database allows a fourth judge kind, `rule`, which no principal records in this release.
[Verdicts](/use/verdicts/) has the rules for each field.

## Run reports

What a source sends to `POST /v1/runs` is described by a JSON Schema at
[`/schemas/run-report.schema.json`](/schemas/run-report.schema.json), also in the repository at
`internal/contracts/run-report.schema.json`. A source can vendor it and validate its own output in
its tests. [Run registry API](/build/run-registry/) explains each field.

## Spans

The span fields that matter to a run document are `run.id`, `attempt.id` and the root span name
`dispatch`. [Sending telemetry](/use/telemetry/#spans) covers them and the trace shape
superwitness expects.

## What superwitness reads, pinned

superwitness keeps a recorded example of each payload it reads from another product, in
`internal/contracts/` in its repository. Its adapter tests decode them, and fake mode serves the
development run from them:

| File | The answer from |
|---|---|
| `superpipeline_evidence.json` | superpipeline's run evidence route: run, card, gates and usage |
| `hub_evidence_run.json` | the AgentPod hub's run evidence route: the ledger, attempts and fingerprints |
| `hub_evidence_attempt.json` | the hub's attempt route: which run an attempt belongs to |
| `hub_principal.json` | the hub's principal route: a principal's id and kind |
| `jaeger_trace_hub.json` | VictoriaTraces' Jaeger API: a `dispatch`, `attempt` and `turn` trace from `agentpod-hub` |
| `jaeger_trace_workers.json` | VictoriaTraces' Jaeger API: a span from `superpipeline-api` carrying `run.id` |

[With AgentPod and superpipeline](/use/with-agentpod-and-superpipeline/) lists the routes these
come from.
