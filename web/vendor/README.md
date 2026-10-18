# Vendored vibekit

`superjackfruit-vibekit-0.0.13.tgz` is the vibekit design kit (Svelte components, the four vibes and
their tokens, the first-frame appearance script) at version 0.0.13, tag `v0.0.13`. It was built from
that tag with `pnpm package && pnpm pack`. `web/package.json` depends on it as
`"@superjackfruit/vibekit": "file:./vendor/superjackfruit-vibekit-0.0.13.tgz"`, so the app builds
without a registry or the vibekit repository.

`web/scripts/copy-vibekit-assets.mjs` copies the kit's `appearance-head.js` into `web/static/` before
each dev run and build, so the server sends it as a file (the Content-Security-Policy allows no
other inline script).

## Updating

1. In the vibekit repository, check out the new tag and build the tarball: `pnpm package && pnpm pack`.
2. Copy the tarball to `web/vendor/` and remove the old one.
3. In `web/package.json`, change the `file:` path to the new file name.
4. `cd web && npm install` (this updates `package-lock.json`).
5. Check everything: `npm run check && npm test && make web && make e2e` (the last two from the
   repository root). Commit the tarball, `package.json`, `package-lock.json` and the rebuilt
   `internal/web/dist` together.
