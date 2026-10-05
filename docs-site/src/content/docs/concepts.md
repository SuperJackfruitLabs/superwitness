---
title: Concepts
description: Runs, attempts, configuration fingerprints, verdicts, sources and their statuses, and why superwitness says unknown rather than 0.
---

The words below are the field names and values superwitness returns. The run document,
`GET /v1/runs/superpipeline/{board}/{run}`, uses all of them.

## Run

A **run** is one piece of work on a superpipeline board. It is addressed by its board and run
ids, and the document names it in `run.ref` as `superpipeline:<board>/<run>`, for example
`superpipeline:brd_01/run_01`. Board and run ids are letters, digits, `_` and `-`, starting with
a letter or digit, up to 128 characters.

The `run` section carries the card it belongs to (`card.id`, `card.title`), its `stage`, its
`state`, `started_at` and `ended_at`, and two agent fields: `agent` is the agent's principal id
and `agent_ref` is superpipeline's own id for it. `ended_at` is `null` while the run has not
ended, and `"unknown"` when no source could say.

superwitness owns none of this. It reads the run from superpipeline every time the document is
asked for. When superpipeline cannot answer, the document falls back to what AgentPod's ledger
knows about the run: the card id, the state, the agent and the start time. When neither knows
the run, the request answers 404 `run_not_found`.

## Attempt

An **attempt** is one execution of the run by an agent on AgentPod. A run can have several.
Each entry in `attempts` has:

| Field | Meaning |
|---|---|
| `id` | `attempt_<id>` |
| `station` | the AgentPod station it ran on |
| `state` | the attempt's state as AgentPod reports it |
| `session_id`, `seq_from`, `seq_to` | the session and the range of its events the attempt covers; `seq_to` is `null` while it is open |
| `started_at`, `ended_at` | when it ran |
| `fingerprint` | the configuration that ran it (below) |
| `span_count` | spans carrying this attempt's id as `attempt.id`, or `"unknown"` |

`GET /v1/runs/by-attempt/{attempt}` redirects (302) from an attempt to its run's document.

## Configuration fingerprint

Every attempt carries the **fingerprint** of the configuration that ran it:

| Field | Meaning |
|---|---|
| `digest` | one `sha256:` digest of the configuration, so two attempts with the same digest ran the same configuration |
| `harness`, `harness_version` | the agent harness and its version |
| `model` | the model |
| `profile` | the harness profile |
| `skill_release` | the skill release installed |
| `reported_by` | which component reported the fingerprint |

The fingerprint is reported with the attempt by AgentPod; superwitness shows it as reported. A
field nobody reported reads `"unknown"`. It is what lets you ask whether a change of
configuration changed the results.

## Verdict

A **verdict** is a judgement about a subject, against a named standard. superwitness's own
database holds verdicts, append-only. The run document shows two kinds side by side in
`verdicts`:

- **Gates** from superpipeline: `kind` is `gate`, `source` is `superpipeline` and `standard` is
  `stage:<key>`. A gate's `status` is `pending`, `resolved` or `cancelled`, or `unknown` when
  superpipeline reports a status this release does not know; a decided gate's
  value is `{"decision": …}`. superwitness shows gates and never stores them.
- **Recorded verdicts**, posted to `POST /v1/verdicts` or the `record_verdict` MCP tool. Their
  `kind` is `grader`, `review`, `eval` or `calibration`. Left out, it is `review` for a human
  judge and `grader` for anyone else.

Every verdict has:

- **A subject.** Its `subject_kind` is one of:
  - `run`, with a reference `superpipeline:<board>/<run>`;
  - `attempt`, with a reference `attempt_<id>`;
  - `eval_case_run`, with a reference `case:<id>@sha256:<fingerprint>`.
- **A value**: an object with exactly one of `decision` (text), `score` (a number from 0 to 1),
  `label` (text) or `text`.
- **A judge and a judge kind.** The judge is the principal that recorded it. Its `judge_kind` is
  taken from that principal's record, never from the request: a human principal judges as
  `human`, an agent as `agent`, a service as `grader`. The model has a fourth kind, `rule`; no
  principal records one in this release. Only a human may judge a run or attempt they executed;
  any other judge that executed it is refused.
- **A standard**, always versioned or keyed: `rubric:<id>@<version>` for a rubric that exists in
  superwitness, `stage:<key>` for a pipeline stage, or `case:<id>` for an eval case.

A verdict is never edited. To correct one, its judge records a new verdict on the same subject
and standard that names the old one in `supersedes`. The run document shows only the latest
verdict in each chain.

## Sources and their statuses

The run document is assembled at read time from **sources**, and `sources` says how each one
answered:

| Source | What it supplies |
|---|---|
| `superpipeline` | the run, its card, stage, state, cost and gates |
| `agentpod` | the attempts and their fingerprints |
| `traces` | the run's spans and trace ids, from VictoriaTraces |
| `logs` | the run's log count, from VictoriaLogs |
| `errors` | the run's error-level log lines, from VictoriaLogs |
| `verdicts` | recorded verdicts, from superwitness's own database |

Each source's status is one of:

| Status | Meaning |
|---|---|
| `ok` | it answered |
| `unavailable` | it could not be reached or failed |
| `timeout` | it did not answer in time (`SW_SOURCE_TIMEOUT`, 2 s by default) |
| `not_found` | it answered and does not know the run |
| `unauthorized` | it refused superwitness's credentials |

A source that fails never fails the document. The rest of it still renders. The counts and text
fields that source would have supplied read `"unknown"`, and the lists it would have filled
(`attempts`, `errors`, `verdicts`, `trace.trace_ids`) come back empty; `sources` says why.

## Unknown is not 0

A count in the run document, such as `log_count` or an attempt's `span_count`, is either a
number a source supplied or the string `"unknown"` when it could not. It is never a silent 0: a
0 means a source answered and found none. The same holds for `trace.sampled`, which is `true` when
spans were found and `"unknown"` otherwise, and for text fields, which read `"unknown"` rather
than being empty.

Cost follows the same rule. `cost.status` is `reported` when superpipeline reported token counts
and spend, `unreported` when superpipeline answered without them, and `unknown` when it did not
answer.

## Trace status

`trace.status` says how well the run's spans join up:

| Status | Meaning |
|---|---|
| `joined` | the spans found share one trace id, and it has a root span named `dispatch` |
| `partial` | spans were found, but not in that shape |
| `none` | the traces source answered and found no spans for the run |
| `unknown` | the traces source did not answer |

`none` is not an error. Telemetry is sampled and short-lived, so missing spans are not missing
evidence: attempts, gates and verdicts still stand.
