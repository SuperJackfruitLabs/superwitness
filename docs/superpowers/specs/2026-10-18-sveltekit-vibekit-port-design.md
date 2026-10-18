# Superwitness web app on SvelteKit and vibekit: design

Date: 2026-10-18. Status: approved by the operator (scope 2026-10-18: "restyle into the four vibes, same behaviour", phone included; mockups approved the same day).

## Intent

The Superwitness console (`web/`) is a React 19 + Vite single-page app with its own dark theme. Every other product (Superpipeline, Superlibrary, Accounts) now runs on **vibekit**: four user-selectable vibes (Daylight, Paper, Studio "Annual report", Quiet), each in light and dark, with a selectable accent. Superwitness joins them. The app is rewritten in **Svelte 5 on SvelteKit** (static single-page mode) so it can use vibekit's components, and restyled to the approved mockups. **Behaviour does not change.**

## Source of truth for the look

The design canvas "Vibes Product Mockups" (https://claude.ai/artifact/KabncsLYMYiTxSkSgs75YH), page "Superwitness", the rows titled "Restyle · …" (26 rows x 4 vibes, desktop and phone). Local copies of the artboards are in `design/mockups/` of the vibekit repo's `design/mockups/superwitness/` once merged; until then the canvas is authoritative. The earlier boards on the same page (the "queue" design with search, "Judge next", per-board counts, agent faces) are **not** the target: they need features the app does not have (see Non-goals).

## Goals

1. Every page and state in INVENTORY.md renders in all four vibes, light and dark, at 1440, 390 and 360 wide, with no horizontal page scroll and no axe violations.
2. Identical behaviour: same URLs and query-string state, same API calls, same copy, same keyboard behaviour. The existing Playwright e2e specs (`web/e2e/app.spec.ts`, `transcript.spec.ts`) pass unchanged except for selectors that name a removed element (each such change is listed in the PR).
3. Still one Go binary: the SvelteKit build is static and embedded from `internal/web/dist`; no Node server in production; `make web-check` still proves the committed `dist` matches the source.
4. The Content-Security-Policy stays strict: no `'unsafe-inline'` for scripts.
5. Fix the defects in DEFECTS.md that the mockups fix (see "Defects fixed on the way").

## Non-goals (anti-goals)

- No new product features: no search or ⌘K, no "Judge next" or queue, no per-board counts, no agent faces, no filters beyond today's, no rubric editing, no live updates, no charts. (Written up as a follow-up issue; the earlier "queue" mockups are its reference.)
- No change to the Go API, the data model, or the CLI. The only Go change is the CSP (below).
- No change to `landing/` or `docs-site/` (Astro on Cloudflare).
- Not a rewrite of logic: pure modules (format, filters, group, facts, transcript, scale, sources, messages, api) move over unchanged with their tests.

## Constraints (from the server and the repo)

- CSP today: `default-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; frame-ancestors 'none'` plus `X-Content-Type-Options: nosniff`, on every page including the Go-rendered sign-in pages (`internal/web/web.go` `CSP`, reused by `internal/session`). Inline styles are allowed; inline scripts are not. Fonts and images are same-origin or `data:`.
- SPA fallback: any GET without a file extension that is not `/v1/*`, `/mcp*`, `/auth/*`, `/health` returns `index.html`; a missing path with an extension is 404. `/favicon.ico` redirects to `/favicon.svg`.
- SvelteKit static output writes an **inline bootstrap `<script>`** into `index.html`. The Go handler therefore adds that script's `sha256` hash to `script-src` at startup by reading the embedded `index.html` (see plan Task 1). vibekit's inline `HEAD_SCRIPT` is NOT used; `appearance-head.js` is served as a file from `/assets` (vibekit ships it) so the first paint still gets the person's vibe.
- The app never sees a token; cookies are `HttpOnly` sessions. Sessions expire at 2 h idle / 12 h absolute, so any call can 401.
- Dependencies: vibekit is consumed from a vendored tarball (`web/vendor/superjackfruit-vibekit-0.0.13.tgz`, `file:` dependency), as Superpipeline does. Package manager stays npm (`web/package-lock.json`).
- Public repository: names are fine, secrets never.

## Decisions taken (and what they cost if wrong)

| Decision | Why | Cost if wrong |
|---|---|---|
| SvelteKit with `adapter-static`, `fallback: "index.html"`, `ssr = false`, `prerender = false` | matches Superpipeline's web app; Go already does the fallback | none |
| Output goes straight to `internal/web/dist` through the adapter's `pages` option; committed as today | keeps `make web-check` and embed unchanged | low |
| Routes are SvelteKit file routes with param matchers that copy `router.tsx match()` exactly | same URLs, 404s for the same strings | low; route tests pin it |
| Data fetching is a small rune-based helper (`fetched()`, `pages()`), not `load` functions | the app is client-only and its states (loading, error, reload, generation guard) are already modelled in hooks; ports 1:1 | low |
| Appearance uses vibekit's appearance cookie and helpers; the old `localStorage["superwitness.theme"]` is read once to seed the first visit, then ignored | the four vibes and accent need vibekit's model | a person's old light/dark choice may reset once |
| `completed` is drawn in the success tone and words stay as the API sends them | the mockups; colour-only change | none |
| Warning and info tones are a local shim (`src/lib/tones.css`) until vibekit has them | vibekit 0.0.13 has danger only | remove the shim when vibekit adds them |
| New components that vibekit lacks live in `src/lib/ui/` (waterfall, inspector pane, score slider, level badge, redaction mark, key-value list, panel set) and are proposed to vibekit afterwards | not blocking on a vibekit release | moving them later is mechanical |
| Phone Menu stays an inline block under the topbar (today's behaviour), not an overlay | mockups | none |

## Defects fixed on the way (from DEFECTS.md and the mockups)

`completed` shown grey; raw `{"decision":null}` printed for pending/cancelled gates (now "pending" / "no decision yet" / "cancelled"); long agent id overflows at 390/360; span pane has no scrim and clips the Attributes table; deep link is lost on re-login after a mid-session 401 (the sign-in link carries the current path and query); tabs claim `role=tab` without arrow-key support (arrow keys, Home and End now work); raw ISO times shown (readable local time with the ISO in `title`); missing rubric shows "Something went wrong" (now "Rubric not found" with a link back). Each fix has a test.

## What is left to the builder

Component internals, file layout inside `src/lib/`, the exact CSS (taken from the mockup artboards), test structure. The builder may not change behaviour, copy (other than the listed defect fixes and the words the mockups add: column headers Time / Level / Service / Message, the Appearance control labels, "Rubric not found"), or URLs.
