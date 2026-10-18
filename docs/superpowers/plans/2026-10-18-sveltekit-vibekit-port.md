# Superwitness on SvelteKit and vibekit: implementation plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the React/Vite console in `web/` with a Svelte 5 / SvelteKit static app on vibekit 0.0.13, restyled to the approved mockups in all four vibes, with identical behaviour, still embedded in the Go binary.

**Architecture:** SvelteKit (`adapter-static`, `ssr = false`, `fallback: index.html`) builds into `internal/web/dist`. A tiny rune-based data layer replaces the React hooks; pure logic modules and their tests move over unchanged. vibekit supplies tokens, vibes, appearance and base components; components it lacks live in `src/lib/ui/`. The Go handler adds the SvelteKit bootstrap script's hash to `script-src` at startup so the CSP stays strict.

**Tech Stack:** Svelte 5 (runes), SvelteKit 2, `@sveltejs/adapter-static`, vibekit 0.0.13 (vendored tarball), vitest + `@testing-library/svelte` (jsdom), Playwright, `@axe-core/playwright`, Go 1.x (existing), npm.

**Spec:** `docs/superpowers/specs/2026-10-18-sveltekit-vibekit-port-design.md` (binding). **Mockups:** design canvas https://claude.ai/artifact/KabncsLYMYiTxSkSgs75YH, page "Superwitness", rows "Restyle · …". **Behaviour reference:** the current React source (`web/src`, read before porting each piece) and `.superpowers/sdd/2026-10-12-vibes-sweep/superwitness-inventory/` (INVENTORY.md has every state and string; FACTS.md the data; DEFECTS.md the defects) in the Superpipeline `vibes-sweep-plan` worktree.

## Global Constraints

- Behaviour, URLs, query-string state, API calls and copy are unchanged, except the listed defect fixes and the mockups' added words (spec, "Defects fixed on the way").
- URLs: `/`; `/runs/superpipeline/{board}/{run}` (ids `[A-Za-z0-9_-]{1,128}`, optional trailing `/`); `/rubrics`; `/rubrics/{id}/{version}` (id `[A-Za-z0-9_.-]{1,64}`, version `[1-9][0-9]{0,8}`); anything else is the in-shell "Page not found". Query keys: runs filters `source scope status(repeatable) executor since until needs_verdict`; run page `tab` (`trace|transcript|logs|errors|verdicts|attempts`, `trace` omitted) `span` `attempt` `seq` (`N` or `N-M`).
- CSP stays `default-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; frame-ancestors 'none'` plus the hashes added in Task 1. No inline event handlers, no `eval`, no remote fonts or images.
- No new product features (spec "Non-goals"). Do not touch `landing/`, `docs-site/`, the Go API or the data model.
- Evidence ticks (max 100 combined), the drawer, and the Logs level filter are per-run-view client state and are lost on reload, as today.
- Every control meets: real element (`a`, `button`, `input`+`label`), >= 44px target on touch, visible focus, 16px inputs, one `h1` per page, `aria-label` on icon-only buttons, text contrast >= 4.5:1, no colour-only meaning.
- Public repository: no secrets, no tokens, no cookies in fixtures or logs.
- Commit after every task with `git commit` (end messages with the attribution lines the session provides). One PR for the whole port (operator preference): `feat/sveltekit-vibekit`, opened as draft, never merged or tagged by an agent.

## Review Focus (inputs the spec implies but no obvious task tests; each has a test in the owning task)

1. A 401 on any call, including inside the open verdict drawer, must route to sign-in carrying the current path and query (Task 4, Task 11).
2. `unknown`, `unreported`, `in progress` and empty strings render as deliberate values, never blank or 0 (Task 7, Task 9).
3. A 100-item evidence cap: the 101st tick is refused, the drawer's over-limit alert appears (Task 8, Task 11).
4. Long unbroken values (digest, principal, span id, long titles, 5,000-character transcript lines) never widen the page at 360 (Task 14).
5. `?seq=3-4` for a card that is only on page 3 of the transcript pages forward (up to 20 pages) and lands on it; `?span=` for a span not in the loaded trace behaves as `_react/run/SpanPane.tsx` does today (never a blank, broken pane) (Task 8, Task 9).

## File structure (end state)

```
web/
  package.json            svelte, @sveltejs/kit, adapter-static, vibekit (file:), vitest, playwright, axe
  svelte.config.js        adapter-static -> ../internal/web/dist, fallback index.html
  vite.config.ts          sveltekit(), vitest (jsdom), dev proxy to the Go server
  tsconfig.json  vitest.config.ts  playwright.config.ts (existing, edited)
  vendor/superjackfruit-vibekit-0.0.13.tgz
  static/favicon.svg  appearance-head.js (copied from vibekit by a script)  legacy-theme.js
  src/app.html  src/app.css  src/app.d.ts
  src/routes/+layout.ts (ssr=false) +layout.svelte  +page.svelte (runs)
  src/routes/runs/superpipeline/[board=id]/[run=id]/+page.svelte
  src/routes/rubrics/+page.svelte  rubrics/[id=rubricid]/[version=version]/+page.svelte
  src/routes/[...rest]/+page.svelte      (Page not found, in the shell)
  src/params/{id,rubricid,version}.ts   matchers
  src/lib/
    api.ts types.ts format.ts sources.ts messages.ts   (moved, unchanged)
    runs/{filters,group}.ts  run/{facts,transcript}.ts  verdict/scale.ts   (moved, unchanged)
    data/fetched.svelte.ts  data/pages.svelte.ts   (replaces hooks.ts, usePages)
    appearance.svelte.ts  signout.svelte.ts  evidence.svelte.ts
    ui/ (StatusPill, Chips, Tabs, Waterfall, Inspector, ScoreSlider, LevelBadge, Redaction, KeyValue,
         Panel, Loading, ErrorPanel, Empty, ...)  tones.css
    components/ (Shell, EdgePage, RunCard, RunHeader, Facts, TracePanel, SpanPane, TranscriptPanel,
                 LogsPanel, ErrorsPanel, AttemptsPanel, VerdictList, VerdictDrawer, ScaleInput, ...)
  e2e/ (existing specs, unchanged; sweep.spec.ts and phone.spec.ts new)
internal/web/web.go       + hashing of inline scripts; web_test.go
Makefile                  web target cleans dist first
```

---

### Task 0: Worktree, scaffold SvelteKit, vendor vibekit, empty app builds into `internal/web/dist`

**Files:**
- Create: `web/svelte.config.js`, `web/vite.config.ts` (replace), `web/tsconfig.json` (replace), `web/vitest.config.ts`, `web/src/app.html`, `web/src/app.css`, `web/src/app.d.ts`, `web/src/routes/+layout.ts`, `web/src/routes/+layout.svelte`, `web/src/routes/+page.svelte`, `web/static/favicon.svg` (moved from `web/public/`), `web/vendor/superjackfruit-vibekit-0.0.13.tgz`
- Modify: `web/package.json`, `Makefile` (target `web`), `.gitignore`
- Remove (at the end of this task): `web/index.html`, `web/src/main.tsx`, React deps, `web/public/`

**Interfaces:** Produces the build contract: `npm run build` in `web/` writes `internal/web/dist/index.html` plus `internal/web/dist/_app/**` and `favicon.svg`.

- [ ] **Step 1: Work in the worktree** `/Users/rakeshgangwar/SuperJackfruit/superwitness-worktrees/sveltekit` on branch `feat/sveltekit-vibekit` (already created from `origin/main`). Copy the vibekit tarball: `cp /Users/rakeshgangwar/SuperJackfruit/vibekit-worktrees/artifacts/superjackfruit-vibekit-0.0.13.tgz web/vendor/`. Verify `tar tzf` lists `package/dist/appearance-head.js`. (If the file is gone, rebuild from the vibekit `v0.0.13` tag: `pnpm install && pnpm package && pnpm pack`.)

- [ ] **Step 2: Write `web/package.json`** (keep name/private/type). Scripts: `dev: vite dev`, `build: svelte-kit sync && vite build`, `check: svelte-kit sync && svelte-check --tsconfig ./tsconfig.json`, `test: vitest run`, `e2e: playwright test`. Dependencies: `@superjackfruit/vibekit: file:./vendor/superjackfruit-vibekit-0.0.13.tgz`, `svelte ^5.56.0`. Dev: `@sveltejs/kit ^2`, `@sveltejs/adapter-static ^3`, `@sveltejs/vite-plugin-svelte ^6`, `vite ^7`, `vitest ^3`, `jsdom`, `@testing-library/svelte ^5`, `@testing-library/jest-dom`, `svelte-check`, `typescript ~5.9`, `@playwright/test` (existing version), `@axe-core/playwright 4.13.0`. Remove `react`, `react-dom`, `@types/react*`, `@vitejs/plugin-react`, `@fontsource/*` (vibekit brings its own fonts). Run `npm install`. Commit `package-lock.json`.

- [ ] **Step 3: Config files.**

```js
// web/svelte.config.js
import adapter from "@sveltejs/adapter-static";
import { vitePreprocess } from "@sveltejs/vite-plugin-svelte";
export default {
  preprocess: vitePreprocess(),
  kit: {
    // The console is a client-only app served by the Go binary, which answers every
    // extension-less path that is not /v1, /mcp, /auth or /health with index.html.
    adapter: adapter({ pages: "../internal/web/dist", assets: "../internal/web/dist", fallback: "index.html", precompress: false, strict: false }),
    paths: { base: "" },
    router: { type: "pathname" },
    // The committed dist must be byte-identical on every build (make web-check), and the inline
    // bootstrap script's hash must not change: SvelteKit's default version name is a timestamp.
    version: { name: "superwitness" },
  },
};
```

```ts
// web/vite.config.ts
import { sveltekit } from "@sveltejs/kit/vite";
import { defineConfig } from "vitest/config";
export default defineConfig({
  plugins: [sveltekit()],
  // The CSP has no font-src, so default-src 'self' would block data: fonts: never inline assets.
  build: { assetsInlineLimit: 0 },
  server: { proxy: { "/v1": "http://127.0.0.1:8790", "/auth": "http://127.0.0.1:8790", "/health": "http://127.0.0.1:8790" } },
  test: { include: ["src/**/*.test.ts"], environment: "jsdom", setupFiles: ["./vitest.setup.ts"] },
  resolve: process.env.VITEST ? { conditions: ["browser"] } : undefined,
});
```

`web/src/routes/+layout.ts`: `export const ssr = false; export const prerender = false;`. `web/vitest.setup.ts`: `import "@testing-library/jest-dom/vitest";`. Add `.svelte-kit/` and `web/node_modules/` to `.gitignore`. `tsconfig.json` extends `./.svelte-kit/tsconfig.json`.

- [ ] **Step 4: Minimal `app.html`, `+layout.svelte`, `+page.svelte`** (the page prints "Runs"). `app.html` has `<html lang="en" data-product="superwitness">`, the viewport meta with `viewport-fit=cover`, `<link rel="icon" type="image/svg+xml" href="/favicon.svg">`, `<title>superwitness</title>`, `%sveltekit.head%`, `<div style="display: contents">%sveltekit.body%</div>`. The first-frame appearance is added in Task 4.

- [ ] **Step 5: Makefile `web` target** becomes `cd web && npm ci && npm run check && npm test && npm run build` preceded by `rm -rf internal/web/dist` (the adapter does not empty its output). Keep `web-check`'s `git status --porcelain -- internal/web/dist` test.

- [ ] **Step 6: Build and check.** Run `make web`. Expected: exit 0; `internal/web/dist/index.html` exists; `ls internal/web/dist/_app` shows assets; `grep -c '<script' internal/web/dist/index.html` prints the number of inline scripts (note it for Task 1). Run `go test ./internal/web/...` and fix the tests that name the old build: `web_test.go:17` asserts the body contains `<div id="root">` (assert a SvelteKit marker such as `__sveltekit` instead); line 28 requests `/assets/missing.js` (still a 404 for a path with an extension: keep); `TestFaviconMatchesTheMark` (line 78) reads `../../web/public/favicon.svg` (now `../../web/static/favicon.svg`).

- [ ] **Step 7: Move the old app aside, in this order.** (a) FIRST move the pure modules and their tests into the new tree (this is Task 2's move, done here so the new `src/routes` can live beside them): `git mv web/src/types.ts web/src/api.ts web/src/format.ts web/src/sources.ts web/src/verdict/messages.ts web/src/verdict/scale.ts` and the `*.test.ts` files of `api`, `format`, `sources`, `runs/filters`, `runs/group`, `run/facts`, `run/transcript` into `web/src/lib/` (same subfolders: `lib/runs`, `lib/run`, `lib/verdict`), fixing relative imports only. (b) THEN `git mv` everything left under `web/src/` that is React (`*.tsx`, `router.tsx`, `hooks.ts`, `theme.ts`, `theme.css`, `signout.tsx`, `rubrics/`, the remaining `run/`, `runs/`, `verdict/` files) and `web/index.html`, `web/public/` (the favicon goes to `web/static/`) into `web/_react/` (excluded from the build, the type-check and the tests). It is the behaviour reference until Task 13 and is deleted there.

- [ ] **Step 8: Commit** `Superwitness web: SvelteKit scaffold with vendored vibekit 0.0.13`.

### Task 1: CSP allows the SvelteKit bootstrap by hash; appearance script is a file

**Files:**
- Modify: `internal/web/web.go`, `internal/web/web_test.go`
- Create: `web/scripts/copy-vibekit-assets.mjs`, `web/static/legacy-theme.js`
- Modify: `web/package.json` (script `prebuild`), `web/src/app.html`

**Interfaces:** Produces `func policyFor(index []byte) string`: `CSP` plus `; script-src 'self' 'sha256-<b64>' …` for every inline `<script>` in `index`. The exported `CSP` const is unchanged (the Go-rendered sign-in pages use it and carry no script).

- [ ] **Step 1: Write the failing Go tests** in `internal/web/web_test.go`:

```go
func TestPolicyForHashesInlineScripts(t *testing.T) {
	html := []byte(`<html><head><script src="/a.js"></script></head><body><script>console.log(1)</script></body></html>`)
	sum := sha256.Sum256([]byte("console.log(1)"))
	want := "'sha256-" + base64.StdEncoding.EncodeToString(sum[:]) + "'"
	got := policyFor(html)
	if !strings.Contains(got, "script-src 'self' "+want) { t.Fatalf("policy %q lacks %s", got, want) }
	directive := got[strings.Index(got, "script-src"):] // script-src is the last directive policyFor appends
	if strings.Contains(directive, "'unsafe-inline'") || strings.Contains(directive, "'unsafe-eval'") { t.Fatalf("script-src must stay strict: %q", directive) }
}
func TestPolicyForNoInlineScripts(t *testing.T) {
	got := policyFor([]byte(`<script src="/a.js"></script>`))
	if !strings.Contains(got, "script-src 'self'") || strings.Contains(got, "sha256") { t.Fatalf("got %q", got) }
}
func TestHandlerServesHashedPolicyOnIndexAndFallback(t *testing.T) { /* GET / and GET /rubrics: header contains script-src 'self' and the hash of every inline script in the embedded index */ }
func TestEmbeddedIndexHasExactlyOneInlineScript(t *testing.T) { /* count inline <script> bodies in dist/index.html and require exactly 1 (SvelteKit's bootstrap): a stray inline script would otherwise be hashed and allowed silently */ }
```

- [ ] **Step 2: Run** `go test ./internal/web/ -run Policy -v`. Expected: FAIL (`policyFor` undefined).

- [ ] **Step 3: Implement** in `web.go`: a regexp `(?s)<script(?:\s[^>]*)?>(.*?)</script>` finds inline scripts (skip those whose opening tag has `src=`); hash each body with SHA-256, base64 standard encoding; `policyFor` returns `CSP + "; script-src 'self'" + hashes`. In `Handler()` compute `policy := policyFor(indexBytes)` once from the embedded `dist/index.html` and set it instead of `CSP` on every response. Keep `X-Content-Type-Options`.

- [ ] **Step 4: Run** `go test ./internal/web/... ./internal/session/...`. Expected: PASS (session pages still use `CSP`).

- [ ] **Step 5: Serve the appearance head script as a file.** `web/scripts/copy-vibekit-assets.mjs` copies `node_modules/@superjackfruit/vibekit/dist/appearance-head.js` to `web/static/appearance-head.js` (run by `prebuild` and `predev`); `.gitignore` the copy. `static/legacy-theme.js` is the one-time seed of the old toggle into vibekit's cookie (the first line of Superpipeline's `app.html`, with key `superwitness.theme`). In `app.html` add, in this order, `<script src="/legacy-theme.js"></script>` then `<script src="/appearance-head.js"></script>` (blocking, in `<head>`), then the first-frame `<style>` block (copy Superpipeline's `:where(html)` backgrounds).

- [ ] **Step 6: Build twice and compare.** Run `make web` twice; `git status --porcelain -- internal/web/dist` must be empty after the second run (deterministic output: `kit.version.name` is constant). If not, find the varying file and fix the cause before going on.
- [ ] **Step 6b: Build and run in a browser.** `make web && make build`, run the binary the way `web/e2e/serve.sh` does against a throwaway Postgres (`make e2e` starts one; if Docker is unavailable use the repo's documented fake-mode instructions), open `/`, expect zero CSP violations in the console (use Playwright `page.on('console')` in a scratch script). 

- [ ] **Step 7: Run the whole e2e suite against the placeholder app** (`make web && make build && make e2e`). Expected: the specs that need real pages fail, but the sign-in redirect, the CSP and the SPA fallback checks do not: this proves the serving chain end to end before any page is ported. From here on, re-run `make e2e` at the end of every page task (7-12).
- [ ] **Step 8: Commit** `Superwitness: CSP hashes the SvelteKit bootstrap; the appearance script is a file`.

### Task 2: Verify the moved pure modules; add `readableTime`

**Files:** the modules moved in Task 0 Step 7(a) (`types.ts`, `api.ts`, `format.ts`, `sources.ts`, `runs/filters.ts`, `runs/group.ts`, `run/facts.ts`, `run/transcript.ts`, `verdict/scale.ts`, `verdict/messages.ts` and their tests; the DOM cases of `scale.test.tsx` stay in `_react/` and are ported in Task 11). Not moved: `router.tsx`, `theme.ts`, `hooks.ts` (replaced in Tasks 3 and 5).

- [ ] **Step 1: Run** `npm test`. Expected: every moved test passes untouched (the React component tests live in `_react/` and are excluded). Keep relative imports in these files.
- [ ] **Step 3: Add the one new pure function the mockups need:** `readableTime(iso: string, now = new Date()): { text: string; iso: string }` in `format.ts` ("Today, 10:41" / "Yesterday, 09:02" / "13 Oct 2026, 10:02" in the viewer's zone; unparsable input returns `{ text: iso, iso }`). Test first (three cases + invalid). Commit `Superwitness web: pure modules move to src/lib; readableTime`.

### Task 3: Routing and the data layer

**Files:** Create `web/src/params/{id,rubricid,version}.ts`, the route files listed above, `web/src/lib/data/fetched.svelte.ts`, `web/src/lib/data/pages.svelte.ts` and their tests, `web/src/lib/router.test.ts` (port of `router.test.ts`).

**Interfaces:**
- `fetched<T>(path: () => string | null): { readonly data: T | null; readonly error: ApiError | null; readonly loading: boolean; reload(): void }` (runes; refetches when `path()` changes; a stale answer never overwrites a newer one).
- `pages<T>(key: () => string, fetchPage: (cursor: string | null) => Promise<{ items: T[]; next: string | null }>): { items: T[]; next: string | null; error: ApiError | null; moreError: ApiError | null; loading: boolean; more(): void; retry(): void }` (the exact surface of `usePages(key, fetchPage)` at `_react/run/Panels.tsx:31`, including `moreError` and `retry`; cursor paging with a generation guard; callers key it by `board/run|level` etc.). The runs page keeps its own first-page `counts` by returning the first page's extra fields through a `first` accessor added to this surface.
- Param matchers: each `src/params/<name>.ts` is `export function match(param: string): boolean`; `id` = `/^[A-Za-z0-9_-]{1,128}$/`, `rubricid` = `/^[A-Za-z0-9_.-]{1,64}$/`, `version` = `/^[1-9][0-9]{0,8}$/`.

- [ ] **Step 1: Write failing tests** for the three matchers (valid and invalid strings from `router.test.ts`), for `fetched` (loading -> data; error; reload; changing the path ignores the slow first answer) and `pages` (second page appended; error keeps rows; reload resets; a stale page 2 after a reload is dropped).
- [ ] **Step 2: Run** `npm test -- src/params src/lib/data`; expected FAIL.
- [ ] **Step 3: Implement** the matchers, `fetched` and `pages` with `$state`, `$effect` and a generation counter (read `hooks.ts` and `usePages` first; keep their semantics; tests use `flushSync` and a fake `fetch`).
- [ ] **Step 4: Routes.** `src/routes/+page.svelte` (runs), `runs/superpipeline/[board=id]/[run=id]/+page.svelte`, `rubrics/+page.svelte`, `rubrics/[id=rubricid]/[version=version]/+page.svelte`, `[...rest]/+page.svelte` (Page not found). Each renders a placeholder title for now. The Shell decides `aria-current` from SvelteKit's own `page.route.id` and `page.url` (no second routing table that could drift). The unit tests pin the three matchers with the strings from `_react/router.test.ts`; the URL table through the real router is the Task 13 e2e.
- [ ] **Step 5: Run** `npm test && npm run build`. Expected: PASS. Trailing-slash: set `export const trailingSlash = "ignore"` in `+layout.ts` and confirm `/rubrics/` resolves.
- [ ] **Step 6: Commit** `Superwitness web: routes, param matchers, fetched() and pages()`.

### Task 4: Edge pages, signed-out, loading and not-found states

**Files:** Create `src/lib/components/EdgePage.svelte`, `NotFound.svelte`, `RunNotFound` handling in the run route, `src/lib/ui/{Panel,Loading,ErrorPanel,Empty}.svelte`; tests `EdgePage.test.ts` (port of `Edge.test.tsx` cases: every code in INVENTORY section 3 with its heading, body and action), `Loading.test.ts`.

- [ ] **Step 1: Failing tests** for each edge state (signed out, `not_authorised`, `store_unavailable` with 30 s auto-retry via fake timers, `rate_limited` with `Retry-After` seconds and without, `principal_unresolved`, `origin_mismatch`, generic including network, `run_not_found`, page not found in the shell, lingering "Signed out", app-level "Loading…"), and the new "Rubric not found" state with a link to `/rubrics`.
- [ ] **Step 2-4: Implement to the mockups** (`Edge pages`, `SignedOut`, `Phone: edge pages`): the focus layout (one centred column, wordmark above). Loading renders per vibe (Daylight soft bars, Paper ruled blank lines, Studio grey blocks with a dash and LOADING, Quiet plain "Loading…"), always with the words "Loading…" present for screen readers and tests.
- [ ] **Step 5: Run** tests and build; commit `Superwitness web: edge pages and loading states`.

### Task 5: Session (401), appearance, the shell, sign-out, sources dot

**Files:** Create `src/lib/appearance.svelte.ts` (adapt Superpipeline's, product `superwitness`, legacy key `superwitness.theme`, place key `superwitness.place`), `src/lib/signout.svelte.ts` (port of `signout.tsx`), `src/lib/session.svelte.ts` (`me` store, `signInHref(path, query)`), `src/lib/components/Shell.svelte`, `Appearance.svelte` (the control per mockup `Shell: states`), `AccountMenu.svelte`, `SourcesDot.svelte`; modify `+layout.svelte`; tests `Shell.test.ts`, `appearance.test.ts`, `signout.test.ts`, `session.test.ts`.

**Interfaces:** `session.me: Fetched<Me>`; `signInHref(path: string, query: string): string` returns `/auth/login?next=` + `encodeURIComponent(path + (query ? "?" + query : ""))`. Any `ApiError` with status 401 from any call sets `session.expired = true`; the layout then renders the signed-out edge page whose "Sign in" link is `signInHref(currentPath, currentQuery)` (fixes the lost deep link; today inside-page failures link to `/`).

- [ ] **Step 1: Failing tests.** Shell: sidebar groups "Runs" (All, Needs verdict, Failed, Waiting), "Boards" (only if scopes load; label `name ?? id`), "Library" (Rubrics); `aria-current="page"` exactly as `Shell.test.tsx` asserts; sources dot "Sources ok" / "Sources degraded" with detail in `title`; health polled every 60 s (fake timers); `store_unavailable` turns the dot amber "The database is not answering" and recovers; Sign out only when `me.via === "session"`; the failure line and reasons (all seven strings from INVENTORY section 2); 204 navigates to `/`; 503 `store_unavailable` shows the "Signed out" edge page. Phone: topbar with "Menu" (`aria-expanded`, `aria-controls="nav"`), inline block, closes on navigation. Appearance: choosing a vibe/theme/accent writes the `vk_appearance` cookie and sets `data-vibe`/`data-theme`/`data-accent` on `<html>`; the legacy `superwitness.theme` seeds the cookie once. Session: a 401 anywhere makes the sign-in href carry the current path and query.
- [ ] **Step 2: Run** `npm test -- src/lib`; expected FAIL.
- [ ] **Step 3: Implement** to the mockups (`Shell: states`, `Phone: Menu`, row 1 of each vibe for sidebar/rail/masthead conventions: Daylight and Paper a left column, Studio the ink rail, Quiet a plain list above the content). Use vibekit's `VibeRoot`, the vibe's wordmark pattern from the artboards, and `src/lib/tones.css` (new) for `--sw-warn*`/`--sw-info*` in four vibes x light/dark, values taken from the artboards (Daylight and Paper define them locally in the mockup files; use those hexes). One home link per page.
- [ ] **Step 4: Layout** `+layout.svelte`: imports `../app.css` (vibekit `bundle.css`, `tones.css`), calls `appearance.init()` on mount, fetches `/v1/me` first: error -> full-page `EdgePage` (Task 4), loading -> bare "Loading…" without the shell (as today), data -> `Shell` + `{@render children()}`.
- [ ] **Step 5: Run** `npm test && npm run build`; commit `Superwitness web: shell, appearance, sign-out, sources dot, deep-link-safe sign-in`.

### Task 6: Component audit: wrap vibekit where it has the component, build only the gaps

**Decision (builder's call, recorded in the spec):** vibekit 0.0.13 already ships `Tabs`, `DataTable`, `Dialog`, `Drawer`, `Sheet`, `Menu`, `Loading`, `EmptyState`, `Notice`, `FactList`, `LoadMore`, `AppShell` and `AppearancePicker`. Use them wherever their behaviour matches the old app (the mockups were drawn on vibekit's conventions); write a bespoke component only for what vibekit lacks or where a test below fails against vibekit's version.

**Files:** Create `web/src/lib/ui/README.md` (a table: need -> vibekit component or bespoke, with the reason, filled in by each task as it first needs one), plus the bespoke components as they are needed: `StatusPill`, `Waterfall`, `Inspector`, `ScoreSlider`, `LevelBadge`, `Redaction`, `KeyValue` (only `StatusPill` is needed before Task 7; build the others in the task that first uses them).

**Interfaces (unchanged from the plan's first draft):** `StatusPill({ status: string })` with the tone map `succeeded|completed -> success`, `failed -> danger`, `waiting -> warning`, `running -> info`, `queued|cancelled|unknown|other -> neutral`, word always rendered. `ScoreSlider({ value: number | null, onchange })` with 20 steps and `null` = "not set" (`aria-valuetext="not set"`, mid-track thumb). `Inspector({ open, label, onclose })`: non-modal `aside` with `aria-label`, Close button, Escape not captured.

- [ ] **Step 1: Read `node_modules/@superjackfruit/vibekit/dist/index.d.ts` and `llms.txt`; write the README table** for every component the old app has (tabs, tables, dialogs/drawer, loading, empty, notice, fact list, load more, appearance picker). For `Tabs`: write the failing test first (Left/Right/Home/End move and activate; selected tab scrolls into view; each `a` has `role=tab` and `aria-selected`); if vibekit's `Tabs` passes, wrap it; otherwise build `src/lib/ui/Tabs.svelte`.
- [ ] **Step 2: Build `StatusPill`** (test-first: five tone mappings, the word always present, `completed` in the success tone) and the local `tones.css` if Task 5 has not yet.
- [ ] **Step 3: Run** `npm test`; commit `Superwitness web: component audit and StatusPill`.

### Task 7: Runs page

**Files:** `src/routes/+page.svelte`, `src/lib/components/{RunCard,DayGroup}.svelte`, tests `RunsPage.test.ts` (port of `RunsPage.test.tsx`), `RunCard.test.ts`.

- [ ] **Step 1: Failing tests** from the React tests: view titles (`viewTitle`), six chips in order with counts from the first page and the selected state (`aria-current="true"`, clicking sets one status, clicking the selected one clears it, two statuses select none), day groups ("Today", "Yesterday", `Tue, 13 Oct 2026`, "Unknown date"), card variants (link only for superpipeline with `external_ref` `board/run`; others non-interactive with a visible "no run page yet" tag and the `title`), timing text, verdict text (decision, label, score `0.80`, text cut at 40 + "…", JSON, "no verdict"), "Load more" with IntersectionObserver auto-load (fake IO) and error retry, empty states ("No runs reported yet." + external link, "No runs in this view."), loading, `store_unavailable`/rate-limited/server error edge states.
- [ ] **Steps 2-4: Implement** to rows 1 and 10 of the mockups in each vibe. Cards one line at >= 1100px.
- [ ] **Step 5:** run tests and build; commit `Superwitness web: runs page`.

### Task 8: Run page frame, facts, tabs and evidence

**Files:** `src/routes/runs/superpipeline/[board=id]/[run=id]/+page.svelte`, `src/lib/components/{RunHeader,Facts,SourceUnavailable}.svelte`, `src/lib/evidence.svelte.ts` (`createEvidence()`: `ticked: Set<string>`, `cited: Set<Ref>`, `count`, `tick(id)`, `cite(ref)`, `canAdd` false at 100, keyed by `board/run` so it resets when the run changes), tests `RunViewPage.test.ts`, `evidence.test.ts`.

- [ ] **Step 1: Failing tests** (ports of `RunViewPage.test.tsx` and `Tabs.test.tsx`, plus the new ones): header (`superpipeline · board/run`, h1 title or `board/run` when `unknown`, `completed` in the success tone, "Record verdict"), facts (Agent, Attempts, Errors, Duration `unknown|in progress|5m 0s`, Cost `$1.23|unreported|unknown`; a 60-character principal wraps and the facts strip never overflows), tab URL state (`?tab=`, `trace` omitted), per-tab source-unavailable box with the exact sentence for each status, evidence resets when the run changes (`evidence.test.ts`: the 101st tick is refused).
- [ ] **Steps 2-4: Implement** to the mockups (`Run: states`, `Phone: Run`): header on the vibe's header treatment, facts strip (a wrapping grid on phone), tabs, evidence store.
- [ ] **Step 5:** run tests, build and `make e2e`; commit `Superwitness web: run page frame, facts, tabs, evidence`.

### Task 8b: Trace tab and span pane

**Files:** `src/lib/components/{TracePanel,SpanPane,SpanSession}.svelte`, `src/lib/ui/{Waterfall,Inspector,KeyValue}.svelte` (bespoke unless the README says vibekit has them), tests `TracePanel.test.ts`, `SpanPane.test.ts` (port of `SpanPane.test.tsx`).

- [ ] **Step 1: Failing tests:** waterfall (500 spans/page, indentation, cite tick `aria-label="Cite {name} as evidence"`, disabled at 100, the 101st refused, name opens `?span=`), span pane (facts, Attributes sorted, Logs window, Session content states, "Open in transcript ↗", Cite toggle, a `?span=` id that is not in the loaded trace: keep whatever `_react/run/SpanPane.tsx` does today, and pin it with a test; do not invent copy), Close removes `span` from the URL.
- [ ] **Steps 2-4: Implement** to the mockups (`Run: Trace`, `Run: span details`, `Phone: Run`, `Phone: span details`). The pane follows each vibe's round-1 treatment: Daylight and Quiet reflow beside the page, Paper and Studio float with edge and shadow; phone is full screen with a sticky head and a sticky Cite.
- [ ] **Step 5:** run tests, build and `make e2e`; commit `Superwitness web: Trace tab and span pane`.

### Task 9: Transcript tab

**Files:** `src/lib/components/{TranscriptPanel,TranscriptCard,TranscriptRefusal}.svelte`, tests `TranscriptPanel.test.ts` (port of `TranscriptPanel.test.tsx`).

- [ ] **Step 1: Failing tests:** attempt picker (`?attempt=`, unknown id message), running attempt line, empty, each card kind (prompt, message, reasoning in a closed `details`, tool call `details` open when failed with `{title} · {tool_kind} · {status}`, permission with "Outcome: pending" when none, state, error, other-by-type), redaction markers rendered with `Redaction`, "{n} redacted", cut fields with "Show full" (success and error), refusals (403 muted, 503/network amber with Try again, other red), `?seq=N` and `N-M` scroll to and outline the card, paging forward up to 20 pages to find it (fake pages), citing a card toggles "Cite"/"Cited" and counts toward the 100 cap, long monospace lines wrap.
- [ ] **Steps 2-4: Implement** to the mockups (`Run: Transcript`, `Transcript: states`, `Phone: Transcript`).
- [ ] **Step 5:** tests and build; commit `Superwitness web: Transcript tab`.

### Task 10: Logs, Errors, Attempts, Verdicts list

**Files:** `src/lib/components/{LogsPanel,ErrorsPanel,AttemptsPanel,VerdictList}.svelte`, tests (`Panels.test.ts` ported from the logs/errors/attempts/verdict cases in `RunViewPage.test.tsx`).

- [ ] **Step 1: Failing tests:** Logs level select (all, debug, info, warn, error; server-side filter via the query), table WITH a header row (Time, Level, Service, Message) and readable times with the ISO in `title`, 100/page, empty, error + Try again, Load more; Errors table and "No errors."; Attempts table (Attempt, State, Configuration fingerprint, Harness, Model; the 71-character digest wraps inside its cell; missing values `unknown`); Verdicts: gate rows in words (`resolved|pending|cancelled` with "no decision yet" for a null decision, never raw JSON), recorded verdicts oldest first, superseded value struck through with an in-page "superseded by" link, "Revise" only when not superseded and `judge === me.principal` and standard starts `rubric:`, evidence links "transcript {from}–{to} ↗", per-subject read failure box with Try again (never folded into empty).
- [ ] **Steps 2-4: Implement** to the mockups (`Run: Logs`, `Run: Errors`, `Run: Attempts`, `Run: states`, `Phone: Logs, Errors, Attempts, Rubrics`: stacked rows on phone).
- [ ] **Step 5:** tests and build; commit `Superwitness web: Logs, Errors, Attempts and Verdicts`.

### Task 11: Verdict drawer

**Files:** `src/lib/components/{VerdictDrawer,ScaleInput}.svelte`, tests `VerdictDrawer.test.ts` (port of `Drawer.test.tsx`, 306 lines, every case), `ScaleInput.test.ts` (the DOM cases of `scale.test.tsx`).

- [ ] **Step 1: Failing tests:** open from "Record verdict" and from "Revise"; focus moves in and is trapped, Escape closes, backdrop click closes, focus returns to the opener, cannot close while saving; Subject and Rubric selects (disabled when revising; unsupported rubrics disabled with the suffix; "No rubric has a scale the app supports. Add one with superwitness rubric-add."); scale inputs: decision toggle buttons, label select, score slider "not set" until touched (legacy `min..max` scales rescaled to 0..1 with 4 decimals), text area; comment counter `{n} / 10000`; evidence line for each of the four shapes; over-100 alert; refusal alerts for each code; "Save verdict" disabled until complete, becomes "Saving…"; success closes and reloads the run and history; idempotency key is created when the drawer opens and reused on retry (one verdict after a lost response); **a 401 while saving sends the person to sign-in carrying the current path and query and keeps nothing half-saved**.
- [ ] **Steps 2-4: Implement** to the mockups (`Run: Verdicts and the verdict drawer`, `Verdict drawer: variants`, `Phone: verdict drawer`: full width with sticky actions). Use vibekit's dialog primitive if its behaviour matches (focus trap, Escape, backdrop); otherwise keep the existing trap logic from `Drawer.tsx`.
- [ ] **Step 5:** tests and build; commit `Superwitness web: verdict drawer`.

### Task 12: Rubrics and Rubric

**Files:** `src/routes/rubrics/+page.svelte`, `rubrics/[id=rubricid]/[version=version]/+page.svelte`, tests (`RubricPage.test.ts` port plus list cases).

- [ ] **Step 1: Failing tests:** list table (Name link, Rubric `id@version`, Created `YYYY-MM-DD`; " · scale not supported in the app" when `recognised_scale` is null), empty "No rubrics yet. Record one with superwitness rubric-add.", loading; detail (`Rubrics` link · `standard`, h1, "Created {date} by {created_by}", h2 "Scale" with the sentence for each scale kind, the raw JSON block, h2 "Rubric", pre-wrapped body), not found shows "Rubric not found" with a link to `/rubrics` (new).
- [ ] **Steps 2-4: Implement** to rows `Rubrics` and `Rubric` of the mockups; **Step 5:** tests/build; commit `Superwitness web: rubrics`.

### Task 13: Parity: the existing e2e suite passes in full, then delete the React app

**Files:** `web/e2e/*` (edit selectors only where an element was removed or renamed; list every edit), `web/playwright.config.ts`, `Makefile` (`e2e` target already builds the binary and serves `web/e2e/serve.sh`), delete `web/_react/`.

- [ ] **Step 1: Build the embedded app and the binary** (`make web && make build`).
- [ ] **Step 2: Run `make e2e`** (needs Docker; if the daemon is not running, start Docker/colima first or run in CI). Expected: every spec in `app.spec.ts` and `transcript.spec.ts` passes. Fix the app, not the test, unless the test names a removed element (e.g. the old Theme toggle text); record each test edit in the PR description with the reason.
- [ ] **Step 3: Add the URL table as an e2e** (`routes.spec.ts`): `/`, `/runs/superpipeline/brd_01/run_01`, `/runs/superpipeline/brd_01/run_01/`, `/rubrics`, `/rubrics/release/1`, `/rubrics/release/0` (not found), `/runs/superpipeline/brd_01` (not found), `/nope` (not found); and a reload on each keeps the state in the query.
- [ ] **Step 4: Delete `web/_react/`**; `npm test` and the build still pass. Commit `Superwitness web: the e2e suite passes on SvelteKit; the React app is removed`.

### Task 14: Phone and the sweep: four vibes x light/dark x 360/390/1440

**Files:** Create `web/e2e/sweep.spec.ts`, `web/e2e/phone.spec.ts`, `web/e2e/states.ts` (the 39 states from the Accounts sweep adapted: every route and state reachable with the stub hub and fake sources, plus scripted `/v1` failures through Playwright `route`).

- [ ] **Step 1: Write the sweep.** Derive the state list from INVENTORY.md (every route and every state it names that the stub hub and scripted `/v1` failures can reach; record the list in `states.ts` and the PR). Set the appearance with the cookie `vk_appearance=<vibe>.<light|dark>.strong.0` (format as in Superpipeline's `app.html`). Matrix, to keep the run reasonable: ALL states on Daylight x light and dark x 360, 390, 1440; on Paper, Studio and Quiet one representative state per page type (runs, run trace with span pane, transcript, verdicts with drawer, logs, attempts, rubrics, rubric, signed-out, database-down) x light and dark x 360, 390, 1440; set `workers: 4` for this config. For each visit assert zero axe violations (`@axe-core/playwright`, rules `wcag2a/aa`), no horizontal page scroll (`document.documentElement.scrollWidth <= clientWidth`), no console error and no CSP violation, all interactive targets >= 44px under 1024px except inline links inside text.
- [ ] **Step 2: Phone behaviours** (`phone.spec.ts`): Menu opens inline and closes on navigation; tabs row scrolls the current tab into view; span pane and drawer fill the screen with sticky actions; a 60-character agent principal and a 71-character digest do not widen the page.
- [ ] **Step 3: Run** `make e2e` (the new specs use their own project in the same config so the sweep can be run alone with `npx playwright test sweep`). Fix what the sweep finds (CSS only; behaviour changes need a test and a note).
- [ ] **Step 4: Commit** `Superwitness web: four-vibe sweep and phone specs`.

### Task 15: Docs, embedded build, PR

**Files:** `README.md` (web section: SvelteKit, `make web`, vibekit vendor update steps), `CONTRIBUTING.md`, `web/vendor/README.md` (how to update the tarball: build the tag, `npm i file:…`), `CHANGELOG.md` (Unreleased: "The console is rebuilt on SvelteKit and vibekit: four vibes, light and dark, phone layouts, same behaviour"), `docs-site` page for the app if it mentions the stack (grep first).

- [ ] **Step 1:** `make web-check` (clean tree after `make web`), `go vet ./... && go test ./...`, `npm test`, `npm run check`, `make e2e` all green. Record the commands and counts in the PR body.
- [ ] **Step 2: Push** `feat/sveltekit-vibekit`; open a **draft** PR "Superwitness console on SvelteKit and vibekit: four vibes, same behaviour" (GitHub, `SuperJackfruitLabs/superwitness`) with: what changes for a person, what did not (behaviour/URLs/API), the mockup canvas link, the CSP change (hash of one inline script, computed at start-up; the appearance script is a file), the e2e selector edits, known gaps (vibekit lacks warning/info tones and the components in `src/lib/ui/`: filed as a vibekit follow-up), screenshots of each vibe, and the test results. End the body with the session attribution lines.
- [ ] **Step 3:** Wait for GitHub CI (Go tests, `make web-check`, e2e). Fix failures. Mark ready only when green. The operator merges; a release PR (version + CHANGELOG) and tag follow the repository's usual flow and are asked about first.
- [ ] **Step 4: Follow-ups (not in this PR):** (a) issue on superwitness for the earlier "queue" mockups (search/⌘K, Judge next, needs-your-verdict queue, per-board counts, agent faces) with the canvas link; (b) vibekit issue/PR: warning and info tones, and the UI components in `src/lib/ui/` for 0.0.14.

## Self-review notes

- Spec coverage: goals 1-5 map to Tasks 4-14 (1), 13 (2), 0 and 15 (3), 1 (4), the defect list to Tasks 4-12 (each defect has a test named in its task). Non-goals have no task.
- Types: `fetched`, `pages`, `signInHref`, `createEvidence`, `StatusPill`, `Tabs`, `ScoreSlider`, `Inspector` signatures are defined once (Tasks 3, 4, 6, 8) and used as defined later.
- Risks: SvelteKit's static `index.html` may contain more than one inline script (handled: all are hashed); vibekit's dialog primitive may not match the drawer's focus rules (fallback to the existing trap logic stated in Task 11); `make e2e` needs Docker (stated in Tasks 1, 13, 14).
