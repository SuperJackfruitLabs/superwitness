# Code quality checks

Each check below records how the code looks today and fails if it gets worse. Each one also
fails if it gets **better** without the record being lowered, so a stale record cannot let the
next regression through. The numbers live in the files this page cites, not here.

| Check | Run by | CI job |
|---|---|---|
| Go lint ratchet | `make lint` | `go` |
| Web lint ratchet | `make web` (`npm run lint` in `web/`) | `web` |
| Go architecture test | `make test` (`internal/archtest`) | `go` |

## Go lint

[golangci-lint](https://golangci-lint.run) `v2.15.0`, pinned in the `Makefile` and in
[`scripts/lint-go.mjs`](../scripts/lint-go.mjs), which refuses any other version (a newer linter
finds more, and the counts would mean nothing). `make lint` runs `go vet`, installs that exact
version into the ignored `.bin/` on first use, then runs the script. Configuration:
[`.golangci.yml`](../.golangci.yml). The script always analyses the linux/amd64 build with the
`integration` tag, so a Mac and CI count the same files.

- **Linters:** `errcheck`, `govet`, `staticcheck`, `ineffassign`, `unused`, `gosec`, `errorlint`,
  `bodyclose`, `nilerr`, `noctx`, `misspell`, `unconvert`, `copyloopvar` and `nolintlint`.
  `revive` and `gocritic` were measured and left off: almost all of it is style.
- **The ratchet.** Findings are counted per linter in
  [`scripts/lint-go-baseline.json`](../scripts/lint-go-baseline.json). A count may fall but never
  rise, and a linter missing from the baseline must be clean. After a cleanup,
  `node scripts/lint-go.mjs --update` writes the lower counts; lint fails until you do.
- **Shipped code stays clean** of `bodyclose`, `errcheck`, `gosec`, `nilerr`, `noctx`,
  `nolintlint` and `staticcheck` (`SHIPPED_CLEAN` in the script), whatever the baseline allows in
  tests. Its only held findings are `errorlint`'s: errors formatted with `%v` where `%w` would
  change what `errors.Is` matches, so each needs a decision, not a mechanical fix.
- **A deliberate exception is written down.** An error the code means to drop is dropped with
  `_ =` and a comment saying why. Where a linter is wrong about a line, `//nolint:<linter> // why`
  on that line; `nolintlint` rejects a directive that does not name its linter or give a reason.

## Web lint

[oxlint](https://oxc.rs) `1.88.0` with `oxlint-tsgolint` `7.0.2004`, both pinned exactly in
`web/package.json`, type-aware. Configuration: [`web/.oxlintrc.json`](../web/.oxlintrc.json).
Script: [`web/scripts/lint.mjs`](../web/scripts/lint.mjs). It covers `web/src` (TypeScript, and the
script blocks of Svelte files), the e2e tests, `web/scripts` and the vitest setup; the vendored
design kit and build configuration are left out.

- **Rules:** the `correctness` and `suspicious` categories, plus floating and misused promises,
  `await` of a non-promise, exhaustive `switch`, type-only imports, no `any`, no `!` non-null
  assertion, no empty block, and no `console` outside scripts and tests. A CSS side-effect import
  is allowed.
- **The ratchet** works as the Go one does, per rule, in
  [`web/scripts/lint-baseline.json`](../web/scripts/lint-baseline.json);
  `npm run lint -- --update` locks in an improvement. The counts are what a clean `npm ci` sees.
- **Shipped source stays clean** of the rules in `SRC_CLEAN` in the script, among them `any`,
  `!` assertions, empty blocks, floating and misused promises and unused code. "Shipped" means
  under `web/src` and not a test file.
- **Before running it by hand,** run `npm run check` (or `npx svelte-kit sync`): `tsconfig.json`
  extends `.svelte-kit/`, and without it the typed rules report `tsconfig-error`.
- Where a rule is wrong about a line, disable it for that line only:
  `// oxlint-disable-next-line <rule> -- <why>`.

## Go architecture

[`internal/archtest`](../internal/archtest/archtest_test.go) reads every shipped `.go` file under
`cmd/` and `internal/` (test files and `testdata` are left out).

**Size limits.** 1,000 lines per file and 4,000 lines per package. A file or package over its
limit needs an entry in `oversizeFiles` or `oversizePackages`, which holds it at its current
size. Both are empty; when an entry's code shrinks, the test asks for the entry to be lowered.
Split the file or move code out rather than adding one.

**Import direction.** The packages sit in layers, lowest first (`layers` in the test). A package
imports only from a layer below its own; a new package fails the test until it is given a layer.

| Layer | Packages under `internal/` |
|---|---|
| Foundation | `auth`, `config`, `contracts`, `ratelimit`, `web` |
| The source contract | `source` |
| The sources | `source/agentpod`, `source/fake`, `source/logs`, `source/superpipeline`, `source/traces` |
| Derived sources | `source/errors` (read out of the logs) |
| Domain | `verdicts`, `telemetry`, `canary`, `session` |
| Domain over verdicts | `runs`, `join` |
| HTTP API | `api` |
| MCP server | `mcp` (built on the API) |
| Composition root | `app` |
| The binary | `cmd/superwitness` (not under `internal/`) |

Three more rules: only the composition root and the binary import a concrete source, everything
else goes through `internal/source`; nothing imports `cmd/`, tests included; and no shipped
package imports `internal/testutil` or `internal/docsclaims`, which serve tests. Tests may
otherwise import any package.

When a lower layer needs something from a higher one, move the shared piece down to the layer
that needs it, or pass a narrow interface in from the composition root. Do not reorder the layers
to make an import fit.

## Practices

- **Characterize before changing behaviour.** A change that touches what the code does gets a
  test first that pins the current behaviour, and that test is seen to fail against a planted
  change. A test that cannot fail proves nothing.
- **Revert-proof every guard.** A new check is seen to fail: plant the regression it is meant to
  catch, watch it go red, then remove the plant.
- **Move, then fix.** A split moves code verbatim; behaviour changes go in their own change.
