---
title: Transcripts
description: What an agent was asked, what it sent to each tool and what came back, read live from AgentPod, redacted, and never kept by superwitness.
---

A run's spans say where and when something happened. A transcript says what: the prompt an
attempt was given, what the agent sent to each tool, what came back, which permissions it asked
for and how they were answered. superwitness reads it live from AgentPod's session events, the
source of truth, for one attempt at a time.

**Content is evidence, never telemetry.** Traces and logs stay content-free. superwitness never
stores a transcript, never logs one and never lets one be cached: every answer carries
`Cache-Control: no-store`.

## In the app

- **Span details.** In the Trace tab, click a span's name. A pane opens on the right (the whole
  screen on a phone) with the span's timing, status, attributes and log lines. For an `attempt`,
  `turn`, `tool_call` or `permission` span it also shows the **Request** and **Response**: a tool
  call's input and output, a permission's question, options and outcome, or the prompt and the
  agent's reply. Content is read only when a span is opened.
- **The Transcript tab** shows the attempt as a conversation: prompt, agent message, reasoning
  (folded), tool calls (a failed call has a red edge and opens by itself), permissions (question,
  options, outcome), state changes, errors, and any other event by its type. 200 steps a page,
  with **Load more**. A run with several attempts has a picker.
- **Links both ways.** **Open in transcript ↗** in the pane goes to the same step;
  **span ↗** on a step goes back to the Trace tab with its span open. The address keeps the
  tab, the span, the attempt and the `seq`, so a link opens the exact moment.
- **Cite.** A span or a step can be cited as a verdict's evidence (a span id, or
  `{session_id, seq_from, seq_to}`), at most 100 per verdict. A verdict that cites a step links back
  to it.
- **When content is not there.** "Transcripts aren't available to this account" (the rest of the
  page still works), "Transcript unavailable, try again" (AgentPod could not be read), and
  "Attempt still running; refresh for more".

## Who may read content

- A person signed in to the app. Only principals on `SW_ALLOWED_PRINCIPALS` can sign in.
- A bearer token whose AgentPod-signed `scope` includes `transcripts:read`. Anyone else gets 403
  `transcripts_forbidden`, over HTTP and over MCP alike. `evidence:read` alone is not enough.

superwitness itself reads AgentPod with its own service credential, whose grant must hold
`transcripts:read` beside `evidence:read`. Without it, transcript reads answer 502 `hub_refused`
and everything else works as before.

## Redaction

AgentPod redacts every string in a transcript before it leaves the hub; superwitness shows what
it receives. A redacted value reads `[redacted:<rule>]`, and each step and page says how many
values were redacted. The rules are AgentPod's: the hub's own secrets, common credential formats
(API keys, tokens, private keys, `Authorization` values, credentials in URLs), JSON keys whose
names say they hold a secret, and any rules its operator adds. The stored events are never
changed.

A field longer than 16 KiB arrives cut, as `{"truncated": true, "bytes": N, "head": "…"}`.
**Show full** reads that one step whole (up to 1 MiB).

## Audit

Every read leaves two records, neither holding content:

- at AgentPod, an audit row naming superwitness, with the person or agent superwitness read for
  in `on_behalf_of` (superwitness sends their `prn_` id as `X-On-Behalf-Of`);
- in superwitness's log, one `transcript.read` line with `principal`, `via` (`session` or
  `bearer`), `run`, `attempt`, `seq_from`, `seq_to`, `item`, `full`, and on success `items` and
  `redactions`; a refused read logs at WARN with its `code`.

## The API

```
GET /v1/runs/superpipeline/{board}/{run}/transcript?attempt=&seq_from=&seq_to=&cursor=
GET /v1/runs/superpipeline/{board}/{run}/transcript/items/{seq_from}?attempt=&seq_from=&seq_to=&full=1
```

- `attempt` is required when the run has more than one attempt (400 `attempt_required`).
- The range must lie inside the attempt (400 `range_outside_attempt`); without one, the whole
  attempt is read. A running attempt has no end yet: the read goes to the session's current end.
- On the item route, `seq_from` and `seq_to` are optional: the range the step was shown in,
  which the app forwards on **Show full**. They must lie inside the attempt too.
- The answer is AgentPod's, unchanged, plus `"attempt_id"`:
  `{"attempt_id", "session_id", "seq_from", "seq_to", "items": […], "next_cursor", "redactions", "truncated_fields"}`.
  Pass `next_cursor` back as `cursor` for the next page.
- Each item has a `kind`: `prompt`, `message`, `reasoning`, `tool_call`, `permission`, `state`,
  `error` or `other`, with its seqs and fields; a tool call or permission that started before
  the range has `"partial": true`.

Agents use the MCP tool [`get_transcript`](/build/mcp/#get_transcript), with the same rules.
Every code is on the [HTTP API](/build/api/#every-code) page.
