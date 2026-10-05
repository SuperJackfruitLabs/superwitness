# superwitness landing page

The public page at `superwitness.dev`. Astro, one page and a 404, no framework, no scripts.

```sh
npm install
npm run dev     # http://localhost:4327
npm run build   # -> dist/
```

## How it deploys

A Cloudflare Worker with static assets (not Pages: wrangler 4.147 no longer creates Pages
projects), named `superwitness-site`, configured by
`landing/wrangler.jsonc`. From the repo root:

```sh
scripts/deploy-sites.sh landing   # builds, then wrangler deploy --config landing/wrangler.jsonc
```

The custom domains (the apex and `www`) are declared in the config, so a deploy also attaches them. It runs with the
operator's own `wrangler login`; there is no API token, in CI or anywhere else.

The Worker sets `assets.not_found_handling: "404-page"`, which serves `dist/404.html` with a real
404 status for any path that has no file. That is why `src/pages/404.astro` exists: without it a
missing path gets no page of ours, and with single-page handling it would get the home page and a
200 — telling a crawler that `/anything` is a real page.

## Why npm, and why it sits outside the Go module

Astro 7 brings Vite 8. The service's own UI in `web/` has its own toolchain, and this site should
not share a dependency tree with it, so `landing/` has its own `package.json` and
`package-lock.json`. Nothing in the Go build looks in here.

## What is deliberate about the page

**The hero is a run document**, not a headline over a gradient: the one thing superwitness does is
join what an agent did, how the products behaved and how the work was judged into one record, so
the page shows that record.

**Every label in it is real.** Each one is a field name or value that
`GET /v1/runs/superpipeline/{board}/{run}` returns (`internal/join/document.go`); the comment at
the top of `src/pages/index.astro` maps each label to its source. If those change, this page
becomes wrong. The `agent` row shows what ran rather than who: `run.agent` is a principal id, so
the row carries the attempt fingerprint's harness, version and digest instead of printing the id.

**Jade is spent on evidence:** "on the record", a joined trace, an approved gate, the sources that
answered. Cost is shown as `unreported` in amber because that is what the API says when nobody
reported it — the page does not invent a number.

**It follows the visitor's light or dark preference.** Light-theme jade is the deep jade
(#1b7f63), because #3fd0a2 on a light ground is 1.9:1; the measured ratios are in a comment above
`:root`.

**One mark.** The header, the footer, the favicon, the 404 and `og.png` all show the ring-and-eye
in `public/favicon.svg`. `src/components/Mark.astro` and `og/og.html` inline the same shapes; check
they still match with:

```sh
diff <(grep -o 'd="[^"]*"\|r="[^"]*"' public/favicon.svg) \
     <(grep -o 'd="[^"]*"\|r="[^"]*"' src/components/Mark.astro) && echo mark ok
```

The link preview is rendered from `og/` — see the README there.

**No analytics, no scripts, no sign-in.** There is nothing to sign in to: superwitness is
self-hosted. The page says only what ships today.
