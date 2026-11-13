---
title: The canary
description: superwitness canary, the live acceptance test that proves ten flows on a real run, with its subcommands, flags and SWC_* settings.
---

`superwitness canary` is SuperWitness's live acceptance test. It creates one card on a
SuperPipeline board, waits for an agent to run it, and then checks, as a black-box client of
every product involved, that the run can be observed end to end. Each check is one **flow**;
the canary is accepted once each of the ten flows has been proven on a real run.

It is meant to run on the SuperWitness host, nightly from a timer and by hand when needed. It
keeps its history in a JSON-lines file, the **tracker**, which it appends one result to per
run.

## The ten flows

| Flow | Proves |
|---|---|
| 1 | The run document has the run, its card, the work stage and its gates, from SuperPipeline's run evidence route |
| 2 | The AgentPod ledger has the run's attempts, each with a fingerprint whose harness is `SWC_EXPECT_HARNESS` |
| 3 | The run's trace is joined, with the hub's dispatch, attempt and turn spans in it |
| 4 | The agent's node continued the trace the hub started |
| 5 | A SuperPipeline API span carries the run's `run.id` |
| 6 | The gate on `SWC_GATE_STAGE` is shown pending, and a later run sees it decided by a human who is not the run's agent |
| 7 | `record_verdict` over MCP records a verdict, and a retry with the same idempotency key returns the same one |
| 8 | A synthetic error line, sent to `SWC_OTLP_LOGS_URL` with the run's id, shows up in the run's logs and errors |
| 9 | Two markers planted in the card's prompt reach neither VictoriaLogs nor VictoriaTraces |
| 10 | A failure reaches your alert channel (see [`record-alert`](#canary-record-alert)) |

Flow 6 needs a person to decide the gate, so it is usually proven by a later run than the one
that saw the gate pending. The canary looks back at up to seven earlier runs for it.

## The commands

### `canary run`

```sh
superwitness canary run --trigger nightly --results /var/lib/superwitness/canary.jsonl
```

| Flag | Meaning |
|---|---|
| `--trigger` | `nightly` or `manual` (the default) |
| `--force-fail` | record a deliberate failure without creating a card or calling any product, to prove flow 10 |
| `--results` | the tracker to append the result to; required |
| `--result-out` | also write this run's result, as indented JSON, to this file |

It prints one line, `canary PASS …` or `canary FAIL …`, and exits 0 on a pass, 1 on a
failure, and 2 when its flags are wrong or the tracker cannot be read. A result that cannot be
appended to the tracker counts as a failure. The whole run is bounded at 70 minutes.

### `canary record-alert`

```sh
superwitness canary record-alert --results /var/lib/superwitness/canary.jsonl --ntfy-id <id> --query '<how you checked>'
```

Your alerting wrapper calls this after the alert channel (ntfy) has accepted and stored the
message for a forced failure. It records flow 10's proof. All three flags are required, and the
last result in the tracker must be a forced failure; otherwise it records nothing and exits 2.

### `canary status`

```sh
superwitness canary status --results /var/lib/superwitness/canary.jsonl
```

Prints which flows have been proven, when, and on which run, with the last run's result. With
`--json` it prints the same summary as one JSON object.

Flags may be written with one dash or two, as with every Go flag set.

## Settings

The canary reads its own environment. Names it shares with the service keep the `SW_` prefix
and mean the same thing ([Configuration](/use/configuration/)); canary-only names start with
`SWC_`. `deploy/canary/canary.env.example` in the repository is a starting file.

| Variable | Required | Default | Meaning |
|---|---|---|---|
| `SW_SUPERPIPELINE_URL` | yes | none | SuperPipeline, where the canary creates its card |
| `SWC_SUPERWITNESS_URL` | yes | none | SuperWitness itself, for `/v1` and `/mcp`; usually loopback |
| `SW_TRACES_URL`, `SW_LOGS_URL` | yes | none | VictoriaTraces and VictoriaLogs, for flows 5 and 9 |
| `SW_TRACES_TOKEN_FILE`, `SW_LOGS_TOKEN_FILE` | no | none | bearer tokens for the engines |
| `SWC_BOARD_ID` | yes | none | the SuperPipeline board the canary's cards run on |
| `SW_HUB_URL` | without the plane | none | the AgentPod hub |
| `SWC_HUB_CLIENT_ID` | without the plane | none | the canary's own hub service credential id, `svc_…` |
| `SWC_HUB_CLIENT_SECRET_FILE` | without the plane | none | a file holding that credential's secret |
| `SW_ORG_PLANE_URL` | no | none | the organization plane; setting it replaces the three hub settings with the two below |
| `SWC_ORG_PLANE_SERVICE_CREDENTIAL_FILE` | with the plane | none | a file holding the canary's plane service credential, `svc_<id>:<secret>` |
| `SWC_SUPERWITNESS_AUDIENCE` | with the plane | none | SuperWitness's `SW_PUBLIC_URL`, the audience of its tokens (not `SWC_SUPERWITNESS_URL`) |
| `SWC_OTLP_LOGS_URL` | no | `http://127.0.0.1:4318/v1/logs` | the collector's OTLP/HTTP logs endpoint, for flow 8's synthetic error line |
| `SWC_WORK_STAGE` | no | `work` | the board stage the card is created on and an agent claims |
| `SWC_GATE_STAGE` | no | `review` | the board stage whose gate flow 6 watches |
| `SWC_EXPECT_HARNESS` | no | `hermes` | the harness flow 2 expects in each attempt's fingerprint |
| `SWC_STANDARD` | no | `rubric:superwitness-canary@1` | the standard flow 7's verdict names; must be `rubric:<id>@<version>` |
| `SWC_CLAIM_TIMEOUT` | no | `10m` | how long an agent has to claim the card |
| `SWC_RUN_TIMEOUT` | no | `30m` | how long the run has to end completed |
| `SWC_SETTLE_DELAY` | no | `90s` | how long to wait after the run ends, for telemetry to arrive |
| `SWC_DOC_TIMEOUT` | no | `5m` | how long to keep fetching the run document until it is complete |
| `SWC_POLL_INTERVAL` | no | `15s` | the wait between polls |

The five durations are Go durations and must be positive. A missing required setting, or a
duration or `SWC_STANDARD` that does not parse, is recorded as a failed run whose error starts
`setup:`, and `canary run` exits 1.

Like the service, the canary refuses a secret file that is empty or readable by group or others.
Every call it makes has a timeout, so one hung product cannot keep it running past its bound.
