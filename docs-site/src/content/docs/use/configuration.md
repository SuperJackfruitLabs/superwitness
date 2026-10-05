---
title: Configuration
description: Every environment variable superwitness reads, with its default.
---

superwitness reads its settings from environment variables; three of them name files to read
secrets from. Under systemd they come from `/etc/superwitness/env` ([Install](/install/#configure)). Leading and trailing spaces
are trimmed from every value.

## Settings

"Normal" is `serve` reading real sources; "fake" is `serve` with `SW_FAKE_SOURCES=1`.

| Variable | Required (normal / fake) | Default | Meaning |
|---|---|---|---|
| `SW_LISTEN` | no / no | `:8790`; in fake mode `127.0.0.1:8790` | the address `serve` binds. In fake mode it must be a loopback address. |
| `SW_PUBLIC_URL` | yes / no | in fake mode `http://` + `SW_LISTEN` | the address callers reach superwitness at. Caller tokens must carry it as their audience. |
| `SW_DATABASE_URL` | yes / yes | none | the Postgres DSN for the runtime role. |
| `SW_MIGRATE_DATABASE_URL` | no / no | `SW_DATABASE_URL` | the Postgres DSN migrations run over: the owner role. |
| `SW_SUPERPIPELINE_URL` | yes / no | none | superpipeline, where runs, cards, gates and usage are read. |
| `SW_HUB_URL` | yes / no | none | the AgentPod hub: attempts, principals, the service token, and the issuer of caller tokens. |
| `SW_HUB_CLIENT_ID` | yes / no | none | superwitness's AgentPod service credential id, `svc_…`. |
| `SW_HUB_CLIENT_SECRET_FILE` | yes / no | none | a file holding that credential's secret. |
| `SW_TRACES_URL` | yes / no | none | VictoriaTraces. |
| `SW_TRACES_TOKEN_FILE` | no / no | none | a file holding a bearer token to send to VictoriaTraces. |
| `SW_LOGS_URL` | yes / no | none | VictoriaLogs. |
| `SW_LOGS_TOKEN_FILE` | no / no | none | a file holding a bearer token to send to VictoriaLogs. |
| `SW_SOURCE_TIMEOUT` | no / no | `2s` | how long each source has to answer, as a Go duration such as `2s` or `1500ms`; it must be positive. |
| `SW_OTLP_ENDPOINT` | no / no | none: superwitness sends no telemetry of its own | the OTLP/HTTP endpoint for superwitness's own traces and metrics ([Sending telemetry](/use/telemetry/#superwitnesss-own-telemetry)). |
| `SW_FAKE_SOURCES` | no / – | `false` | `1` or `true` serves the built-in development run and accepts development tokens ([Install](/install/#try-it-without-the-other-products)). Accepts `true`/`false`/`1`/`0`; empty is false. |

In fake mode, the product, engine and credential settings are not used: the sources are built
in. A URL among them that is set is still validated. `SW_OTLP_ENDPOINT` and `SW_SOURCE_TIMEOUT`
still apply.

## Validation

`serve` checks its settings before it starts and exits 2 with a message on the first problem it
finds:

- **Missing settings.** Every required setting that is unset is named in one message, for
  example `missing required settings: SW_PUBLIC_URL, SW_HUB_URL`.
- **URLs.** `SW_PUBLIC_URL`, `SW_SUPERPIPELINE_URL`, `SW_HUB_URL`, `SW_TRACES_URL`, `SW_LOGS_URL`
  and `SW_OTLP_ENDPOINT`, when set, must be absolute `http` or `https` URLs with a host. A
  trailing `/` is removed.
- **`SW_SOURCE_TIMEOUT`** must parse as a positive duration.
- **`SW_FAKE_SOURCES`** must parse as a boolean.
- **Fake mode** refuses a `SW_LISTEN` that is not loopback.

Secret files are checked as `serve` starts. `SW_HUB_CLIENT_SECRET_FILE`, `SW_TRACES_TOKEN_FILE`
and `SW_LOGS_TOKEN_FILE` must each hold a non-empty value (surrounding whitespace is trimmed) and
must not be readable by group or others; otherwise `serve` exits 1 naming the file.

## Other commands

- `superwitness migrate` reads only `SW_MIGRATE_DATABASE_URL`, else `SW_DATABASE_URL`, and exits
  2 if neither is set.
- `superwitness rubric-add` needs `SW_DATABASE_URL`, and migrates first over
  `SW_MIGRATE_DATABASE_URL` when that is set.
