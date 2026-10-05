---
title: With AgentPod and superpipeline
description: What superwitness reads from AgentPod and superpipeline, how it authenticates to them, and the tokens its own callers present.
---

In this release a run document is for a superpipeline run, with its attempts read from the
AgentPod hub. superwitness reads both products over their APIs, as a service principal of the
AgentPod hub, and owns none of what it reads.

Neither product requires superwitness. They send plain OTLP to the collector and never wait on
superwitness, and superwitness only ever reads from them.

## The service credential

superwitness authenticates to both products with one AgentPod service credential:

| Setting | Holds |
|---|---|
| `SW_HUB_URL` | the AgentPod hub, for example `https://hub.agentpod.dev` |
| `SW_HUB_CLIENT_ID` | the credential's id, `svc_…` |
| `SW_HUB_CLIENT_SECRET_FILE` | a file holding the credential's secret alone, readable by its owner only |

Create the credential at the hub as a service principal for superwitness with the
`evidence:read` scope. superwitness refuses to start if the secret file is empty, or readable by
group or others.

To read, superwitness exchanges the credential for a short-lived token:

```
POST {SW_HUB_URL}/api/auth/service-token
Authorization: Bearer <SW_HUB_CLIENT_ID>:<secret>
```

The hub answers `{"token": …, "expiresIn": <seconds>}`. superwitness keeps the token until 30
seconds before it expires, and uses the same token for every request to the hub and to
superpipeline. If the hub refuses the credential (400, 401 or 403), the sources that need it
report `unauthorized`. If a product answers 401 or 403, superwitness drops the token and
exchanges again on the next read.

## What superwitness reads from AgentPod

From the hub at `SW_HUB_URL`, with the service token:

| Request | For |
|---|---|
| `GET /api/evidence/runs/superpipeline/{run}` | the run's ledger: its attempts, each attempt's fingerprint and agent principal, and the dispatch outcome |
| `GET /api/evidence/attempts/{attempt}` | which run an attempt belongs to, for `GET /v1/runs/by-attempt/{attempt}` and attempt verdicts |
| `GET /api/evidence/principals/{id}` | a principal's kind, to show a gate's judge and judge kind; `{id}` is a `prn_…` id or a hub user id |
| `GET /health` | `/health`'s `agentpod` status (no token) |

And, without a token, the hub's published keys at `GET /api/auth/jwks`, to verify the tokens
callers present (below).

A run the ledger knows on a different board from the one asked for counts as not found.

## What superwitness reads from superpipeline

From superpipeline at `SW_SUPERPIPELINE_URL`, for example `https://app.superpipeline.dev`, with
the same service token:

| Request | For |
|---|---|
| `GET {SW_SUPERPIPELINE_URL}/v1/boards/{board}/runs/{run}/evidence` | the run, its card, stage and state, its agent, its gates, and its usage |
| `GET /health` | `/health`'s `superpipeline` status (no token) |

When superpipeline cannot answer, the run document falls back to what the AgentPod ledger knows
about the run ([Concepts](/concepts/#run)).

## How answers become statuses

For both products:

| Answer | Source status |
|---|---|
| 200 with a body that parses | `ok` |
| 401 or 403 | `unauthorized` |
| 404 with the product's own not-found error | `not_found` |
| any other 404, or any other status | `unavailable` |
| no answer within `SW_SOURCE_TIMEOUT` | `timeout` |
| an answer about a different run | `unavailable` |

The product's not-found error is `{"error": "not_found"}` from the hub, and an error whose
`code` is `RUN_NOT_FOUND` or `BOARD_NOT_FOUND` from superpipeline. A bare 404, such as a route
that is not deployed, is `unavailable`, so a missing route never reads as a missing run.

## Tokens callers present

Every `/v1` route and `/mcp` take a bearer token issued by the same AgentPod hub. superwitness
verifies it offline, against the keys from `{SW_HUB_URL}/api/auth/jwks`, and accepts it only
when:

- it is an EdDSA (Ed25519) JWT whose key id is among the hub's published keys;
- its issuer (`iss`) is `SW_HUB_URL`;
- its audience (`aud`) includes `SW_PUBLIC_URL`;
- it has an expiry (`exp`) that has not passed and an issued-at time (`iat`), with 30 seconds of
  clock leeway;
- it names a subject (`sub`) and a `tenant`, and its `principalKind` is `human`, `agent` or
  `service`.

Both URLs are compared as configured, less any trailing `/`.

If the token has a `scope` claim, superwitness reads it as a space-separated list. Only
`POST /v1/runs` needs a scope today: `runs:write`, from a service principal
([Run registry API](/build/run-registry/#who-may-report)). A `scope` claim that is not a string
makes the token invalid.

Every such principal is admitted; which principals can get a token for superwitness's audience
is decided at the hub. The principal kind decides how its verdicts are recorded: their
`judge_kind`, their default `kind`, and whether it may judge work it executed
([Verdicts](/use/verdicts/#subjects-and-who-may-judge-them)).

superwitness keeps the hub's keys for 10 minutes. A token with an unknown key id makes it fetch
the keys again, at most once every 10 seconds. If the hub cannot be reached, keys it already
holds keep working.

A missing or refused token is 401 `unauthenticated`, with
`WWW-Authenticate: Bearer realm="superwitness"`:

```json
{"error":{"code":"unauthenticated","message":"the bearer token is not valid for this service"}}
```

With no token at all, the message is `a hub-issued bearer token is required`.

In fake mode (`SW_FAKE_SOURCES=1`) none of this applies: superwitness reads no product and
accepts development tokens of the form `dev:<principal>:<human|agent|service>` instead
([Install](/install/#try-it-without-the-other-products)).
