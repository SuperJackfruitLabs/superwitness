---
title: Verdicts
description: Recording a verdict, the standards it names, and why verdicts are append-only.
---

A verdict is a judgement about a run, an attempt or an eval case run, against a named standard.
superwitness stores verdicts in its own Postgres and never changes one once it is written.
[Concepts](/concepts/#verdict) introduces them; this page is how to record them.

## Record a verdict

Post one JSON object to `POST /v1/verdicts`, or call the `record_verdict` MCP tool with the same
fields:

```sh
curl -X POST -H 'Authorization: Bearer <token>' -H 'Content-Type: application/json' \
  https://superwitness.example.internal/v1/verdicts -d '{
    "idempotency_key": "review-brd_01-run_01-1",
    "subject_kind": "run",
    "subject_ref": "superpipeline:brd_01/run_01",
    "standard": "stage:draft",
    "value": {"decision": "approved"},
    "comment": "Reads well."
  }'
```

| Field | Required | Meaning |
|---|---|---|
| `idempotency_key` | yes | a key you choose, 1 to 200 bytes; see [Retries](#retries-and-idempotency) |
| `kind` | no | `grader`, `review`, `eval` or `calibration`; left out, `review` for a human judge and `grader` for anyone else |
| `subject_kind` | yes | `run`, `attempt` or `eval_case_run` |
| `subject_ref` | yes | `superpipeline:<board>/<run>`, `attempt_<id>`, or `case:<id>@sha256:<64 hex>` to match `subject_kind` |
| `standard` | yes | what the verdict judges against; see [Standards](#standards) |
| `value` | yes | the judgement; see [Values](#values) |
| `comment` | no | free text, at most 10000 characters |
| `evidence_refs` | no | up to 100 pointers to evidence; see [Evidence references](#evidence-references) |
| `supersedes` | no | the id of your earlier verdict that this one corrects |

The HTTP body also accepts `judge`, which must equal the caller's own principal id if it is
present, and refuses `judge_kind` outright (400 `judge_kind_not_accepted`): the judge is always
the caller, and its kind comes from the caller's principal record. Any other field is refused as
400 `invalid_json`, as is a body over 64 KiB or one holding more than one JSON value.

A `kind` outside the four is 400 `invalid_kind`; a `subject_kind` outside the three is 400
`invalid_subject_kind`; a `subject_ref` that does not fit its kind is 400 `invalid_subject`.

A new verdict answers 201 with the stored verdict:

```json
{
  "id": "vrd_…",
  "idempotency_key": "review-brd_01-run_01-1",
  "kind": "review",
  "subject_kind": "run",
  "subject_ref": "superpipeline:brd_01/run_01",
  "judge": "prn_human01",
  "judge_kind": "human",
  "standard": "stage:draft",
  "value": {"decision": "approved"},
  "comment": "Reads well.",
  "evidence_refs": [],
  "supersedes": null,
  "created_at": "…"
}
```

`comment` is `""` and `evidence_refs` is `[]` when none were given. Recorded verdicts on the run
and its attempts appear in the run document's `verdicts`, after the run's gates.

## Values

`value` is an object with exactly one key:

| Value | Rule |
|---|---|
| `{"decision": "…"}` | a non-empty string |
| `{"score": 0.7}` | a number from 0 to 1 |
| `{"label": "…"}` | a non-empty string |
| `{"text": "…"}` | a non-empty string |

Anything else is 400 `invalid_value`, as is a string holding NUL or an unpaired UTF-16 surrogate
escape.

## Standards

Every verdict names a standard. Leaving it out is 422 `missing_standard`; a standard in none of
these forms is 422 `invalid_standard`:

| Standard | Form |
|---|---|
| `rubric:<id>@<version>` | a rubric recorded in superwitness: id of letters, digits, `_`, `.` and `-`, up to 64; version a whole number from 1 |
| `stage:<key>` | a pipeline stage: key of letters, digits, `_`, `.` and `-`, up to 64 |
| `case:<id>` | an eval case: id of letters, digits, `_`, `.` and `-`, up to 128 |

A rubric standard must name a rubric version that exists, or the verdict is 422
`unknown_rubric`.

### Rubrics

Rubrics are recorded with the binary, against the same database:

```sh
superwitness rubric-add -id press -version 1 -name Press \
  -scale '{"kind":"decision","options":["pass","fail"]}' -body-file press.md -created-by prn_human01
```

All six flags are required: `-id`, `-version` (1 or more), `-name`, `-scale` (valid JSON),
`-body-file` (a file holding the rubric text) and `-created-by` (the author's principal id).
`rubric-add` needs `SW_DATABASE_URL`. It runs any pending migrations first, over
`SW_MIGRATE_DATABASE_URL` when that is set, then inserts the rubric over `SW_DATABASE_URL` and
prints `rubric:press@1 recorded`. Give rubrics ids that a standard can name: letters, digits,
`_`, `.` and `-`, up to 64.

Rubrics are append-only, like verdicts. A version, once recorded, cannot be changed or
recorded again; `rubric-add` fails with `rubric version already exists`. To change a rubric,
record the next version and judge against `rubric:press@2`.

### Rubric scales

A rubric's scale is JSON. These shapes are recognised, and the superwitness app offers an input
for each:

| Scale | Input | Value recorded |
|---|---|---|
| `{"kind":"decision","options":["pass","fail"]}` (2 to 20 options) | one button per option | `{"decision": "pass"}` |
| `{"kind":"score"}` | a slider from 0 to 1 in steps of 0.05 | `{"score": 0.75}` |
| `{"kind":"label","labels":["clear","muddled"]}` (1 to 50 labels) | a pick list | `{"label": "clear"}` |
| `{"kind":"text"}` | a text area | `{"text": "…"}` |
| `{"min":0,"max":1}` (any range, the older form) | a slider over the range | `{"score": …}`, rescaled to 0 to 1 |

Each shape allows no other key. `rubric-add` records a rubric in any other shape but prints a
warning, and `GET /v1/rubrics` shows its `recognised_scale` as `null`. `GET /v1/rubrics` and
`GET /v1/rubrics/{id}/{version}` read rubrics back ([HTTP API](/build/api/#get-v1rubrics-and-get-v1rubricsidversion)).

## Subjects and who may judge them

- A `run` or `attempt` subject must exist. superwitness checks with superpipeline and the
  AgentPod hub; a subject neither knows is 404 `subject_not_found`, and one it cannot check is
  503 `subject_unresolved`.
- An `eval_case_run` subject is not checked against any product.
- **A non-human judge may not judge a run or attempt it executed**: 403 `self_judgement`. The
  executors are the run's agent principal from superpipeline and each attempt's agent
  principal from AgentPod. If superwitness cannot establish every one of them as a hub
  principal, because a source did not answer or an executor is not mapped to a principal yet,
  a non-human verdict on that subject is 503 `subject_unresolved` rather than accepted on
  incomplete information.
- A human judge may judge any subject, including one they executed.

The judge's kind is never taken from the request. A human principal records as `human`, an
agent as `agent`, and a service as `grader`.

## Corrections: append-only

Nothing about a verdict can change once it is written. In the database, triggers refuse
`UPDATE` and `DELETE` on each row of `verdicts` and `rubrics`, and `TRUNCATE` on either table.
The runtime role has only `SELECT` and `INSERT` on the two tables, and is
not their owner, so it cannot drop the triggers either ([Install](/install/#postgres-roles)).

A change of mind is a new verdict that names the old one in `supersedes`:

```json
{
  "idempotency_key": "review-brd_01-run_01-2",
  "subject_kind": "run",
  "subject_ref": "superpipeline:brd_01/run_01",
  "standard": "stage:draft",
  "value": {"decision": "changes_requested"},
  "supersedes": "vrd_…"
}
```

- Only the judge of the old verdict may supersede it: anyone else gets 403
  `not_original_judge`.
- The correction must name the same subject and standard: 422 `supersedes_mismatch`.
- The old verdict must exist: 422 `supersedes_not_found`. `supersedes` must look like a
  verdict id (`vrd_…`): 400 `invalid_supersedes`.
- Each verdict is superseded at most once, so a chain stays a line. Superseding a verdict that
  already has a successor is 409 `already_superseded`; supersede the latest in the chain
  instead.

The run document shows only the latest verdict in each chain.

## Retries and idempotency

Every verdict carries an `idempotency_key`, and a key is used once across the whole store. Send
the same request again with the same key, and superwitness returns the verdict it already
stored, with 200 instead of 201, and writes nothing. Send a different verdict with a key that
is already used, by you or anyone else, and it is 409 `idempotency_conflict`.

So on a timeout or a retryable error, retry with the same key. The retryable errors are 503
`store_unavailable` and 503 `subject_unresolved`; both carry `"retryable": true` and a
`Retry-After: 5` header.

## Evidence references

`evidence_refs` points at evidence; it never holds it. It is an array of at most 100 entries,
each either:

- a span id: 16 lowercase hex characters, such as `"00f067aa0ba902b8"`; or
- a range of session events: `{"session_id": "…", "seq_from": 1, "seq_to": 9}`, with
  `seq_from` 0 or more and `seq_to` not below it, and no other keys.

Anything else is 400 `invalid_evidence_refs`.

## When the database is down

`serve` runs its migrations in the background at start. Until they succeed, and whenever
Postgres cannot answer, `POST /v1/verdicts` is 503 `store_unavailable`, run documents show
`sources.verdicts` as `unavailable` (or `timeout`), and the rest of the document still renders.
