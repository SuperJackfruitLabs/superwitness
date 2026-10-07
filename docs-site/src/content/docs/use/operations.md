---
title: Operations
description: Health, backups, upgrades, the database roles, the listen address and the commands, for running superwitness day to day.
---

superwitness is one binary and one Postgres database. It keeps no state on disk; telemetry
lives in the engines, and everything it reads from AgentPod and superpipeline stays there.

## The commands

| Command | Does |
|---|---|
| `superwitness serve` | runs the service; also what runs with no command |
| `superwitness migrate` | runs pending migrations once and exits |
| `superwitness rubric-add` | records a rubric version ([Verdicts](/use/verdicts/#rubrics)) |
| `superwitness version` | prints the version; `--version` and `-version` do the same |

`serve` exits 2 when its settings are missing or invalid: missing settings are all named in one
message, and any other invalid setting stops it at the first one found
([Configuration](/use/configuration/#validation)). It exits 1 when it cannot start for another reason, such as an unreadable secret file. It stops cleanly on
`SIGINT` or `SIGTERM`, giving requests in flight up to 10 seconds. It logs JSON to standard
output.

## Health

`GET /health` needs no token. It answers 200 whenever the process can answer at all, so a
monitor can tell "superwitness is down" from "a source is down". Each source's state is in the
body:

```json
{"sources":{"agentpod":"ok","logs":"ok","superpipeline":"ok","traces":"ok","verdicts":"ok"},"status":"ok","version":"v0.0.6"}
```

- `status` is always `ok`, and `version` is the running release.
- Each of `agentpod`, `logs`, `superpipeline` and `traces` is a `GET /health` against that
  product or engine: any 2xx is `ok`, 401 or 403 `unauthorized`, anything else `unavailable`. Each
  check gets 1 second, after which it reads `timeout`. The engine checks carry the
  `SW_TRACES_TOKEN_FILE` or `SW_LOGS_TOKEN_FILE` token when one is set.
- `verdicts` is a ping of superwitness's own Postgres. It reads `unavailable` until migrations
  have succeeded.

To alert on a source that keeps failing during real reads, use the
`superwitness.source.fetches` metric ([Sending telemetry](/use/telemetry/#superwitnesss-own-telemetry)).

## Migrations at start

`serve` answers at once and runs its migrations in the background, over
`SW_MIGRATE_DATABASE_URL`, else `SW_DATABASE_URL`. While they fail it retries, waiting 1 second
and doubling to at most 30, and logs each failure. Until they succeed:

- `/health` shows `verdicts` as `unavailable`;
- run documents show `sources.verdicts` as `unavailable`, and everything else in them as usual;
- `POST /v1/verdicts` answers 503 `store_unavailable`, which is retryable, and the
  `record_verdict` tool returns the same error.

A Postgres that is down never takes run documents with it.

## The two database roles

superwitness uses two Postgres roles, set up in [Install](/install/#postgres-roles):

- **`superwitness_owner`** owns the database and the tables, and is used only for migrations,
  through `SW_MIGRATE_DATABASE_URL`.
- **`superwitness_app`** is the runtime role, through `SW_DATABASE_URL`, with `SELECT` and
  `INSERT` on `verdicts` and `rubrics` and nothing else.

The split keeps the running service from rewriting verdicts through SQL: it does not own the
tables, so it cannot disable or drop the triggers that refuse `UPDATE`, `DELETE` and `TRUNCATE`.
It does not protect against a compromised process that holds the owner's password, so if
`SW_MIGRATE_DATABASE_URL` is in the service's settings file, the service holds that password.
To keep it out:

1. leave `SW_MIGRATE_DATABASE_URL` out of `/etc/superwitness/env`, and set it only in your own
   environment when you run `superwitness migrate`;
2. as the owner, grant the runtime role read access to the migration bookkeeping table:

   ```sql
   GRANT SELECT ON goose_db_version TO superwitness_app;
   ```

`serve` then runs its background migration pass as the runtime role. With that grant, the pass
succeeds when nothing is pending; without it, the pass always fails. When a migration is
pending it fails either way, and verdicts stay unavailable until you run `superwitness migrate`
as the owner.

## Upgrades

1. Download and verify the new release as in [Install](/install/#download-and-verify).
2. Run the new binary's migrations as the owner, before restarting:

   ```sh
   SW_MIGRATE_DATABASE_URL='postgres://superwitness_owner:<password>@127.0.0.1:5432/superwitness?sslmode=disable' \
     superwitness_<version>_linux_amd64/superwitness migrate
   ```

   It exits 0 when the schema is up to date, and non-zero with the error otherwise. It gives up
   after 60 seconds if Postgres does not answer.
3. Install the binary and restart: `sudo systemctl restart superwitness`.

A migration that adds a table needs a grant of its own for the runtime role: nothing is granted
through `ALTER DEFAULT PRIVILEGES`.

## Backups

superwitness's only state is plain Postgres: the `superwitness` database, with its `verdicts`
and `rubrics` tables. Back it up with the usual tools, for example as the owner role or the
Postgres superuser:

```sh
pg_dump -Fc -f superwitness.dump 'postgres://superwitness_owner@127.0.0.1:5432/superwitness'
```

Use the owner or the superuser: the runtime role cannot read the migration bookkeeping table
unless you granted it above.

The telemetry engines are not superwitness's to back up. Telemetry is lossy by design, sampled
and kept for a short time, and superwitness never treats a missing span as missing evidence.
Evidence that belongs to AgentPod or superpipeline stays in those products, not in
superwitness's database.

## The listen address

`SW_LISTEN` is the address `serve` binds. Left unset it is `:8790`, every interface. The
[Install](/install/#configure) guide sets `127.0.0.1:8790`; to reach it from other machines, bind
a private address of the host instead, or keep loopback and put a reverse proxy in front.

In fake mode (`SW_FAKE_SOURCES=1`) superwitness accepts development tokens, so it refuses to
start unless `SW_LISTEN` is a loopback address: `localhost`, a `127.x.x.x` address or `[::1]`,
with a port. Left unset in fake mode, it is `127.0.0.1:8790`.

A client has 5 seconds to send a request's headers.
