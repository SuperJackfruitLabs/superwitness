---
title: Read a run
description: The run page, the run document, and the paged spans and logs behind them.
---

A run can be read three ways: in a browser on the run page, over HTTP, or through the
[MCP tools](/build/mcp/). All three return the same run document, assembled at read time from
every source. [Concepts](/concepts/) explains its fields; [Contracts](/build/contracts/) lists
every one.

## The run page

superwitness serves a page for each run at `/runs/superpipeline/{board}/{run}` on its own
address, for example `/runs/superpipeline/brd_01/run_01`. The page is built into the binary.

It asks for a hub token whose audience is this superwitness (its `SW_PUBLIC_URL`; see
[With AgentPod and superpipeline](/use/with-agentpod-and-superpipeline/#tokens-callers-present)).
You paste the token once; the page keeps it in the browser tab's session storage, so it is gone
when the tab closes. If superwitness answers 401 or 403, the page drops the token and asks
again.

The page then shows, from the run document:

- the run: its ref, card title, stage, state, agent, and start and end times;
- **Sources**, with each source's status;
- **Attempts**: id, station, state, fingerprint digest, harness and version, model, and span
  count;
- **Trace**: the trace status, `sampled`, and the trace ids;
- **Errors**: time, service, message and trace id of each error line;
- **Verdicts**: kind, source, status, value, judge, judge kind, standard and time, gates and
  recorded verdicts together;
- **Cost**: status, input and output tokens, and USD;
- **Logs**: the run's log count, and a button that loads the first 100 lines, then the next 100.

An empty section reads `none` only when every source behind it answered `ok`. Otherwise it reads
`unknown` and names the source and its status, because an empty list from a source that did not
answer is not evidence that there was nothing.

The page does not list spans; read them through the API below.

## The run document

```sh
curl -H 'Authorization: Bearer <token>' \
  https://superwitness.example.internal/v1/runs/superpipeline/brd_01/run_01
```

`GET /v1/runs/superpipeline/{board}/{run}` returns the run document. Board and run ids are
letters, digits, `_` and `-`, starting with a letter or digit, up to 128 characters; anything
else is 400 `invalid_run_ref`. The answer is 404 `run_not_found` only when superpipeline and the
AgentPod hub both answer that they do not know the run. Any other failure of a source is a 200
with that source's status in `sources`.

The document's `links` hold the paths for this run:

```json
"links": {
  "self": "/v1/runs/superpipeline/brd_01/run_01",
  "spans": "/v1/runs/superpipeline/brd_01/run_01/spans",
  "logs": "/v1/runs/superpipeline/brd_01/run_01/logs"
}
```

## From an attempt to its run

`GET /v1/runs/by-attempt/{attempt}` asks the AgentPod hub which run an attempt belongs to and
answers 302, with the run document's path in `Location`:

```
HTTP/1.1 302 Found
Location: /v1/runs/superpipeline/brd_01/run_01
```

An attempt id is `attempt_<id>`; anything else is 400 `invalid_attempt_id`. The hub not knowing
the attempt is 404 `attempt_not_found`, and an attempt that was not dispatched from a
superpipeline run is 404 `attempt_has_no_run`.

## Spans, one page at a time

`GET /v1/runs/superpipeline/{board}/{run}/spans?cursor=&limit=` lists the run's spans, oldest
first (by start time, then trace id, then span id):

```json
{
  "spans": [
    {
      "trace_id": "4bf92f3577b34da6a3ce929d0e0e4736",
      "span_id": "00f067aa0ba902b7",
      "name": "dispatch",
      "service": "agentpod-hub",
      "start": "2026-10-04T10:00:00Z",
      "duration_ms": 300000,
      "attributes": {"board.id": "brd_01", "card.id": "crd_01", "external.source": "superpipeline", "run.id": "run_01"}
    }
  ],
  "next_cursor": "eyJvIjoxfQ"
}
```

Each span has `trace_id`, `span_id`, `parent_span_id` (left out for a span with no parent),
`name`, `service`, `start`, `duration_ms` and `attributes`, whose values are all strings.
`next_cursor` is left out on the last page. Which spans count as the run's is covered in
[Sending telemetry](/use/telemetry/#spans).

## Logs, one page at a time

`GET /v1/runs/superpipeline/{board}/{run}/logs?cursor=&level=&limit=` lists the run's log lines,
oldest first:

```json
{
  "logs": [
    {
      "at": "2026-10-04T10:00:02Z",
      "service": "agentpod-hub",
      "level": "info",
      "message": "attempt opened",
      "trace_id": "4bf92f3577b34da6a3ce929d0e0e4736",
      "span_id": "00f067aa0ba902b8",
      "run_id": "run_01"
    }
  ],
  "next_cursor": "eyJvIjoxfQ",
  "trace_join": "ok"
}
```

- `level` filters by level: `debug`, `info`, `warn` or `error`. Left out, every level is listed.
  Any other value is 400 `invalid_level`.
- Each line's `level` is one of `debug`, `info`, `warn`, `error` or `unknown`, and `service` is
  `unknown` when the line has no service name. `trace_id`, `span_id` and `run_id` are left out
  when the line has none.
- `next_cursor` is `null` on the last page.
- A line belongs to the run when it carries the run's id, or one of the run's trace ids. To
  know the trace ids, superwitness first reads the run's spans; `trace_join` is the traces
  source's status for that read. When it is not `ok`, lines were matched by run id alone.

## Paging

Both lists page the same way:

- `limit` is the page size. Left out, it is 100; above 500, it is 500. Anything that is not a
  whole number of at least 1 is 400 `invalid_limit`.
- `cursor` is the `next_cursor` from the previous page. Treat it as opaque. A cursor
  superwitness did not issue is 400 `invalid_cursor`.

If the engine behind a list cannot answer, the request fails rather than returning an empty
page: `traces_unavailable` or `logs_unavailable` (503), `traces_timeout` or `logs_timeout` (504),
or `traces_unauthorized` or `logs_unauthorized` (502). The [HTTP API](/build/api/#errors) lists
every error.
