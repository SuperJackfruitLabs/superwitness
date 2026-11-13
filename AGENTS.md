# superwitness: notes for agents

Start with [`CONTRIBUTING.md`](CONTRIBUTING.md) (checks, building, testing) and [`docs/code-quality.md`](docs/code-quality.md). This repository is public: no private hosts, credentials or internal links.

## Things agents get wrong

- **Run what CI runs before pushing.** Read `.github/workflows/ci.yml` and [`CONTRIBUTING.md`](CONTRIBUTING.md): `make lint test licenses`, `make web-check`, `make test-integration`, `make e2e` rather than guessing; the commands are there.
- **Snapshots and goldens pass in a bare environment and on Linux.** Run them under `env -i PATH="$PATH" HOME="$HOME" CI=true go test ./...`. Normalise platform strings (OS, arch) and `COLORTERM` in the test instead of recording your machine's.
- **Count lint baselines on a clean install** (`npm ci` in `web/`; the Go linter version is pinned by `make lint`), never on a tree with stray packages or a stale build.
- **No skipped tests.** Never `.skip`, `.only` or delete a red test to get green; fix it, or say in the PR why it is red.
- **Characterization test first.** Before a refactor that touches behaviour, commit a test against the old code and prove it can fail (revert the code, watch it go red).
- **Check the PR is still open before pushing to its branch.** PRs are often merged within minutes, and a push to a merged branch strands the work.
- **A `v*` tag deploys production** through the suite's shared release workflow (`.github/workflows/release.yml` and `.github/release/`, synced byte-identical from the suite's release kit: change them there, never here; settings in `.github/release-config.json`; this repo's own hooks `.github/release/{build,smoke}.sh` run the scripts in `deploy/release/`). The build first checks that the binary's `--version` reports the tag. Only the operator tags.
- **The `internal/docsclaims` tests check the docs against the code:** every `SW_*` variable, `/v1` route and MCP tool a page names must exist; every route, setting, CLI command and flag, MCP tool and log event the code defines must be named on a page or listed in `docs/undocumented-baseline.json` (which only shrinks); and the tracked tree must not name private infrastructure. Change the docs with the code.
- **Integration and e2e tests need Docker** (`make test-integration`, `make e2e`). Without it they have not run, which is not a pass.
- **The Claude CI files are synced copies.** `.github/claude/claude-ci.mjs` and the `claude-review`/`claude-triage` workflows come from the suite's shared kit; change them there, not here. The advisory PR review follows [`REVIEW.md`](REVIEW.md).

