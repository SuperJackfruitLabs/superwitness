# superwitness

[![CI](https://github.com/SuperJackfruitLabs/superwitness/actions/workflows/ci.yml/badge.svg)](https://github.com/SuperJackfruitLabs/superwitness/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/SuperJackfruitLabs/superwitness)](https://github.com/SuperJackfruitLabs/superwitness/releases)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue)](LICENSE)

**Observability and evaluation for agent fleets.** It shows what an agent did,
how the product it ran on behaved, and whether the work was any good, joined on
one run.

**Website:** [superwitness.dev](https://superwitness.dev) · **Docs:** [docs.superwitness.dev](https://docs.superwitness.dev) · **History:** [how it was built](docs/history.md)

superwitness is part of a family of self-hosted tools for working with agents.
[AgentPod](https://github.com/SuperJackfruitLabs/agentpod) manages the machines and runtimes
agents live in; [superpipeline](https://github.com/SuperJackfruitLabs/superpipeline) owns boards,
cards, runs and approval gates; [supermessage](https://github.com/SuperJackfruitLabs/supermessage)
is the Matrix client that puts agents in the conversation. superwitness records what happened and
whether it was any good.

> **Status: v0.0.1** ([changelog](CHANGELOG.md)). Run documents cover superpipeline runs, with attempts read from the
> AgentPod hub. To try it without either product, `SW_FAKE_SOURCES=1` serves one development run
> on loopback.

## What it is for

Generic observability tools stop at a trace. superwitness goes one step further:
from a trace to the run it belongs to, the attempts that executed it, the agent
configuration that ran it, and the verdicts that judged it. That is what turns
"did this configuration get better?" into a question you can answer.

It handles two kinds of data and keeps them apart:

| | Telemetry | Verdicts |
|---|---|---|
| What | Traces, metrics, logs | Grader scores, offline eval results, calibration runs, configuration comparisons |
| Loss | Expected: sampled, short retention | Never |
| Where | Upstream engines (below) | superwitness's own database |

Evidence that belongs to another product, such as a work run, a gate decision or
a transcript, stays in that product. superwitness reads it through that
product's API and never becomes its owner.

## Rules this repository keeps

1. **Meant to stand alone.** The design rule is that no other SuperJackfruit product
   should be required, so that anyone's agents can send OTLP and record verdicts. This
   release is not there yet: it reads runs from superpipeline and attempts from AgentPod,
   and without them it runs only in fake mode (`SW_FAKE_SOURCES=1`).
2. **Nothing requires it.** Products send plain OTLP and never wait on
   superwitness.
3. **Upstream engines, unforked.** superwitness queries VictoriaTraces and
   VictoriaLogs, fed by the OpenTelemetry Collector, through their published query
   APIs only. It is designed to sit beside VictoriaMetrics and Perses the same way:
   upstream releases, run as released, never forked.
4. **Permissive licences only** in anything linked or shipped: MIT, Apache-2.0,
   BSD. No AGPL, so no Grafana, Loki, Tempo or Mimir. No source-available licences
   (ELv2, BSL, FSL, SSPL). Bundled components are credited in [NOTICE](NOTICE).
5. **Agents first.** It is evaluation and observability for agent fleets, with
   product health underneath. It is not a general monitoring platform that also
   handles agents.

The docs site (`docs-site/`) and landing page (`landing/`) are checked against the
code by `internal/docsclaims`: a page that names an `SW_*` variable, `/v1` route or
MCP tool the code does not define fails `make test`.

## Running superwitness

```bash
make build
SW_FAKE_SOURCES=1 SW_DATABASE_URL=postgres://sw:sw@127.0.0.1:5432/superwitness?sslmode=disable ./bin/superwitness
curl -H 'Authorization: Bearer dev:prn_human01:human' localhost:8790/v1/runs/superpipeline/brd_01/run_01
```

Fake mode serves one development run (`brd_01/run_01`) and accepts `dev:<principal>:<human|agent|service>`
tokens. It refuses to listen on anything but loopback.

To try the run registry in fake mode, bind a development reporter and report a run:

```bash
SW_RUN_SOURCES=prn_reporter01=superpipeline SW_FAKE_SOURCES=1 SW_DATABASE_URL=… ./bin/superwitness &
curl -H 'Authorization: Bearer dev:prn_reporter01:service:runs:write' -d '{"source":"superpipeline",
  "external_ref":"brd_01/run_01","status":"running","source_status":"in_progress","reported_at":"2026-10-06T10:00:00Z"}' \
  localhost:8790/v1/runs
curl -H 'Authorization: Bearer dev:prn_human01:human' localhost:8790/v1/runs
```

A fourth segment of a development token lists its scopes, comma-separated.

| Surface | Path |
|---|---|
| Run document | `GET /v1/runs/superpipeline/{board}/{run}` |
| Spans, logs (paged) | `GET …/spans?cursor=&limit=`, `GET …/logs?cursor=&level=&limit=` |
| Attempt → run | `GET /v1/runs/by-attempt/{attempt}` (302) |
| Record a verdict | `POST /v1/verdicts` |
| Report runs (service principals with `runs:write`) | `POST /v1/runs` |
| List runs | `GET /v1/runs?source=&scope=&status=&executor=&since=&until=&needs_verdict=&cursor=&limit=` |
| Rubrics | `GET /v1/rubrics`, `GET /v1/rubrics/{id}/{version}` |
| MCP (streamable HTTP) | `/mcp`: `get_run`, `list_run_spans`, `list_run_logs`, `list_runs`, `record_verdict` |
| Run page | `/runs/superpipeline/{board}/{run}` |
| Health | `GET /health` (no auth; always 200 while alive, per-source status in the body) |

Rubrics are append-only: `superwitness rubric-add -id press -version 1 -name Press -scale '{"min":0,"max":1}' -body-file press.md -created-by prn_…`.

## Deploying

The [install guide](https://docs.superwitness.dev/install/) covers one Linux host with systemd:
the binary at `/usr/local/bin/superwitness`, `deploy/superwitness.service`, settings in
`/etc/superwitness/env` from `deploy/env.example`, the AgentPod service credential's secret in
`/etc/superwitness/hub-client-secret` (0600, the secret alone), and superwitness's own Postgres.

### Two database roles

Verdicts are append-only: triggers refuse `UPDATE`, `DELETE` and `TRUNCATE` on `verdicts` and
`rubrics`. A table's owner can disable or drop those triggers, so the running service must not
own the tables. superwitness takes two DSNs:

- `SW_MIGRATE_DATABASE_URL` connects as `superwitness_owner`, which owns the database and the
  tables. It is used only to run migrations.
- `SW_DATABASE_URL` connects as `superwitness_app`, the runtime role. It can insert and read
  verdicts and rubrics, insert, read and update registry runs, and nothing else.

If `SW_MIGRATE_DATABASE_URL` is unset, migrations run over `SW_DATABASE_URL`, which must then
be the owner. That suits development only. superwitness never logs either DSN.

The split stops SQL-level misuse through the runtime pool, not a compromised process: when
`SW_MIGRATE_DATABASE_URL` sits in the service env, the running process holds the owner
password. To keep it out, leave `SW_MIGRATE_DATABASE_URL` only in the operator's env for
`superwitness migrate`, run that before each upgrade, and also run
`GRANT SELECT ON goose_db_version TO superwitness_app;` as the owner. `serve` then runs its
background migration pass over `SW_DATABASE_URL` as the runtime role. With that grant, the pass
succeeds when nothing is pending. Without it, the pass always fails. When a migration is pending,
it fails either way, and the verdict store stays unavailable while the pass retries.
`TestRuntimeRoleMigrationPass` checks all three cases.

Provisioning, run as the Postgres superuser before the first start. Passwords, or peer or
certificate authentication, are set per deployment and are left out here:

<!-- sql:provision -->
```sql
CREATE ROLE superwitness_owner LOGIN;
CREATE ROLE superwitness_app LOGIN;
CREATE DATABASE superwitness OWNER superwitness_owner;
REVOKE ALL ON DATABASE superwitness FROM PUBLIC;
GRANT CONNECT ON DATABASE superwitness TO superwitness_app;
```

Then create the schema as the owner. `superwitness migrate` reads only `SW_MIGRATE_DATABASE_URL`
(else `SW_DATABASE_URL`), runs every pending migration once, and exits 0, or non-zero with the
error. It gives up after 60 s if Postgres does not answer:

```sh
set -a; . /etc/superwitness/env; set +a; superwitness migrate
```

Then grant the runtime role, connected to the `superwitness` database as `superwitness_owner`:

<!-- sql:runtime-grants -->
```sql
GRANT USAGE ON SCHEMA public TO superwitness_app;
GRANT SELECT, INSERT ON TABLE verdicts, rubrics TO superwitness_app;
GRANT SELECT, INSERT, UPDATE ON TABLE runs TO superwitness_app;
```

That is the whole runtime grant set. The tables have no sequences (keys are text), and the
runtime pool never reads goose's `goose_db_version` table, so it needs no grant on it (unless
serve migrates as the runtime role, above). Nothing is
granted through `ALTER DEFAULT PRIVILEGES`: a later migration that adds a table must add that
table's grant here. The runtime role cannot update, delete, truncate, alter or drop `verdicts`
and `rubrics` or their triggers; it can update registry rows in `runs` but never delete them;
and it cannot create tables. `TestRuntimeRoleCanAppendButNotRewrite` and
`TestRuntimeRoleRegistryGrants` run both blocks above verbatim against Postgres 17 and check
all of this.

Upgrading from 0.0.1: after `superwitness migrate`, run only the new line,
`GRANT SELECT, INSERT, UPDATE ON TABLE runs TO superwitness_app;`, as the owner. Until then the
run registry answers 503 `store_unavailable`; verdicts are unaffected.

On Postgres 14 or older, schema `public` belongs to the bootstrap superuser and every role may
create tables in it. There, after the provision block and before `superwitness migrate`, run
this as the superuser, connected to the `superwitness` database, so the owner role owns the
schema and the runtime role cannot create tables. Postgres 15 and later need neither statement:
`public` is owned by the database owner and `PUBLIC` has no `CREATE` on it.

```sql
ALTER SCHEMA public OWNER TO superwitness_owner;
REVOKE CREATE ON SCHEMA public FROM PUBLIC;
```

`superwitness rubric-add` follows the same split: it migrates over the migrate DSN and inserts
over `SW_DATABASE_URL`.

### Starting and health

`superwitness serve` connects its runtime pool with `SW_DATABASE_URL` only. It starts serving
at once and runs its migrations in the background over the migrate DSN, retrying with backoff (1 s doubling to 30 s) while its Postgres is unreachable. Until they succeed, run documents show `sources.verdicts: unavailable`, `POST /v1/verdicts` returns 503 `store_unavailable` (retryable) and `/health` reports `verdicts: unavailable`.

`/health` stays 200 when a source is down, so an outside monitor can tell "superwitness is down"
from "a source is down". A source outage shows in its body (`sources.<name>`) and in the
`superwitness.source.fetches{source,status}` metric, which an alerting rule can watch.

## Engine queries

superwitness uses only the engines' published query APIs. Lookback is 7 days.

- **VictoriaTraces** (Jaeger API), one request per service (`agentpod-hub`,
  `agentpod-node-agent`, `superpipeline-api`):
  `GET /select/jaeger/api/traces?service=<svc>&tags={"run.id":"<run>"}&start=<µs>&end=<µs>&limit=100`
- **VictoriaLogs** (LogsQL), `POST /select/logsql/query`, all queries prefixed `_time:7d`:
  - run filter `("run.id":="<run>" or trace_id:in("<trace>",…))`
  - level filter `(severity_number:range[17, 24] or severity_text:i("error"))` (error shown; debug 5-8, info 9-12, warn 13-16)
  - page `… | sort by (_time) | offset N | limit M`
  - count `… | stats count() as n`
  - errors list: run filter, error level filter, `| sort by (_time) | limit 50`

Log fields read: `severity_text`, `severity_number`, `trace_id`, `span_id`, `run.id`, `service.name`.

## Contributing and security

See [CONTRIBUTING.md](CONTRIBUTING.md) for how changes land and how to run the checks. Please report vulnerabilities privately, as described in [SECURITY.md](SECURITY.md).

## License

[MIT](LICENSE).
