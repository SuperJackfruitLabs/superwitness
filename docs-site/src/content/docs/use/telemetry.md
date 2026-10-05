---
title: Sending telemetry
description: Which spans and log lines superwitness finds for a run, the trace conventions it relies on, and the telemetry it sends about itself.
---

superwitness does not receive telemetry. Products send OTLP to the OpenTelemetry Collector,
which writes spans to VictoriaTraces and logs to VictoriaLogs ([Install](/install/#the-telemetry-engines)
has a minimal configuration). When a run is read, superwitness queries both engines through their
published query APIs and keeps nothing. This page is what it looks for, so you know what to send.

Both engines are searched over the last 7 days, and older telemetry is not found. A run whose
spans are not found has trace status `none`, which is not an error.

## Spans

superwitness reads spans from VictoriaTraces' Jaeger-compatible API, with one request for each
of these services:

- `agentpod-hub`
- `agentpod-node-agent`
- `superpipeline-api`

```
GET /select/jaeger/api/traces?service=<service>&tags={"run.id":"<run>"}&start=<µs>&end=<µs>&limit=100
```

So spans are found through the **`run.id` attribute**, which holds the superpipeline run id
(`run_01`, not the full ref), on a span from one of those three services. Each request returns
up to 100 traces, and every span of those traces is listed, including spans without `run.id` of
their own. A span that appears in more than one answer is listed once. A trace is found only
through a span from one of these three services; spans from any other service appear only when
they share a trace with one.

From the spans it finds, superwitness uses:

| What | Used for |
|---|---|
| `run.id` attribute | finding the run's traces |
| trace ids | `trace.trace_ids`, ordered by each trace's earliest span, and matching log lines |
| `attempt.id` attribute | each attempt's `span_count`: spans whose `attempt.id` is that attempt's id |
| a span named `dispatch` with no parent | `trace.status` is `joined` when every span found is in one trace that has this root |

If any of the three requests fails, the traces source reports the worst status among them, and
superwitness uses none of the answers rather than some of them.

### The trace shape superwitness expects

superwitness's contract fixtures pin the trace it is built to read, with these spans from
`agentpod-hub`:

- a root span **`dispatch`**, carrying `run.id`, `board.id`, `card.id` and `external.source`;
- under it, an **`attempt`** span per attempt, carrying `attempt.id`, `station.id`,
  `fingerprint.digest`, `harness.name`, `acp.session_id`, `acp.seq_from` and `run.id`;
- under that, **`turn`** spans carrying `attempt.id`, `acp.seq_from` and `acp.seq_to`;
- spans from `superpipeline-api` carrying `run.id`.

### Trace context between products

For a run to land in one trace, each hop must continue the caller's trace context. This is the
convention AgentPod and superpipeline follow. They carry the context; superwitness only reads
the trace that results:

| Hop | Where the context travels |
|---|---|
| HTTP between products | the W3C `traceparent` header |
| AgentPod's hub to its node-agent, over WebSocket | `_meta.traceparent` on each request message |
| node-agent to the agent harness, over ACP | `traceparent` in `_meta` on `session/new` and `session/prompt` |

superwitness does not send or read `_meta` itself. When a hop does not continue the trace, the
run's spans arrive in more than one trace. superwitness still finds them through `run.id`, lists
every trace id, and reports `trace.status` as `partial` instead of `joined`.

## Log lines

superwitness reads logs from VictoriaLogs with LogsQL (`POST /select/logsql/query`). A line
belongs to a run when either:

- its `run.id` field equals the run id; or
- its `trace_id` field is one of the run's trace ids, found as above.

Lines from any service match; there is no service list for logs. The fields superwitness reads
from each line:

| Field | Becomes |
|---|---|
| `_time` | `at` |
| `_msg` | `message` |
| `service.name` | `service` (`unknown` when absent) |
| `severity_text`, `severity_number` | `level` |
| `trace_id`, `span_id` | `trace_id`, `span_id` |
| `run.id` | `run_id` |

A line's level comes from `severity_text` when it names one, in any case: text starting with
`err`, or equal to `fatal` or `critical`, reads as `error`; text starting with `warn` as `warn`;
`info` and `debug` as themselves. Otherwise it comes from `severity_number`: 5–8 `debug`, 9–12
`info`, 13–16 `warn`, 17–24 `error`. A line with neither reads `unknown`. Filtering by level
matches `severity_number` in that range, or `severity_text` matching the level's name,
case-insensitively.

From these lines the run document takes:

- `log_count`: the number of matching lines, at every level;
- `errors`: the first 50 error-level lines, oldest first, each with `service`, `message`,
  `trace_id` and `at`.

The full list is paged through [the logs route](/use/read-a-run/#logs-one-page-at-a-time).

## Content stays out of telemetry

No prompt, message or tool content belongs in any span or log. Content stays in the product
that holds it, and telemetry points at it instead: the `attempt` and `turn` spans above carry
`acp.session_id` and a range of event sequence numbers, not the events. Verdicts follow the same
rule: their `evidence_refs` are span ids or `{session_id, seq_from, seq_to}` ranges, never the
evidence.

## superwitness's own telemetry

Set `SW_OTLP_ENDPOINT` to the collector's OTLP/HTTP address, for example
`http://127.0.0.1:4318`, and superwitness sends its own:

- **traces**, on the standard OTLP/HTTP traces path under that endpoint: spans for the HTTP
  requests it serves and the HTTP requests it makes to the products and engines;
- **metrics**, on the standard OTLP/HTTP metrics path, every 30 seconds, including the
  `superwitness.source.fetches` counter.

Its resource is `service.name` `superwitness`, with `service.version` set to the release.

`superwitness.source.fetches` counts each source read made while building a run document, with
exactly two attributes: `source` (`superpipeline`, `agentpod`, `traces`, `logs` or `errors`) and
`status` (`ok`, `unavailable`, `timeout`, `not_found` or `unauthorized`). No run, attempt,
station or fingerprint id is ever a label. Alert on `unavailable`, `timeout` and
`unauthorized` to see a source failing; `not_found` only means a source did not know a run.

superwitness propagates W3C trace context, so its spans for a request join the caller's trace
when the request carries `traceparent`. Its exporters drop data rather than block: the span
queue holds 2048 spans. Left unset, `SW_OTLP_ENDPOINT` sends nothing.

superwitness also writes JSON logs to standard output: startup, migration retries, and a warning
naming any source that refused its credential.
