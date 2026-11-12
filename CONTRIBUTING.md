# Contributing

superwitness lives at https://github.com/SuperJackfruitLabs/superwitness.
Documentation is at https://docs.superwitness.dev.

## Branch flow

There's a single maintainer, and work happens on a trunk. Changes land on
**`main`** through a pull request, gated by the required checks rather than by
review. Each check is a job in `.github/workflows/ci.yml`:

| Check | What it runs |
|---|---|
| `go` | `go vet` and the golangci-lint ratchet, unit tests (the architecture test among them), the licence check, and the docs-claims and internal-reference guards |
| `integration` | tests against real Postgres, VictoriaTraces and VictoriaLogs containers |
| `web` | the embedded SvelteKit app's type check, lint ratchet, tests and build, and a check that the committed bundle is current |
| `landing` | the superwitness.dev build |
| `docs` | the docs.superwitness.dev build |
| `e2e` | the app in Chromium against fake mode, a stand-in hub and Postgres |

Branch protection requires all six to pass and the branch to be up to date
with `main`, so rebase before merging. Security issues don't go through pull
requests; see [SECURITY.md](SECURITY.md).

## Building and testing

You need Go (the version is in `go.mod`), Node, and Docker for the integration
tests.

```sh
make build              # bin/superwitness
make lint test          # vet, the lint ratchet, and unit tests, including the guards
make test-integration   # needs Docker
make web                # rebuilds the SvelteKit app into internal/web/dist
make web-check          # the same, then fails if the committed bundle changed
make licenses           # dependency licence allowlist
make e2e                # needs Docker, and npx playwright install chromium once
```

The app in `web/` is SvelteKit with Svelte 5 and the vendored vibekit design kit
(see [web/vendor/README.md](web/vendor/README.md)). Its build output is committed in
`internal/web/dist` and embedded in the binary, so change `web/src`, run `make web`, and commit
both. The browser tests are described in the README's web section.

To run either site locally:

```sh
cd landing && npm ci && npm run dev      # http://localhost:4327
cd docs-site && npm ci && npm run dev    # http://localhost:4328
```

## Guards that will stop your change

- **Docs claims** (`internal/docsclaims`). A docs or site page that names a
  `SW_*` variable, a `/v1` route or an MCP tool the code doesn't define fails
  the build, as does a page missing a title or description. Fix the page or
  the code; don't silence the check.
- **Internal references** (`internal/docsclaims`, the internal-refs test).
  Every tracked file is scanned for identifiers that don't belong in a public
  repository, such as private host names, private-network addresses and
  real-looking principal or board ids. Test fixtures use obviously fake values
  such as `prn_human01` and `brd_01`.
- **Code quality** (`make lint`, `make web`, `internal/archtest`). Lint
  findings are held per linter by a ratchet that may fall but never rise,
  shipped code stays clean of the linters it is clean of now, files and
  packages have size limits, and imports point down the package layers. See
  [docs/code-quality.md](docs/code-quality.md).

## Dependencies

Anything superwitness links or ships must be permissively licensed (MIT,
Apache-2.0, BSD, ISC). There's no AGPL and no source-available licences such
as ELv2, BSL, FSL or SSPL. `make licenses` enforces this for Go modules.
Credit bundled components in [NOTICE](NOTICE). Dependabot opens update pull
requests weekly.

## Releases

Maintainers only. Tagging `v*` on `main` runs the suite's shared release workflow,
`.github/workflows/release.yml` (synced from the suite's release kit; settings in
`.github/release-config.json`). It refuses a tag whose commit is not on `main` or whose `main`
CI run did not pass, builds the linux and darwin (amd64, arm64) tarballs at the tag
(`deploy/release/build.sh`, which checks that the binary reports the tag), writes `SHA256SUMS`
and a provenance attestation, deploys production and publishes the GitHub release, its notes
quoting this version's `CHANGELOG.md` section. Before the tag, a release-prep PR runs
`sh .github/release/prep-release.sh vX.Y.Z` and updates the version in the docs and install
snippets; the docs-claims test keeps them consistent.

The two sites deploy on every merge to main that touches them:
`.github/workflows/sites.yml` runs `scripts/deploy-sites.sh` with the organisation's
`CLOUDFLARE_API_TOKEN`, in a `websites` environment that only accepts main. No
credentials are stored in the repository.
