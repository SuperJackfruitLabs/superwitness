# superwitness docs site

The user-facing documentation published at `docs.superwitness.dev`. Astro + Starlight.

```sh
npm install
npm run dev     # http://localhost:4328
npm run build   # -> dist/
```

## Why npm, and why it sits outside the Go module

Astro 7 brings Vite 8. The service's own UI in `web/` has its own toolchain, and this site
should not share a dependency tree with it, so `docs-site/` has its own `package.json` and
`package-lock.json`, as `landing/` does. The site ships nothing the binary embeds or imports, so
it has no claim on the service's dependency resolution, and nothing in the Go build looks in
here.

## Claims are checked

`internal/docsclaims` reads the pages under `src/content/docs` and fails `go test` if they name
an environment variable `internal/config` does not read, an HTTP route the server does not
register, or an MCP tool the server does not define, or if a page lacks a `title` and
`description`. It also checks that the two SQL blocks in `src/content/docs/install.md` marked
`<!-- sql:provision -->` and `<!-- sql:runtime-grants -->` are identical to the ones in the
repository README, which `TestRuntimeRoleCanAppendButNotRewrite` runs against Postgres. Prose
about the product is checked against the product.

## What is deliberate about it

**It wears the landing page's identity.** `src/styles/theme.css` maps the landing page's tokens
onto Starlight's gray ramp and accent, with the measured contrast of every accent pairing in a
comment at the top. `src/components/SiteTitle.astro` puts the mark beside the name, as the
landing page does. `public/favicon.svg`, `src/assets/mark.svg` and `public/og.png` are copies of
`landing/public/favicon.svg` and `landing/public/og.png`; change them there and copy them here
(`go test ./internal/web` checks the two mark copies, and the app's).

**The install commands match the landing page's.** The download-and-verify block in
`src/content/docs/install.md` is the `install` array in `landing/src/pages/index.astro`, line for
line. Change both together.

**It says only what ships.** Every page describes the current release, with the source files it
was written from; nothing here describes planned work.

## Publishing

A Cloudflare Worker with static assets (not Pages: wrangler 4.147 no longer creates Pages
projects), named `superwitness-docs`, configured by
`docs-site/wrangler.jsonc`. From the repo root:

```sh
scripts/deploy-sites.sh docs   # builds, then wrangler deploy --config docs-site/wrangler.jsonc
```

The custom domain is declared in the config, so a deploy also attaches it. It runs with the
operator's own `wrangler login`; there is no API token, in CI or anywhere else.

The site is static and has no runtime, so the deploy either served the built tree or it did not.
`curl -sI https://docs.superwitness.dev/install/` is the whole smoke test.
