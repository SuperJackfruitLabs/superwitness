# Changelog

All notable changes to superwitness are recorded here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

## 0.0.3 — unreleased

superwitness has a web app. People on an allowlist sign in with AgentPod, browse the runs
sources report, read a run and record verdicts against rubrics.

### Added

- **The app**, served from the binary: runs grouped by day with Needs verdict, Failed, Waiting
  and per-board views; a run view with a span waterfall, logs, errors, verdict history and
  attempts; a drawer that records and revises verdicts for each rubric scale; rubric pages;
  light and dark themes; a phone layout. Fraunces, IBM Plex Sans and IBM Plex Mono are bundled,
  and their SIL Open Font License 1.1 texts ship in `licenses/fonts/` in the repository and in
  every release tarball.
- **Sign-in with AgentPod** (`/auth/login`, `/auth/callback`, `/auth/logout`): PKCE, a signed
  ten-minute login cookie (`__Host-sw_login`), and superwitness's own sessions
  (`__Host-sw_session`; 12 hours at most, 2 hours idle) in a new `sessions` table. Only
  principals on `SW_ALLOWED_PRINCIPALS` may sign in. Settings: `SW_APP_CLIENT_ID`,
  `SW_ALLOWED_PRINCIPALS`, `SW_SESSION_SECRET_FILE`, `SW_TRUSTED_PROXIES`. `SW_PUBLIC_URL` must be
  https when sign-in is on. Refusals are logged as `auth.signin_refused` with a reason
  (including `login_missing` and `no_code`). `POST /auth/logout` can answer 503
  `store_unavailable`, with the cookie cleared.
- Changes made with a session must carry the app's own `Origin` (403 `origin_mismatch`).
- **Rate limits**: `/auth/*` per client address, session changes and run reports per principal;
  429 `rate_limited` with `Retry-After`.
- `/mcp` answers 404 to requests that arrive through the public edge (from a trusted proxy,
  with `CF-Connecting-IP`), so the MCP tools stay private when the app is published.
- `GET /v1/me`, `GET /v1/verdicts` (one subject's verdict history) and `GET /v1/scopes`.
- An end-to-end test (`make e2e`, Playwright against Chromium) and its CI job. Playwright
  (Apache-2.0) and jsdom (MIT) are test-only npm dependencies and are never bundled.

### Changed

- A person's bearer token is resolved to their `prn_` principal id, so a person is the same judge
  whether they use the app or a token. Verdicts recorded with a person's token before 0.0.3 keep
  the account id as their judge.
- `/mcp` and `POST /v1/runs` refuse session cookies.
- The token-paste run page is gone; the app replaces it.

### Upgrading

- Migration `00003` adds the `sessions` table. After `superwitness migrate`, grant the runtime
  role as the owner: `GRANT SELECT, INSERT, UPDATE, DELETE ON TABLE sessions TO superwitness_app;`.
- The app is off until `SW_ALLOWED_PRINCIPALS` names someone; it then needs an https
  `SW_PUBLIC_URL`, `SW_APP_CLIENT_ID` and `SW_SESSION_SECRET_FILE`.

## 0.0.2 — 2026-10-05

superwitness keeps its own list of runs. Sources report runs to a published contract, and
people and agents list them with filters, counts and each run's latest verdict.

### Added

- **Run registry.** `POST /v1/runs` takes one run report or a batch of up to 100 from a service
  principal whose token carries `runs:write`, bound to its source by `SW_RUN_SOURCES`. A report
  updates its run only when its `reported_at` is newer than the stored one. The body is described
  by a JSON Schema, `internal/contracts/run-report.schema.json`, also published on the docs site.
- **`GET /v1/runs`** lists runs newest first, filtered by source, scope, status, executor, time
  and `needs_verdict`, with per-status counts, cursor paging and each run's latest verdict.
- **`list_runs`** MCP tool, with the same filters.
- **Rubric reads:** `GET /v1/rubrics` and `GET /v1/rubrics/{id}/{version}`, each with the scale
  recognised as `decision`, `score`, `label` or `text` (or `null`).
- `superwitness rubric-add` warns when a scale has none of the recognised shapes.
- Tokens' `scope` claim is read; development tokens take an optional fourth segment of scopes.

### Upgrading

- Migration `00002` adds the `runs` table. After `superwitness migrate`, grant the runtime role
  as the owner: `GRANT SELECT, INSERT, UPDATE ON TABLE runs TO superwitness_app;`. Until then
  the run registry answers 503 `store_unavailable`; verdicts are unaffected.
- Set `SW_RUN_SOURCES` for each reporter. Unset, nobody may report.

## 0.0.1 — 2026-10-05

First public release. superwitness builds run documents that join superpipeline runs with AgentPod
attempts, traces and logs, and keeps grader and eval verdicts in its own database.

### Added

- **Run API (HTTP, `/v1`).** `GET /v1/runs/superpipeline/{board}/{run}` returns a run document;
  `…/spans` and `…/logs` page through its spans and log lines (cursor, `limit`, and a `level`
  filter for logs); `GET /v1/runs/by-attempt/{attempt}` redirects an attempt to its run (302);
  `POST /v1/verdicts` records a verdict. `GET /health` needs no auth, stays 200 while the process
  is alive, and reports the status of each source in its body.
- **MCP server** (streamable HTTP at `/mcp`) with four tools: `get_run`, `list_run_spans`,
  `list_run_logs` and `record_verdict`. The by-attempt lookup is HTTP-only.
- **Verdict store.** Verdicts and rubrics live in superwitness's own Postgres database and are
  append-only: triggers refuse `UPDATE`, `DELETE` and `TRUNCATE`, so a rubric change is a new
  version. `superwitness rubric-add` records a rubric version.
- **Evidence join** over superpipeline (runs), AgentPod (attempts), VictoriaTraces (spans, through
  the Jaeger API) and VictoriaLogs (log lines, through LogsQL), matched on the run id and the
  run's trace ids. An unavailable source shows as such in the run document and in `/health`
  instead of failing the request.
- **Embedded run page** at `/runs/superpipeline/{board}/{run}`, served from the binary.
- **Separate database roles.** A `superwitness migrate` subcommand runs migrations as the table
  owner (`SW_MIGRATE_DATABASE_URL`); the running service connects as a runtime role
  (`SW_DATABASE_URL`) that can insert and read verdicts and rubrics and nothing else.
- **Fake mode** (`SW_FAKE_SOURCES=1`) serves one development run on loopback, with `dev:` tokens,
  for trying superwitness without the other products.
- **Release tarballs** for linux/amd64 and linux/arm64, each with the binary, `LICENSE`, `NOTICE`,
  a systemd unit and an example env file, plus a `SHA256SUMS` file. Tag-triggered, and the
  binary's `--version` reports the tag.
- **Canary** (`superwitness canary`) that checks the acceptance flows against a running
  deployment.
- **Websites.** [superwitness.dev](https://superwitness.dev) (`landing/`) and the documentation
  site, [docs.superwitness.dev](https://docs.superwitness.dev) (`docs-site/`), with CI builds
  and Worker configs. Docs-claims tests check the pages against the code's own registries of
  `SW_*` variables, `/v1` routes and MCP tools.
- A guard test that fails when a tracked file contains internal identifiers.
