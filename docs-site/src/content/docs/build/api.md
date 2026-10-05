---
title: HTTP API
description: The HTTP routes superwitness serves, their parameters and their errors.
---

superwitness serves a small JSON API under `/v1`. The [MCP tools](/build/mcp/) are a thin layer
over the same operations. One route, `GET /v1/runs/by-attempt/{attempt}`, is HTTP-only and has no tool.

## Authentication

Every `/v1` route takes a bearer token issued by the AgentPod hub, with superwitness's
`SW_PUBLIC_URL` as its audience:

```
Authorization: Bearer <token>
```

[With AgentPod and superpipeline](/use/with-agentpod-and-superpipeline/#tokens-callers-present)
lists what a token must carry. A missing or refused token is 401 `unauthenticated`. In fake
mode, tokens are `dev:<principal>:<human|agent|service>`.

## Routes

| Method | Path | Answers |
|---|---|---|
| `GET` | `/v1/runs/superpipeline/{board}/{run}` | 200 with the [run document](/build/contracts/#the-run-document) |
| `GET` | `/v1/runs/superpipeline/{board}/{run}/spans` | 200 with one page of spans |
| `GET` | `/v1/runs/superpipeline/{board}/{run}/logs` | 200 with one page of log lines |
| `GET` | `/v1/runs/by-attempt/{attempt}` | 302 to the run document |
| `POST` | `/v1/verdicts` | 201 with a new verdict, or 200 with the one already recorded under the key |

Path parameters:

- `{board}` and `{run}` are superpipeline board and run ids: letters, digits, `_` and `-`,
  starting with a letter or digit, up to 128 characters. Otherwise 400 `invalid_run_ref`.
- `{attempt}` is `attempt_` followed by 1 to 128 letters, digits, `_` or `-`. Otherwise 400
  `invalid_attempt_id`.

JSON answers carry `Content-Type: application/json` and `Cache-Control: no-store`.

### GET /v1/runs/superpipeline/{board}/{run}

The run document, assembled from every source at read time. It is 404 `run_not_found` only when
superpipeline and the AgentPod hub both say they do not know the run; any other source failure
is a 200 with that source's status in `sources`. [Read a run](/use/read-a-run/#the-run-document).

### GET /v1/runs/superpipeline/{board}/{run}/spans

| Query | Meaning |
|---|---|
| `cursor` | `next_cursor` from the previous page; left out for the first page |
| `limit` | page size, 1 or more; default 100, and anything above 500 is 500 |

Answers `{"spans": [...], "next_cursor": "…"}`, oldest first; `next_cursor` is left out on the
last page. Each span has `trace_id`, `span_id`, `parent_span_id` (left out for a root span),
`name`, `service`, `start`, `duration_ms` and `attributes` (string values).

### GET /v1/runs/superpipeline/{board}/{run}/logs

| Query | Meaning |
|---|---|
| `cursor` | `next_cursor` from the previous page; left out for the first page |
| `level` | `debug`, `info`, `warn` or `error`; left out for every level |
| `limit` | page size, 1 or more; default 100, and anything above 500 is 500 |

Answers `{"logs": [...], "next_cursor": "…" or null, "trace_join": "<status>"}`, oldest first;
`next_cursor` is `null` on the last page. Each line has `at`, `service`, `level`, `message`, and,
when the line has them, `trace_id`, `span_id` and `run_id`. `trace_join` is the traces source's
status while the run's trace ids were looked up; when it is not `ok`, lines were matched by run
id only. [Read a run](/use/read-a-run/#logs-one-page-at-a-time).

### GET /v1/runs/by-attempt/{attempt}

302 with `Location: /v1/runs/superpipeline/{board}/{run}` for the run the attempt belongs to,
and no body.

### POST /v1/verdicts

Body: one JSON verdict request, at most 64 KiB, with no unknown fields. Answers the stored
verdict. [Verdicts](/use/verdicts/) has every field and rule; [Contracts](/build/contracts/#verdicts)
has the shape.

## Outside /v1

| Path | |
|---|---|
| `GET /health` | No token. 200 whenever the process is alive, with each source's status in the body ([Operations](/use/operations/#health)). |
| `/mcp` | The MCP server, with the same bearer token ([MCP tools](/build/mcp/)). |
| `GET /runs/superpipeline/{board}/{run}` | The run page, a browser page built into the binary ([Read a run](/use/read-a-run/#the-run-page)). |

Any other `GET` or `HEAD` outside `/v1` and `/mcp` is served by the run page: one of its files,
or the page itself for a path without a file extension. Other methods on unknown paths are 404
`not_found`. A wrong method on a route that exists, such as `POST /health`, is 405
`method_not_allowed`.

## Errors

Every error from `/v1` is one JSON object:

```json
{"error":{"code":"logs_unavailable","message":"logs is unavailable","retryable":true}}
```

- `code` is stable; match on it.
- `message` is for people and may change.
- `retryable` is present, and `true`, only when the same request may succeed later. A retryable
  503 also carries `Retry-After: 5`.

### Every code

| Status | Code | When |
|---|---|---|
| 400 | `invalid_run_ref` | a board or run id is malformed |
| 400 | `invalid_attempt_id` | the attempt id is not `attempt_<id>` |
| 400 | `invalid_limit` | `limit` is not a whole number of 1 or more |
| 400 | `invalid_cursor` | `cursor` is not one superwitness issued |
| 400 | `invalid_level` | `level` is not `debug`, `info`, `warn` or `error` |
| 400 | `invalid_json` | the verdict body is not exactly one JSON object of known fields, or is over 64 KiB |
| 400 | `invalid_idempotency_key` | the key is missing, over 200 bytes, or holds NUL |
| 400 | `invalid_kind` | `kind` is not `grader`, `review`, `eval` or `calibration` |
| 400 | `invalid_subject_kind` | `subject_kind` is not `run`, `attempt` or `eval_case_run` |
| 400 | `invalid_subject` | `subject_ref` does not fit its `subject_kind` |
| 400 | `invalid_value` | `value` is not exactly one of decision, score (0 to 1), label or text |
| 400 | `invalid_comment` | `comment` holds NUL |
| 400 | `comment_too_long` | `comment` is over 10000 characters |
| 400 | `invalid_evidence_refs` | `evidence_refs` is not up to 100 span ids or session ranges |
| 400 | `invalid_supersedes` | `supersedes` is not a verdict id |
| 400 | `judge_mismatch` | `judge` is set to someone other than the caller |
| 400 | `judge_kind_not_accepted` | `judge_kind` is set; it comes from the caller's principal record |
| 401 | `unauthenticated` | the bearer token is missing or not valid for this service |
| 403 | `self_judgement` | a non-human judge's verdict on a run or attempt it executed |
| 403 | `not_original_judge` | superseding someone else's verdict |
| 404 | `run_not_found` | neither superpipeline nor the hub knows the run |
| 404 | `attempt_not_found` | the hub does not know the attempt |
| 404 | `attempt_has_no_run` | the attempt was not dispatched from a superpipeline run |
| 404 | `subject_not_found` | the verdict's run or attempt does not exist |
| 404 | `not_found` | no such route |
| 405 | `method_not_allowed` | the route exists, but not with this method |
| 409 | `idempotency_conflict` | the key was already used for a different verdict |
| 409 | `already_superseded` | the verdict named in `supersedes` already has a successor |
| 422 | `missing_standard` | the verdict names no standard |
| 422 | `invalid_standard` | the standard is not `rubric:<id>@<version>`, `stage:<key>` or `case:<id>` |
| 422 | `unknown_rubric` | the rubric version does not exist |
| 422 | `supersedes_not_found` | the verdict named in `supersedes` does not exist |
| 422 | `supersedes_mismatch` | the correction names a different subject or standard |
| 500 | `internal` | an unexpected failure |
| 502 | `<source>_unauthorized` | a source refused superwitness's credential |
| 503 | `<source>_unavailable` | a source could not be reached or failed (retryable) |
| 503 | `store_unavailable` | the verdict database is unavailable (retryable) |
| 503 | `subject_unresolved` | superwitness could not confirm the subject or who executed it (retryable) |
| 504 | `<source>_timeout` | a source did not answer in time (retryable) |

`<source>` is the source a route depends on: `traces` for spans, `logs` for logs, and
`agentpod` for `by-attempt`. So the source codes are `traces_unavailable`, `traces_timeout`,
`traces_unauthorized`, `logs_unavailable`, `logs_timeout`, `logs_unauthorized`,
`agentpod_unavailable`, `agentpod_timeout` and `agentpod_unauthorized`. The run document itself
never fails for a source; it reports the source's status instead.

The 401 comes from the authentication layer and carries `WWW-Authenticate:
Bearer realm="superwitness"`; its body has `code` and `message` only.
