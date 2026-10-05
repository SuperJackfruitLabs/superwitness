---
title: Install
description: Install superwitness on one Linux host with systemd, next to Postgres, VictoriaTraces, VictoriaLogs and the OpenTelemetry Collector.
---

This guide puts everything on one Linux host: the superwitness binary as a systemd service, its
Postgres database, the two telemetry engines it queries, and the collector that feeds them.
Everything in it, superwitness included, listens on loopback, so nothing here is reachable from
outside the host until you [choose to expose it](#configure).

To look around first without the other products, skip to
[Try it without the other products](#try-it-without-the-other-products).

## Requirements

- **Linux on amd64 or arm64, with systemd.** Releases ship a tarball for each.
- **Postgres.** superwitness's own tests run against Postgres 17. Postgres 15 and later need no
  schema statements beyond the two blocks below; 14 or older needs
  [two more](#postgres-14-or-older).
- **VictoriaTraces** and **VictoriaLogs**, which hold the telemetry superwitness reads.
- **The OpenTelemetry Collector**, which receives OTLP from your products and writes it to both
  engines.

superwitness itself needs no disk: it reads over HTTP and writes only to its Postgres.

## Download and verify

Each release has a tarball per architecture and a `SHA256SUMS` file covering them:

```sh
curl -LO https://github.com/SuperJackfruitLabs/superwitness/releases/download/v0.0.1/superwitness_0.0.1_linux_amd64.tar.gz
curl -LO https://github.com/SuperJackfruitLabs/superwitness/releases/download/v0.0.1/SHA256SUMS
sha256sum --ignore-missing -c SHA256SUMS
tar xzf superwitness_0.0.1_linux_amd64.tar.gz
```

On arm64, replace `amd64` with `arm64`. The tarball holds the binary, `LICENSE`, `NOTICE`,
`deploy/superwitness.service` and `deploy/env.example`. Install the binary:

```sh
sudo install -m 0755 superwitness_0.0.1_linux_amd64/superwitness /usr/local/bin/
superwitness version
```

## Postgres roles

Verdicts are append-only: triggers refuse `UPDATE`, `DELETE` and `TRUNCATE` on the `verdicts` and
`rubrics` tables. A table's owner can disable or drop those triggers, so the running service must
not own the tables. superwitness therefore uses two roles:

- **`superwitness_owner`** owns the database and the tables. superwitness connects as it, through
  `SW_MIGRATE_DATABASE_URL`, only to run migrations.
- **`superwitness_app`** is the runtime role, through `SW_DATABASE_URL`. It can insert and read
  verdicts and rubrics and nothing else.

Run this as the Postgres superuser (for example `sudo -u postgres psql`) before the first start.
Passwords, or peer or certificate authentication, are yours to choose; set them now, for example
with `\password superwitness_owner` and `\password superwitness_app` in `psql`.

<!-- sql:provision -->
```sql
CREATE ROLE superwitness_owner LOGIN;
CREATE ROLE superwitness_app LOGIN;
CREATE DATABASE superwitness OWNER superwitness_owner;
REVOKE ALL ON DATABASE superwitness FROM PUBLIC;
GRANT CONNECT ON DATABASE superwitness TO superwitness_app;
```

### Postgres 14 or older

On Postgres 14 or older, schema `public` belongs to the bootstrap superuser and every role may
create tables in it. There, after the block above and before you [migrate](#migrate), run this as
the superuser, connected to the `superwitness` database, so the owner role owns the schema and the
runtime role cannot create tables:

```sql
ALTER SCHEMA public OWNER TO superwitness_owner;
REVOKE CREATE ON SCHEMA public FROM PUBLIC;
```

Postgres 15 and later need neither statement: `public` is owned by the database owner and
`PUBLIC` has no `CREATE` on it.

## The telemetry engines

Install VictoriaTraces and VictoriaLogs from their upstream releases; superwitness uses them as
released and only through their published query APIs. Their own documentation covers installing
them as services: [VictoriaTraces](https://docs.victoriametrics.com/victoriatraces/) and
[VictoriaLogs](https://docs.victoriametrics.com/victorialogs/). For this guide, each needs a data
directory and a loopback address:

```sh
victoria-traces-prod -storageDataPath=/var/lib/victoria-traces -httpListenAddr=127.0.0.1:10428
victoria-logs-prod -storageDataPath=/var/lib/victoria-logs -httpListenAddr=127.0.0.1:9428
```

Then run the [OpenTelemetry Collector](https://opentelemetry.io/docs/collector/) with an OTLP
receiver and one exporter per engine. The core distribution has both. A minimal configuration:

```yaml
receivers:
  otlp:
    protocols:
      http: { endpoint: 127.0.0.1:4318 }
exporters:
  otlphttp/traces:
    traces_endpoint: http://127.0.0.1:10428/insert/opentelemetry/v1/traces
  otlphttp/logs:
    logs_endpoint: http://127.0.0.1:9428/insert/opentelemetry/v1/logs
service:
  pipelines:
    traces: { receivers: [otlp], exporters: [otlphttp/traces] }
    logs: { receivers: [otlp], exporters: [otlphttp/logs] }
```

Your products send OTLP over HTTP to `127.0.0.1:4318`. Which spans and log lines superwitness
finds for a run is covered in [Sending telemetry](/use/telemetry/).

## Configure

Create the service user and its settings file. The file is readable by root and the
`superwitness` group only:

```sh
sudo useradd -r -s /usr/sbin/nologin superwitness
sudo install -d -m 0750 -o root -g superwitness /etc/superwitness
sudo install -m 0640 -o root -g superwitness superwitness_0.0.1_linux_amd64/deploy/env.example /etc/superwitness/env
```

Edit `/etc/superwitness/env`. These are the settings `deploy/env.example` holds, with this guide's
addresses filled in:

```sh
SW_LISTEN=127.0.0.1:8790
SW_PUBLIC_URL=https://superwitness.example.internal
# Runtime role: SELECT and INSERT on verdicts and rubrics only, never the table owner.
SW_DATABASE_URL=postgres://superwitness_app:<password>@127.0.0.1:5432/superwitness?sslmode=disable
# Owner role, used only for migrations (serve's background migrations and `superwitness migrate`).
SW_MIGRATE_DATABASE_URL=postgres://superwitness_owner:<password>@127.0.0.1:5432/superwitness?sslmode=disable
SW_SUPERPIPELINE_URL=https://app.superpipeline.dev
SW_HUB_URL=https://hub.agentpod.dev
SW_HUB_CLIENT_ID=svc_REPLACE_ME
SW_HUB_CLIENT_SECRET_FILE=/etc/superwitness/hub-client-secret
SW_TRACES_URL=http://127.0.0.1:10428
SW_LOGS_URL=http://127.0.0.1:9428
# SW_TRACES_TOKEN_FILE=/etc/superwitness/traces-token
# SW_LOGS_TOKEN_FILE=/etc/superwitness/logs-token
SW_SOURCE_TIMEOUT=2s
# SW_OTLP_ENDPOINT=http://127.0.0.1:4318
```

- `SW_LISTEN` is the address superwitness binds. This guide keeps it on loopback; left unset it
  is `:8790`, every interface. To let other machines reach it, either bind it to a private or
  tailnet address of this host (`SW_LISTEN=<that address>:8790`) or keep it on loopback and put
  a reverse proxy in front.
- `SW_PUBLIC_URL` is the address callers reach superwitness at. Tokens sent to it must name it as
  their audience.
- `SW_SUPERPIPELINE_URL`, `SW_HUB_URL`, `SW_HUB_CLIENT_ID` and `SW_HUB_CLIENT_SECRET_FILE` are
  where superwitness reads runs and attempts, and the AgentPod service credential it signs in
  with.
- Outside fake mode, `serve` refuses to start unless every one of these is set:
  `SW_DATABASE_URL`, `SW_PUBLIC_URL`, `SW_SUPERPIPELINE_URL`, `SW_HUB_URL`, `SW_HUB_CLIENT_ID`,
  `SW_HUB_CLIENT_SECRET_FILE`, `SW_TRACES_URL` and `SW_LOGS_URL`. Its error names each one
  missing.
- `SW_OTLP_ENDPOINT` is optional: where superwitness sends its own traces and metrics. The
  collector configuration above has no metrics pipeline, so add one before setting it.

The hub client secret goes in a file of its own, holding the secret alone. superwitness refuses
to read it if group or others can:

```sh
sudo install -m 0600 -o superwitness -g superwitness /dev/null /etc/superwitness/hub-client-secret
sudoedit /etc/superwitness/hub-client-secret
```

## Migrate

Create the schema as the owner role. `superwitness migrate` reads only `SW_MIGRATE_DATABASE_URL`
(else `SW_DATABASE_URL`), runs every pending migration once, and exits 0, or non-zero with the
error. It gives up after 60 s if Postgres does not answer:

```sh
sudo sh -c 'set -a; . /etc/superwitness/env; set +a; superwitness migrate'
```

Then grant the runtime role, connected to the `superwitness` database as `superwitness_owner`
(for example `psql "postgres://superwitness_owner@127.0.0.1:5432/superwitness"`):

<!-- sql:runtime-grants -->
```sql
GRANT USAGE ON SCHEMA public TO superwitness_app;
GRANT SELECT, INSERT ON TABLE verdicts, rubrics TO superwitness_app;
```

That is the whole runtime grant set. The runtime role cannot update, delete, truncate, alter or
drop the tables or their triggers, and cannot create tables. Nothing is granted through
`ALTER DEFAULT PRIVILEGES`, so a later migration that adds a table needs a grant of its own.

The settings file above keeps the owner's DSN in the service's environment, so the running
process holds the owner password. To keep it out, leave `SW_MIGRATE_DATABASE_URL` only in your
own environment for `superwitness migrate`, run that before each upgrade, and also run
`GRANT SELECT ON goose_db_version TO superwitness_app;` as the owner. `serve` then runs its
background migration pass as the runtime role, which succeeds when nothing is pending.

## Start

```sh
sudo install -m 0644 superwitness_0.0.1_linux_amd64/deploy/superwitness.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable --now superwitness
curl -s localhost:8790/health
```

The unit runs `superwitness serve` as the `superwitness` user with its settings from
`/etc/superwitness/env`, and restarts it if it exits. `/health` needs no token and answers 200
whenever the process is alive; each source's state is in the body:

```json
{"sources":{"agentpod":"ok","logs":"ok","superpipeline":"ok","traces":"ok","verdicts":"ok"},"status":"ok","version":"v0.0.1"}
```

A source that cannot be reached shows `unavailable` or `timeout` there, and `/health` stays 200.
`serve` starts answering at once and runs its migrations in the background, retrying while
Postgres is unreachable; until they succeed, `verdicts` reads `unavailable`.

## Try it without the other products

With `SW_FAKE_SOURCES=1`, superwitness serves one development run, `brd_01/run_01`, from built-in
fixtures, and accepts development tokens of the form `dev:<principal>:<human|agent|service>`.
Because it accepts those, it refuses to listen on anything but loopback, and it needs only a
database:

```sh
SW_FAKE_SOURCES=1 SW_DATABASE_URL='postgres://<user>:<password>@127.0.0.1:5432/<database>?sslmode=disable' superwitness serve
curl -H 'Authorization: Bearer dev:prn_human01:human' localhost:8790/v1/runs/superpipeline/brd_01/run_01
```

Without `SW_MIGRATE_DATABASE_URL`, migrations run over `SW_DATABASE_URL`, which must then own the
database. That suits a scratch database for development, not the install above.

Next: [Concepts](/concepts/) explains what is in the document that comes back.
