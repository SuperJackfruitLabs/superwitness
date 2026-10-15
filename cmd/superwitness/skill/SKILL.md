---
name: superwitness-ops
description: Use when running, migrating or checking a superwitness server, recording rubrics with rubric-add, or running its canary. Covers the superwitness command line (serve, migrate, rubric-add, canary, version, skill), the environment each verb needs, and which verbs change data.
---

# superwitness operations

Written for superwitness {{VERSION}}.

## What it is

superwitness is a SERVER binary. It reads runs from superpipeline and attempts from AgentPod
(plus telemetry from VictoriaTraces and VictoriaLogs), joins them, and records verdicts against
rubrics in its own Postgres. People and agents use it through its HTTP API (`/v1/...`, `/mcp`) and
its web app, not through this command line. The CLI exists to run, migrate and check the server.

`suwi` is a short alias for `superwitness` (a symlink); every command below works with either name.
The installer puts the binary and the alias in `~/.local/bin`; it installs no service. A server host
uses `deploy/superwitness.service` and `deploy/env.example` from the repository.

## Commands

| Command | What it does | Environment it needs |
|---|---|---|
| `superwitness version` (or `--version`) | Prints the version tag. | none |
| `superwitness skill` | Prints this file. | none |
| `superwitness skill install [--dir DIR]` | Writes `DIR/superwitness-ops/SKILL.md` (default DIR `~/.claude/skills`) and prints the path. | none |
| `superwitness serve` (also the default with no verb) | Runs the long-lived server. | `SW_DATABASE_URL`; unless `SW_FAKE_SOURCES=1` all of `SW_PUBLIC_URL`, `SW_SUPERPIPELINE_URL`, `SW_HUB_URL`, `SW_TRACES_URL`, `SW_LOGS_URL`, plus its way of signing in to the other products: either the organization-plane settings (`SW_ORG_PLANE_*`, including `SW_ORG_PLANE_SERVICE_CREDENTIAL_FILE`) or `SW_HUB_CLIENT_ID` with `SW_HUB_CLIENT_SECRET_FILE` (a path, never the secret itself); optional `SW_LISTEN`, `SW_MIGRATE_DATABASE_URL` |
| `superwitness migrate` | Applies the verdict schema migrations once, then exits. | `SW_MIGRATE_DATABASE_URL`, else `SW_DATABASE_URL` |
| `superwitness rubric-add -id ID -version N -name NAME -scale JSON -body-file FILE -created-by prn_...` | Records a rubric version (append-only; a change is a new version). | `SW_DATABASE_URL` (and `SW_MIGRATE_DATABASE_URL` if set, for migrations) |
| `superwitness canary run --trigger nightly\|manual --results PATH [--force-fail] [--result-out PATH]` | End-to-end check of the whole chain. | `SW_SUPERPIPELINE_URL`, `SW_TRACES_URL`, `SW_LOGS_URL`, `SWC_SUPERWITNESS_URL`, `SWC_BOARD_ID`, plus `SW_HUB_URL`, `SWC_HUB_CLIENT_ID`, `SWC_HUB_CLIENT_SECRET_FILE` (or `SW_ORG_PLANE_URL`, `SWC_ORG_PLANE_SERVICE_CREDENTIAL_FILE`, `SWC_SUPERWITNESS_AUDIENCE`) |
| `superwitness canary record-alert --results PATH --ntfy-id ID --query TEXT` | Appends proof that a forced failure's alert was delivered. | none beyond the flags |
| `superwitness canary status --results PATH [--json]` | Reads the results file and prints the state. | none beyond the flags |

`SW_FAKE_SOURCES=1` serves one development run with development tokens and refuses any
`SW_LISTEN` that is not loopback; it is for trying the server, never for a real host.
Settings that name a secret take a FILE path (`..._FILE`), not the secret itself.

## Safety

- **Read-only:** `version`, `skill`, `canary status`. `skill install` writes one file under the
  directory you name and nothing else.
- **`serve`** starts a long-running server and holds a port. Do not start it in the foreground of
  a task; a service manager runs it.
- **`migrate` and `rubric-add` change the verdict database.** Verdicts are records people and agents
  relied on, so schema changes and new rubric versions need the operator's approval. Never run
  either against production unasked, and never against a database whose DSN you were not given for
  this task. Rubrics are append-only: there is no edit or delete, so a wrong rubric stays.
- **`canary run`** writes synthetic data into live systems: it dispatches a real card on the board
  named by `SWC_BOARD_ID` in superpipeline (so an agent does real work), plants random marker
  strings in that card's spec, scans traces and logs for them, may record a verdict through
  superwitness's own API, and appends one JSON line to the `--results` file. With `--force-fail`
  it touches no live system and only appends a deliberate failure to the results file. Run it
  only when asked.
- **Never print or paste DSNs, tokens, client secrets or the contents of a `..._FILE`.** Report
  which variable was missing, not what it held. A DSN contains a password.

## Checking a server

`GET /health` needs no auth and answers 200 while the process is alive; its body lists the status
of each source. `superwitness version` on the host says which build is installed.
