# Claims ledger — SuperWitness docs voice rollout

Status: draft-stage evidence for the 18-page voice rollout.

## Immutable revisions and scope

- PR base: `f3c03a2500512181fe23b368c63de2c962fa70e8` (merge of SuperWitness PR #84).
- Voice guide: `SuperJackfruitLabs/sketchbook@29690e4397467bcf92ea7ef7e88b4e89cffba88f`, `ideas/2026-11-14-docs-voice-guide-v1/README.md`.
- Reference rollout: `SuperJackfruitLabs/superdispatch`, merged PR #32; its published narrator and ledger shape were used as the standard, not as a source of wording.
- Scope: all 18 published pages under `docs-site/src/content/docs/` and this ledger.
- The existing `BentoHome` block and home-page layout are unchanged. Product code, site setup, package/configuration files, styles, components, assets and working notes are unchanged.
- Product evidence below is pinned to the PR base.

## Page-by-page voice review

All pages pass narrator, exactness, vocabulary/privacy, originality and claims. The page-kind-specific rows are below. No running gag was added: none of these pages is a numbered first-use tutorial. One-off asides stay in narrative prose.

| Page | Kind | Narrator and momentum | Action payoff | Exact and safety material |
|---|---|---|---|---|
| `index.mdx` | home/reference | pass — the prose below the unchanged Bento block now guides you through the three questions | not applicable — the block's existing actions and flow are unchanged | pass — component props, commands, routes, values and warning remain literal |
| `what-it-is.md` | concept | pass — shared `we` and direct `you` frame the boundary and the two ways to start | pass — fake mode ends with the development-run result | pass — field names, source statuses and rules remain literal |
| `concepts.md` | concept/reference | pass — the opening gives the vocabulary a reason and the close returns to the trust boundary | not applicable — no action-bearing steps | pass — field tables, enums, identifiers and limits are unchanged |
| `install.md` | install/task | pass — a sustained we/you narrator carries the host from binary to health response | pass — verification, roles, collector, secret, grants, start and fake-mode actions each state the result they buy | pass — commands, SQL, YAML, settings, warning/recovery prose and credentials remain literal |
| `use/operations.md` | operate/task | pass — the opening names the operating journey and later prose guides migrations, upgrades and backup | pass — owner-password split, upgrade, health check and dump each end in an operational consequence | pass — commands, SQL, statuses, timings and settings remain literal |
| `use/the-app.md` | task/concept | pass — direct guidance carries setup, sign-in, browsing and verdict recording | pass — secret creation and verdict recording each end with an observable result | pass — settings, routes, refusal reasons, limits and recovery table remain literal |
| `use/telemetry.md` | concept/reference | pass — direct address follows the boundary from OTLP input to joined evidence | not applicable — no action-bearing sequence | pass — query, attributes, service names, limits and metric labels remain literal |
| `use/verdicts.md` | task/reference | pass — the reader records, standards and corrects one judgement with a continuing narrator | pass — record, rubric and correction actions each state the stored or visible result | pass — command, JSON, tables, enums, limits and refusal codes remain literal |
| `use/configuration.md` | reference | pass — warm framing tells the reader how to use the exhaustive table | not applicable | pass — all setting names, defaults and validation rules remain literal |
| `use/canary.md` | task/reference | pass — narrator carries the acceptance-test purpose through run, alert proof and status | pass — every subcommand ends with the receipt or ledger state it creates | pass — commands, flags, settings, durations, exits and flow table remain literal |
| `use/with-agentpod-and-superpipeline.md` | concept/reference | pass — direct framing follows credentials, reads and status mapping across trust boundaries | not applicable | pass — routes, token fields, scopes, statuses and errors remain literal |
| `use/transcripts.md` | concept/reference | pass — direct opening moves from span to content while holding the evidence boundary | not applicable | pass — routes, fields, limits, codes, redaction and audit names remain literal |
| `use/read-a-run.md` | task/reference | pass — the same run is followed through app, HTTP, redirect and paged evidence | pass — document read, attempt redirect and paging state their useful result | pass — commands, response examples, fields, limits, order and errors remain literal |
| `build/contracts.md` | reference | pass — direct framing separates exact shape from explanatory concepts | not applicable | pass — all field/type tables and JSON remain unchanged |
| `build/api.md` | reference | pass — client-focused framing makes the literal route and error map easier to enter | not applicable | pass — route table, parameters, bodies, statuses and every error code remain literal |
| `build/run-registry.md` | task/reference | pass — shared narrator explains why reports and listings exist | pass — report and page actions end with `applied`, cursor and count consequences | pass — JSON, schema rules, fields, limits, codes and compatibility warning remain literal |
| `build/licences.md` | concept/reference | pass — direct framing separates shipped, build-only and adjacent software | not applicable | pass — component and licence tables remain literal |
| `build/mcp.md` | task/reference | pass — direct setup and closing connect tool discovery to the HTTP contract | pass — endpoint configuration ends with discovery of the six tool schemas | pass — JSON, tool names, arguments, defaults, results and errors remain literal |

Candour passes across the set: unknown versus zero, partial sources, append-only evidence, self-judgement refusal, fake-mode boundaries, unreleased run-registry fields, lossy telemetry, owner-role risk and external-source failures remain explicit. No private host, person, agent, team, credential, customer or private-message name was added. Loopback addresses, reserved example domains, public product names, setting names and synthetic identifiers are the only examples.

## Claims checked at the PR base

Each row covers every material command, route, setting, default, field, enum, limit, status, version and behavioural statement on the page. Where a page spans many exact tables, the listed implementation files and repository claims tests cover the complete surface rather than only the prose changed in this rollout.

| Page | Material checked | Base evidence |
|---|---|---|
| `index.mdx` | quick-start command and Go version; run join; source timeout/unknown semantics; rubric/verdict append-only and self-judgement; HTTP/MCP reads; v0.0.12 release wording | `go.mod:1-5`; `internal/join/document.go:29-42,126-178,207-222`; `internal/join/join.go:15-17,40-97,141-171,229-257`; `internal/verdicts/service.go:119-164,273-322`; `internal/api/server.go:67-103`; `internal/mcp/server.go:80-170`; quick-start result recorded under Real checks; deployment statement separately marked UNVERIFIED below |
| `what-it-is.md` | three-question join; source statuses and unknown values; telemetry/verdict ownership; source fallbacks; fake mode and required real-source settings | `internal/join/document.go:29-124,126-178`; `internal/join/join.go:87-171,193-247,299-365`; `internal/config/config.go:73-220`; `internal/source/fake/dev.go:1-200`; `internal/verdicts/migrate.go:1-200` |
| `concepts.md` | run/attempt/fingerprint/verdict fields; subject and standard enums; source names/statuses; unknown/count/cost/trace semantics; by-attempt redirect | `internal/source/runref.go:1-120`; `internal/join/document.go:12-124,126-223`; `internal/join/join.go:87-247,315-371`; `internal/verdicts/model.go:13-44`; `internal/verdicts/service.go:167-234,273-350`; `internal/api/handlers.go:82-90` |
| `install.md` | release assets/install paths; Postgres roles, migrations and grants; collector endpoints; every setting/default/requirement; secret-file modes; systemd command/health/fake mode | `scripts/install.sh:1-260`; `deploy/release/build.sh:1-200`; `deploy/env.example:1-80`; `deploy/superwitness.service:1-80`; `internal/config/config.go:16-220,239-347`; `cmd/superwitness/main.go:30-170`; `internal/verdicts/migrate.go:1-200`; migrations under `internal/verdicts/migrations/`; SQL blocks mechanically compared by `internal/docsclaims/docs_test.go:132-230` |
| `use/operations.md` | command list and exits; shutdown/header timeouts; health source mapping; migration retry/degraded behaviour; roles/grants; upgrade/migrate timeout; backup boundary; listen/fake-mode rules | `cmd/superwitness/main.go:30-116`; `internal/config/config.go:16-20,73-220,239-285`; `internal/api/health.go:12-54`; `internal/app/app.go:1-320`; `internal/verdicts/migrate.go:1-200`; `deploy/superwitness.service:1-80` |
| `use/the-app.md` | app enablement/settings; OAuth/PKCE and allowlist; session cookies/lifetimes/refresh/revocation; origin checks; pages and run/verdict behaviour; limits; edge-hidden MCP; failure states | `internal/config/config.go:33-71,130-220`; `internal/session/login.go:27-174,205-377,407-435`; `internal/session/manager.go:17-224`; `internal/session/store.go:1-120`; `internal/api/server.go:18-36,49-135`; `internal/ratelimit/limiter.go:1-160`; `web/src/lib/` and `web/src/routes/` component tests |
| `use/telemetry.md` | seven-day trace search; three services; Jaeger query/limits/full-trace join; trace attributes/status; LogsQL fields/levels/count/errors; content boundary; own OTLP paths, interval, queue and labels | `internal/source/traces/adapter.go:20-51,90-215,222-285`; `internal/source/logs/logsql.go:1-240`; `internal/source/logs/adapter.go:15-49`; `internal/source/errors/adapter.go:1-120`; `internal/join/join.go:229-257`; `internal/telemetry/telemetry.go:1-67`; contract fixtures/tests under `internal/contracts/` and `internal/source/` |
| `use/verdicts.md` | request/response fields; kinds/subjects/standards/values; limits; rubrics/scales; self-judgement; supersession; idempotency/retries; evidence refs; database-down behaviour | `internal/verdicts/model.go:13-82`; `internal/verdicts/validate.go:13-121`; `internal/verdicts/service.go:23-164,167-350,429-450`; `internal/verdicts/rubrics.go:1-180`; `internal/verdicts/scale.go:1-220`; `cmd/superwitness/main.go:119-170`; `internal/api/ops_verdicts.go:1-220` |
| `use/configuration.md` | every `SW_*` variable, required/default/conditional behaviour, URL/audience rules, booleans, durations, principal/source grammar, loopback, secret files and other-command scope | `internal/config/config.go:16-220,223-347`; `internal/app/app.go:1-320`; `cmd/superwitness/main.go:30-170`; exact environment-name validity and completeness checked by `internal/docsclaims/claims.go:1-180` and `internal/docsclaims/surface_test.go:340-395` |
| `use/canary.md` | ten flow definitions; real-run versus forced-alert proof; run/record-alert/status flags, exits, output and 70-minute bound; every `SWC_*` setting/default/duration and secret rule | `internal/canary/checks.go:1-360`; `internal/canary/run.go:1-300`; `internal/canary/cmd.go:16-185`; `internal/canary/config.go:13-114`; `internal/canary/tracker.go:1-260`; `internal/canary/cmd_test.go` and `internal/canary/checks_test.go` |
| `use/with-agentpod-and-superpipeline.md` | service-token exchange/cache/invalidation; AgentPod and SuperPipeline routes; status mapping/not-found bodies; bearer/plane token claims, scopes and key cache; fake tokens | `internal/source/agentpod/adapter.go:1-153`; `internal/source/superpipeline/adapter.go:1-76`; `internal/auth/token.go:1-220`; `internal/auth/verify.go:1-260`; `internal/auth/resolve.go:1-180`; `internal/config/config.go:33-68,73-220`; `internal/source/httpx.go:1-180` |
| `use/transcripts.md` | app views/evidence links; transcript scope; live AgentPod reads; redaction/truncation; audit fields; both routes, ranges, pagination, item kinds and no-store | `internal/api/transcripts.go:17-203,206-313,316-372`; `internal/api/transcript_audit.go:1-140`; `internal/source/agentpod/transcript.go:1-180`; `web/src/lib/run/transcript.ts`; `web/src/lib/components/TranscriptPanel.test.ts`; `web/src/lib/components/SpanPane.test.ts` |
| `use/read-a-run.md` | app/run document; run-ref grammar and not-found rule; links; by-attempt redirect/errors; span/log shapes, order, filters, limits/cursors and source errors | `internal/source/runref.go:1-120`; `internal/api/handlers.go:12-90`; `internal/api/ops.go:1-260`; `internal/api/cursor.go:1-160`; `internal/api/limits.go:1-100`; `internal/join/document.go:29-223`; `internal/source/traces/adapter.go:90-215`; `internal/source/logs/logsql.go:1-240` |
| `build/contracts.md` | every run/verdict field/type/enum; fallback values; order; fake example; run-report schema; contract fixture inventory | `internal/join/document.go:12-223`; `internal/join/join.go:87-371`; `internal/verdicts/model.go:13-59`; `internal/runs/report.go:18-346`; `internal/contracts/contracts.go:1-160`; fixtures and decoder tests under `internal/contracts/`; published schema equality checked by `internal/docsclaims/schema_test.go:13-21` |
| `build/api.md` | authentication; all `/v1`, health/auth/app/MCP routes; parameters, bodies, limits, cache headers and statuses; complete stable error-code table | `internal/api/server.go:18-136`; `internal/api/handlers.go:12-90`; `internal/api/runs.go:21-114`; `internal/api/transcripts.go:296-372`; `internal/api/errors.go:1-180`; `internal/api/ops*.go`; route and surface validity checked by `internal/docsclaims/docs_test.go:77-175` and completeness by `internal/docsclaims/surface_test.go:340-395` |
| `build/run-registry.md` | report fields/grammar/statuses/timestamps; 1–100 batch, 256 KiB and skew limits; reporter auth; newer-wins semantics; response/errors/schema; list filters/order/limits/counts/cursor/latest verdict | `internal/runs/report.go:18-38,41-172,175-346`; `internal/runs/store.go:10-56`; `internal/runs/pgstore.go:1-360`; `internal/api/runs.go:21-114`; `internal/contracts/run-report.schema.json`; `internal/runs/schema_test.go` and `internal/runs/report_test.go` |
| `build/licences.md` | release contents; linked/runtime/build-only dependencies; eight fonts; allowlist; adjacent upstream products and exclusions | `go.mod:1-80`; `web/package.json`; `NOTICE`; `LICENSE`; `Makefile:1-180`; `deploy/release/build.sh:1-200`; `internal/installscript/install_test.go`; licence check invoked by `.github/workflows/ci.yml:83-101` |
| `build/mcp.md` | stateless transport/auth; server identity/version; six tools; HTTP-only routes; every tool argument/default/result; structured/text output and errors | `internal/mcp/server.go:1-85,87-214`; `internal/api/server.go:67-95,106-125`; `internal/mcp/server_test.go`; tool-name and HTTP-route validity checked by `internal/docsclaims/docs_test.go:132-175`; completeness checked by `internal/docsclaims/surface_test.go:340-395` |

## Corrections and evidence-bound changes

No factual change was made silently.

| Old material | New material | Why and evidence |
|---|---|---|
| `use/canary.md` described the canary as proving “ten flows on a real run” and said every flow was proven on a real run. | The description and opening now say flows 1–9 are proven on real runs, while flow 10 is proven by a forced failure that reaches the alert channel. | `internal/canary/cmd.go:52-120` shows `--force-fail` creates no card and calls no product; `internal/canary/cmd.go:123-160` records flow 10 only after the alert wrapper confirms the forced failure was accepted and stored. |

## Deployment claim that remains UNVERIFIED

- **UNVERIFIED:** `index.mdx` says “v0.0.12 is the release in production.” Repository release files and `CHANGELOG.md` prove that v0.0.12 is a release and the docs consistently pin it, but this public repository cannot prove what private infrastructure is currently running. A request for pinned private-estate file:line evidence was posted on card `card_ce8bc71da89a4a53`. The statement is not treated as false merely because the live deployment is outside this repository.

## Mechanical claims ratchet

The existing `internal/docsclaims` suite already checks both directions of the relevant public surface:

- every `SW_*` variable, `/v1` route and MCP tool named by a page exists in code;
- every route, setting (`SW_*`, `SWC_*` and installer variables), CLI command/flag, MCP tool and log event defined by code is named in docs or held in the shrinking baseline;
- every page has title and description;
- the run-report schema copy and install SQL blocks match their sources;
- tracked files contain no prohibited private infrastructure identifiers.

No ratchet code changed. This rollout adds no product fact or public-surface item, and the operator asked this branch to change page text only so it remains on the docs-only CI path. The correction above is already pinned by canary command tests; changing implementation tests would be out of scope.

## Exact-material inventory

- `index.mdx`: unchanged `BentoHome`/`FlowSteps` props, command, routes, settings, values and warning.
- `install.md`, `use/operations.md`: commands, SQL, YAML, settings, paths, versions, modes, health JSON, warnings and recovery.
- `use/the-app.md`, `use/telemetry.md`, `use/verdicts.md`, `use/canary.md`, `use/read-a-run.md`: commands, routes, settings, fields, enums, limits, response bodies, refusal/recovery text and tables.
- `use/configuration.md`, `use/with-agentpod-and-superpipeline.md`, `use/transcripts.md`: complete setting/route/status/scope/token/audit tables and literal examples.
- `concepts.md`, `build/contracts.md`, `build/api.md`, `build/run-registry.md`, `build/licences.md`, `build/mcp.md`: field, type, route, code, schema, component/licence and tool tables plus every JSON/command example.

No humour was added to an exact region, warning or recovery instruction.

## Real checks

- Published quick-start command, `go test -v ./internal/join/ ./internal/verdicts/`: pass — both packages passed; a separate uncached JSON run counted **55 top-level tests passed**, matching the home-page note.
- Docs-only claims lane, `go test ./internal/docsclaims/...`: pass — `ok github.com/SuperJackfruitLabs/superwitness/internal/docsclaims` in 1.896 s.
- Docs-only site lane, `npm ci && npm run build` in `docs-site`: pass — 350 packages added, 351 audited; npm reported 5 dependency vulnerabilities (2 moderate, 3 high); Astro built **19 pages**; Pagefind indexed 19 HTML files; sitemap generated.
- The successful site build emitted the existing warnings that the optional `i18n` collection is empty and that the bundled `use astro:head-inject` directive may not be preserved. Neither warning failed the build or points to page prose changed by this rollout.
- Rendered-copy assertions: pass — the changed home, canary, verdict and run-reading passages occur in the generated HTML/plain-text outputs.
- Scope/whitespace audit: `git diff --check` passed. Before the ledger commit, the changed-path list contained exactly the 18 page files and `docs/reviews/2026-11-15-docs-voice-superwitness-rollout.md`; no product code, configuration, site setup, asset, component, style, package file or `press/` working note changed.
- Privacy guard: pass as part of `go test ./internal/docsclaims/...`; the tracked-tree internal-reference test accepted the synthetic examples and found no prohibited private identifier.
