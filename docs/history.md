# How superwitness was built

superwitness was built between 2026-10-04 and 2026-10-05 in a private predecessor of this repository. For the first public release, 0.0.1, that history was condensed into a single commit. This page keeps a readable account of it: the fifteen pull requests in number order, then every commit.

Pull request numbers below are historical labels from the private repository; they do not exist here. Dates are YYYY-MM-DD in UTC+05:30. A pull request's date is the day it was merged.

## Pull requests

### #1 — docs: design and implementation plans for the first release

Merged 2026-10-04.

Added the design for the first release and the implementation plans that split the work into parallel streams across superwitness, its sibling products and their deployment. The design fixes the core shape: a Go service that joins run evidence at read time from superpipeline, the AgentPod hub, VictoriaTraces and VictoriaLogs into one run document, keeps grader and eval verdicts append-only in its own Postgres database, and exposes the result through a `/v1` HTTP API, MCP tools and a small web page. It also settled the contracts between the products (for example, a hub endpoint that issues tokens to service principals) and recorded what was still open, such as harness version and model staying `unknown` until harnesses report them.

### #2 — docs: acceptance is every data flow proven; telemetry edge is otlp.agentpod.dev

Merged 2026-10-04.

Redefined acceptance for the first release: rather than a number of passing nights, ten named data flows each need one recorded canary run that demonstrates them (superpipeline evidence, hub attempts, spans from the hub, node agents and Workers, gate and grader verdicts, logs and errors linked to the run, no content in telemetry, and a forced failure reaching the alert channel). Once a flow has passed it counts as done; if it breaks afterwards, that is reported as a regression rather than undoing acceptance. The planned public OTLP endpoint for Cloudflare Workers moved to otlp.agentpod.dev.

### #3 — contracts: callers' tokens must name superwitness (SW_PUBLIC_URL) as their audience

Merged 2026-10-04.

superwitness accepts a caller's hub-issued token only when its `aud` claim names superwitness's own public URL (`SW_PUBLIC_URL`), so a token minted for another service cannot be replayed against it. The cross-product contract had not given any calling client that audience, which would have refused every canary and operator call; the fix was deploy-time configuration of the two calling clients, not code.

### #4 — Core service: run document join, verdict store, HTTP + MCP API, embedded run page

Merged 2026-10-04.

The core service as one Go binary. It joins the four sources into a run document with a two-phase parallel join in which a failing source never fails the document and a missing value shows as `unknown` rather than 0. Callers' hub JWTs are verified offline (EdDSA, audience check, a bounded key refetch while the hub is down). Verdicts are append-only, enforced by database triggers, because a verdict is a record of judgement: a correction is a new verdict that supersedes the old one, so nothing is lost. Recording rules refuse a reused idempotency key with a different body, refuse an agent judging a run it executed (failing closed when the executor is unknown), and require every verdict to cite a versioned standard. It serves the HTTP API, an MCP endpoint (`get_run`, `list_run_spans`, `list_run_logs`, `record_verdict`) and an embedded React run page.

### #5 — Live canary: per-flow proofs and first-release acceptance

Merged 2026-10-04.

Added `superwitness canary run|status|record-alert`, a live canary that proves each data flow end to end. Each run puts a test card through a throwaway board, checks the flows against the live deployment, and records a proof per flow in a JSON-lines tracker; `status` reports acceptance once all ten flows are proven. The no-content scan uses positive controls and treats truncated results as failures. Contract tests run the canary's real clients against the core service's handlers, and an integration test runs against real VictoriaTraces and VictoriaLogs. The canary keeps its own copy of the run-document types on purpose, so contract drift fails it.

### #6 — Release workflow and migration/runtime database role split

Merged 2026-10-04.

Made superwitness releasable: `--version`, `make dist` building static linux amd64/arm64 tarballs with a `SHA256SUMS` file, and a tag-triggered release workflow that checks the binary reports the tag. It also split the database roles: migrations run as the table owner (`SW_MIGRATE_DATABASE_URL`, and a new `superwitness migrate` command), while the service runs as a runtime role (`SW_DATABASE_URL`) that may only read and insert verdicts and rubrics, so it cannot update, delete or truncate them or disable the append-only triggers. Integration tests check that the runtime role is refused. Its documented limit: when the service is given both connection strings it holds the owner credentials too, so the split protects against mistakes and bad queries on the everyday connection, not against an attacker who controls the process.

### #7 — chore: the telemetry edge is otlp.superwitness.dev

Merged 2026-10-04.

Switched the public OTLP endpoint used by Cloudflare Workers to otlp.superwitness.dev, the product's own domain (replacing otlp.agentpod.dev), and updated the contract, the design, the canary docs and the canary's label for that flow.

### #8 — docs: canary card queue (card-create rule, queuing scope, setup)

Opened 2026-10-05; not merged.

A live check found that superpipeline refused the canary's service token when it tried to create a card. This documented the agreed fix: a separate hub scope for queuing cards and a per-board list of services allowed to queue, with the admin who lists a service owning the cards it queues. The code belonged to the sibling products. The pull request was never merged; its three documentation commits reached the main line through #9, which was built on top of it.

### #9 — docs: the canary model is under discussion

Merged 2026-10-05.

Recorded an open question about the canary: should it create its own synthetic runs, or verify the runs the agents already do? The options were compared flow by flow, and the canary docs said not to schedule the nightly run until it was decided.

### #10 — docs: move unbuilt and under-discussion ideas to the maintainers' private planning notes

Merged 2026-10-05.

Moved the canary-model discussion out of this repository into the maintainers' private planning notes, leaving a stub behind so existing links still resolved. Documentation only.

### #11 — docs: web sign-in and recent runs are on hold

Merged 2026-10-05.

Noted that browser sign-in and a list of recent runs were paused until their design was settled, with what already existed to build on.

### #12 — docs: website, docs site and going-public design

Merged 2026-10-05.

Wrote down how the project would present itself publicly: a product website at superwitness.dev, a documentation site at docs.superwitness.dev, and the steps for opening the repository without leaking anything private. A later change chose to publish the sites from the maintainer's machine using an interactive Cloudflare sign-in, so no deploy token is stored anywhere.

### #13 — docs: move internal specs, plans and discussions to the maintainers' private planning notes

Merged 2026-10-05.

Removed the internal specs, plans, discussions and the canary operator guide from the repository ahead of going public; they moved to the maintainers' private planning notes. No code referred to them.

### #14 — superwitness.dev and docs.superwitness.dev: landing, docs, docs-claims test, CI and deploy script

Merged 2026-10-05.

Added the two public sites: a one-page landing site whose hero is a real run document, and a Starlight docs site written from the code. A new docs-claims test checks every configuration variable, API route and MCP tool mentioned on the sites against what the code actually registers, and keeps the database-role SQL in the install guide identical to the README's. CI builds both sites, each site deploys as a Cloudflare Worker with static assets, and the README and deploy examples were cleaned up.

### #15 — Remove internal references, add a guard, prepare the second pre-release

Merged 2026-10-05.

Removed the remaining internal references from code comments, user-visible strings, docs and fixtures, and added an internal-reference guard: a test that scans every tracked file (this page included) and fails on internal identifiers, matching private names by digest so the guard itself does not publish them. It also bumped the version and added the changelog for the second internal pre-release.

## All commits

All 101 commits, oldest first, by commit date. Only the first line of each message is kept, reworded where it named internal systems or planning documents.

- 2026-10-04 — superwitness: observability and evaluation for agent fleets
- 2026-10-04 — docs: design and implementation plans for the first release (#1)
- 2026-10-04 — docs: acceptance is every data flow proven; telemetry edge is otlp.agentpod.dev
- 2026-10-04 — docs: acceptance is every data flow proven; telemetry edge is otlp.agentpod.dev (#2)
- 2026-10-04 — feat: scaffold superwitness module, config and CI
- 2026-10-04 — feat(auth): principal types and hub service-token client
- 2026-10-04 — feat(source): Source interface, RunRef, fragments and superpipeline/hub contract fixtures
- 2026-10-04 — feat(source): scripted fake source and development data set
- 2026-10-04 — feat(auth): offline hub JWT verification and caller middleware
- 2026-10-04 — fix(auth): bound JWKS refetches during an outage, detach shared fetch from caller ctx
- 2026-10-04 — fix(auth): join an in-flight JWKS fetch instead of failing against it
- 2026-10-04 — feat(source): superpipeline evidence adapter
- 2026-10-04 — feat(source): AgentPod hub adapter, attempt links, principal kinds
- 2026-10-04 — feat(source): VictoriaTraces adapter over the Jaeger query API
- 2026-10-04 — feat(source): VictoriaLogs LogsQL client and logs adapter
- 2026-10-04 — feat(source): error lines from VictoriaLogs
- 2026-10-04 — feat(verdicts): append-only Postgres store with migrations
- 2026-10-04 — feat(verdicts): recording rules: idempotency, self-judgement, standards, supersedes
- 2026-10-04 — fix(verdicts): refuse NUL in client input with 400, not a retryable 503
- 2026-10-04 — fix(verdicts): validate supersedes id and scan raw JSON tokens for NUL
- 2026-10-04 — feat(join): RunDocument, two-phase parallel join, principal mapping, subjects
- 2026-10-04 — feat(api): run document, paged spans and logs, by-attempt redirect
- 2026-10-04 — feat(api): POST /v1/verdicts and /health
- 2026-10-04 — feat(mcp): get_run, list_run_spans, list_run_logs, record_verdict over streamable HTTP
- 2026-10-04 — feat(web): embedded React run page over /v1
- 2026-10-04 — fix(web): render nulls as unknown, re-enter bad token, paginate logs
- 2026-10-04 — feat: wire superwitness serve with own telemetry, fake mode and rubric-add
- 2026-10-04 — test: end-to-end run document against containerised Victoria engines and Postgres
- 2026-10-04 — build: vet and licence-check integration-tagged code; unit tests skip test/
- 2026-10-04 — docs(deploy): systemd unit, env template, health-check entry, NOTICE
- 2026-10-04 — fix(app): serve while the verdict Postgres is down at start; fix metric server name
- 2026-10-04 — fix(verdicts): replay a concurrent identical supersede; refuse lone surrogates
- 2026-10-04 — test: make two review-requested tests discriminate
- 2026-10-04 — fix(web): forbid framing the run page (CSP frame-ancestors 'none')
- 2026-10-04 — contracts: callers' tokens must name superwitness (SW_PUBLIC_URL) as their audience
- 2026-10-04 — contracts: cancelled gates, legacy gate rule and usage caveat; token rotation
- 2026-10-04 — fix(join): show a cancelled gate as cancelled, never pending
- 2026-10-04 — Merge #3: contracts: callers' tokens must name superwitness (SW_PUBLIC_URL) as their audience
- 2026-10-04 — Merge #4: Core service: run document join, verdict store, HTTP + MCP API, embedded run page
- 2026-10-04 — canary: scratch board pipeline, stage instructions, env names
- 2026-10-04 — canary: config, markers, flow proofs and run-document types
- 2026-10-04 — canary: checks for flows 1-8 with failure injection
- 2026-10-04 — canary: telemetry scans with positive controls, Workers tag search; flow 9
- 2026-10-04 — canary: redact needle values from scan locations
- 2026-10-04 — canary: hub token, superpipeline, run API and logs, MCP and OTLP clients
- 2026-10-04 — canary: orchestration across nine flows, error injection, gate follow-up
- 2026-10-04 — canary: gate follow-up waits when agent or judge identity is unresolved
- 2026-10-04 — canary: tracker with per-flow proofs; acceptance when all ten flows are proven
- 2026-10-04 — canary: a cancelled-gate regression survives later passing runs until a passing resolution
- 2026-10-04 — canary: run/status/record-alert subcommands, production wiring, operator doc
- 2026-10-04 — canary: traces client timeout, 70-minute process deadline
- 2026-10-04 — canary: contract tests against the core service's handlers and real Victoria engines
- 2026-10-04 — canary: choose this run's gate, else the latest
- 2026-10-04 — canary: flow 6 counts only a gate tied to the run
- 2026-10-04 — canary: wire form always has checks:[], fail the run when it cannot be recorded; docs and test fixes
- 2026-10-04 — Merge #5: Live canary: per-flow proofs and first-release acceptance
- 2026-10-04 — Add --version flag, make dist, and tag-triggered release workflow
- 2026-10-04 — Split migration and runtime database roles; add superwitness migrate
- 2026-10-04 — README: give the owner role schema public on Postgres 14 or older
- 2026-10-04 — Final-review fixes: checkout credentials, 42501 checks, owner-DSN note, tar xattrs
- 2026-10-04 — chore: the telemetry edge is otlp.superwitness.dev
- 2026-10-04 — Merge #7: chore: the telemetry edge is otlp.superwitness.dev
- 2026-10-04 — Merge #6: Release workflow and migration/runtime database role split
- 2026-10-05 — docs: card-queue contract, queuing scope, canary setup steps
- 2026-10-05 — docs: canary card-queue review fixes (existing principal, live-check row, card-create detail)
- 2026-10-05 — docs: mark stale canary plan commands superseded
- 2026-10-05 — docs: record the canary model as under discussion
- 2026-10-05 — Merge #9: docs: the canary model is under discussion
- 2026-10-05 — docs: move unbuilt and under-discussion ideas to the maintainers' private planning notes
- 2026-10-05 — docs: web sign-in and a recent-runs page are on hold
- 2026-10-05 — Merge #11: docs: web sign-in and recent runs are on hold
- 2026-10-05 — docs: website, docs site and going-public design
- 2026-10-05 — spec: no API tokens; deploy with the maintainer's wrangler login
- 2026-10-05 — Merge #12: docs: website, docs site and going-public design
- 2026-10-05 — docs: move internal specs, plans and discussions to the maintainers' private planning notes
- 2026-10-05 — feat(landing): superwitness.dev, one page with the run document as its hero
- 2026-10-05 — fix(landing): agent row, 404 without canonical, light-tab favicon
- 2026-10-05 — feat(docs): docs.superwitness.dev scaffold, theme and Start pages
- 2026-10-05 — fix(docs): loopback listen, release requirements, empty lists and gate/sampled values
- 2026-10-05 — docs: Use it and Build on it pages, written from the code
- 2026-10-05 — test: check the docs' claims against the code's own registries
- 2026-10-05 — ci: build both sites on pull requests; Worker configs and the local deploy script
- 2026-10-05 — fix(docs): 405 on a wrong method, serve's exit-2 wording, request-message traceparent, engines in use
- 2026-10-05 — docs: README badges, links and family; strip internal references
- 2026-10-05 — Final-review fixes: true landing/docs claims, release wording, wider docsclaims scan
- 2026-10-05 — Merge #10: docs: move unbuilt and under-discussion ideas to the maintainers' private planning notes
- 2026-10-05 — Merge the main line (#10) into the internal-docs move
- 2026-10-05 — Merge #13: docs: move internal specs, plans and discussions to the maintainers' private planning notes
- 2026-10-05 — Merge the main line (#10, #13) into the website work
- 2026-10-05 — Merge #14: superwitness.dev and docs.superwitness.dev: landing, docs, docs-claims test, CI and deploy script
- 2026-10-05 — Remove internal planning pointers from Go code and canary output
- 2026-10-05 — docs: say the MCP server covers four of the five operations
- 2026-10-05 — Replace bare contract and plan ids in comments with descriptions
- 2026-10-05 — test: use an obviously fake executor id in canary fixtures
- 2026-10-05 — chore: drop an internal pointer from a test name and neutralise a fixture handle
- 2026-10-05 — test: guard tracked tree against internal identifiers
- 2026-10-05 — chore: bump the version for the second pre-release and add CHANGELOG
- 2026-10-05 — test: match private names by digest and add pointer families to the guard
- 2026-10-05 — docs: describe the product in the changelog and neutralise canary wording
- 2026-10-05 — changelog: date the second pre-release
- 2026-10-05 — Merge #15: Remove internal references, add a guard, prepare the second pre-release

## Pre-0.0.1 releases

Two internal pre-releases were tagged in the private repository before it went public. The public version line starts at 0.0.1; see the [changelog](../CHANGELOG.md) for what it contains.
