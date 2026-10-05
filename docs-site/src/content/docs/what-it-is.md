---
title: What superwitness is
description: Observability and evaluation for agent fleets. Three questions answered on one run, and why telemetry and evidence are kept apart.
---

superwitness is observability and evaluation for agent fleets. For one run of work it shows what
an agent did, how the products it ran on behaved, and whether the work was any good, joined into
one record.

Generic observability tools stop at a trace. superwitness goes one step further: from a trace to
the run it belongs to, the attempts that executed it, the agent configuration that ran it, and
the verdicts that judged it. That is what turns "did this configuration get better?" into a
question you can answer.

## Three questions, one run

| Question | Where the answer comes from | In the run document |
|---|---|---|
| **What did it do?** | The attempts that executed the run, each with its configuration fingerprint, and the spans they left | `attempts`, `trace` |
| **How did the products run?** | Log lines and errors that carry the run's id or one of its trace ids | `log_count`, `errors` |
| **Was it any good?** | Gate decisions read from superpipeline, and verdicts recorded in superwitness | `verdicts` |

The run document is what `GET /v1/runs/superpipeline/{board}/{run}` returns. It is assembled at
read time from every source, and it says which sources answered: a source that is down shows as
`unavailable` or `timeout`, and the numbers it would have supplied show as `unknown`, never as 0.
[Concepts](/concepts/) has the vocabulary.

## Telemetry and evidence are different things

superwitness handles two kinds of data and keeps them apart:

| | Telemetry | Verdicts |
|---|---|---|
| What | Traces, metrics, logs | Grader scores, offline eval results, calibration runs, configuration comparisons |
| Loss | Expected: sampled, short retention | Never |
| Where | Upstream engines | superwitness's own database |

Telemetry lives in VictoriaTraces and VictoriaLogs, sent there through the OpenTelemetry
Collector. superwitness only queries it. Verdicts live in superwitness's own Postgres, where they
are append-only: database triggers refuse to update, delete or truncate them, and a change of
mind is a new verdict that supersedes the old one.

A missing span is not missing evidence. When a run has no spans, its trace status is `none` and
the rest of the document still stands.

Evidence that belongs to another product, such as a work run, a gate decision or a transcript,
stays in that product. superwitness reads it through that product's API and never becomes its
owner.

## The rules the project keeps

1. **It is meant to stand alone.** The design rule is that no other SuperJackfruit product
   should be required, so that anyone's agents can send OTLP and record verdicts. This release
   is not there yet: it reads runs from superpipeline and attempts from AgentPod, and without
   them it runs only in fake mode (below).
2. **Nothing requires it.** Products send plain OTLP and never wait on superwitness.
3. **Upstream engines, unforked.** superwitness queries VictoriaTraces and VictoriaLogs, fed by
   the OpenTelemetry Collector, through their published query APIs only. It is designed to sit
   beside VictoriaMetrics and Perses the same way: upstream releases, run as released, never
   forked.
4. **Permissive licences only** in anything linked or shipped: MIT, Apache-2.0, BSD. No AGPL, so
   no Grafana, Loki, Tempo or Mimir, and no source-available licences (ELv2, BSL, FSL, SSPL).
   Bundled components are credited in the `NOTICE` file in every release.
5. **Agents first.** It is evaluation and observability for agent fleets, with product health
   underneath. It is not a general monitoring platform that also handles agents.

## What this release reads

Run documents in this release are for superpipeline runs, addressed as
`superpipeline:<board>/<run>`, with attempts read from the AgentPod hub. Started for real,
superwitness asks for both products' URLs and an AgentPod service credential; to try it without
either, run it with `SW_FAKE_SOURCES=1`, which serves one development run on loopback. The
[install guide](/install/#try-it-without-the-other-products) shows both.
