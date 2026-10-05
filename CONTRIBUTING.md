# Contributing

superwitness lives at https://github.com/SuperJackfruitLabs/superwitness.
Documentation is at https://docs.superwitness.dev.

## Branch flow

There's a single maintainer, and work happens on a trunk. Changes land on
**`main`** through a pull request, gated by the required checks rather than by
review. Each check is a job in `.github/workflows/ci.yml`:

| Check | What it runs |
|---|---|
| `go` | `go vet`, unit tests, the licence check, and the docs-claims and internal-reference guards |
| `integration` | tests against real Postgres, VictoriaTraces and VictoriaLogs containers |
| `web` | the embedded run page's tests and build, and a check that the committed bundle is current |
| `landing` | the superwitness.dev build |
| `docs` | the docs.superwitness.dev build |

Branch protection requires all five to pass and the branch to be up to date
with `main`, so rebase before merging. Security issues don't go through pull
requests; see [SECURITY.md](SECURITY.md).

## Building and testing

You need Go (the version is in `go.mod`), Node, and Docker for the integration
tests.

```sh
make build              # bin/superwitness
make lint test          # vet and unit tests, including the guards
make test-integration   # needs Docker
make web-check          # rebuilds the run page and checks the committed bundle
make licenses           # dependency licence allowlist
```

To run either site locally:

```sh
cd landing && npm ci && npm run dev      # http://localhost:4327
cd docs-site && npm ci && npm run dev    # http://localhost:4328
```

## Two guards that will stop your change

- **Docs claims** (`internal/docsclaims`). A docs or site page that names a
  `SW_*` variable, a `/v1` route or an MCP tool the code doesn't define fails
  the build, as does a page missing a title or description. Fix the page or
  the code; don't silence the check.
- **Internal references** (`internal/docsclaims`, the internal-refs test).
  Every tracked file is scanned for identifiers that don't belong in a public
  repository, such as private host names, private-network addresses and
  real-looking principal or board ids. Test fixtures use obviously fake values
  such as `prn_human01` and `brd_01`.

## Dependencies

Anything superwitness links or ships must be permissively licensed (MIT,
Apache-2.0, BSD, ISC). There's no AGPL and no source-available licences such
as ELv2, BSL, FSL or SSPL. `make licenses` enforces this for Go modules.
Credit bundled components in [NOTICE](NOTICE). Dependabot opens update pull
requests weekly.

## Releases

Maintainers only. Tagging `v*` on `main` triggers
`.github/workflows/release.yml`. It builds the linux/amd64 and linux/arm64
tarballs and `SHA256SUMS`, checks that the binary reports the tag, and
publishes a GitHub release. Update `CHANGELOG.md` and the version in the docs
and install snippets in the same change; the docs-claims test keeps them
consistent.

The two sites deploy from a maintainer's machine with `scripts/deploy-sites.sh`,
using a `wrangler login`. No deploy credentials are stored in the repository or
in CI.
