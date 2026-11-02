#!/usr/bin/env node
// golangci-lint with a ratchet (docs/code-quality.md). `make lint` runs it.
//
// Findings are counted per linter in scripts/lint-go-baseline.json: a count may fall but never
// rise, a linter missing from the baseline must be clean, and an improvement must be locked in
// with `node scripts/lint-go.mjs --update`, which refuses to raise a count.
//
// SHIPPED_CLEAN linters must have no findings in shipped code (not a _test.go file, not under
// test/), whatever the baseline allows in tests.
//
// The linter is pinned. `make lint` installs that exact version into the ignored .bin/; by hand:
//   GOBIN="$PWD/.bin" go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.15.0
// The script uses $GOLANGCI_LINT, then .bin/golangci-lint, then golangci-lint on PATH, and
// refuses any other version: a newer linter finds more, and the counts would mean nothing.
//
// It always analyses the linux/amd64 build, which is what CI and the release run; on a Mac the
// darwin-only files would otherwise change the counts.
import { spawnSync } from 'node:child_process';
import { existsSync, readFileSync, writeFileSync } from 'node:fs';
import { dirname, join } from 'node:path';
import { fileURLToPath } from 'node:url';

export const VERSION = '2.15.0';
const root = join(dirname(fileURLToPath(import.meta.url)), '..');
const baselinePath = join(root, 'scripts/lint-go-baseline.json');
const update = process.argv.includes('--update');
// The same patterns as the Makefile's PKGS: web/node_modules can hold .go files.
const PACKAGES = ['./cmd/...', './internal/...', './test/...'];

const local = join(root, '.bin/golangci-lint');
const bin = process.env.GOLANGCI_LINT || (existsSync(local) ? local : 'golangci-lint');
const env = { ...process.env, GOOS: 'linux', GOARCH: 'amd64', CGO_ENABLED: '0' };

const version = spawnSync(bin, ['version', '--short'], { encoding: 'utf8', env });
if (version.error || version.stdout.trim().replace(/^v/, '') !== VERSION) {
  console.error(
    `lint-go needs golangci-lint ${VERSION} (found ${version.error ? 'none' : version.stdout.trim()}).\n` +
      `  GOBIN="$PWD/.bin" go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v${VERSION}`,
  );
  process.exit(2);
}

const run = spawnSync(
  bin,
  ['run', '--output.json.path=stdout', '--output.text.path=stderr', '--show-stats=false', ...PACKAGES],
  { cwd: root, encoding: 'utf8', env, maxBuffer: 256 * 1024 * 1024 },
);
if (run.error) throw run.error;
let report;
try {
  report = JSON.parse(run.stdout);
} catch {
  process.stderr.write(run.stderr || run.stdout);
  process.exit(2);
}
// Exit 1 means "issues found"; anything else is the linter failing to run.
if (run.status !== 0 && run.status !== 1) {
  process.stderr.write(run.stderr);
  process.exit(2);
}

const findings = report.Issues ?? [];
const counts = {};
for (const f of findings) counts[f.FromLinter] = (counts[f.FromLinter] ?? 0) + 1;
const baseline = JSON.parse(readFileSync(baselinePath, 'utf8'));

// Shipped code: neither a test file nor under test/ (every file there is an integration test).
const isShipped = (file) => !file.endsWith('_test.go') && !file.startsWith('test/');
const SHIPPED_CLEAN = new Set(['bodyclose', 'errcheck', 'gosec', 'nilerr', 'noctx', 'nolintlint', 'staticcheck']);
const unclean = findings.filter((f) => SHIPPED_CLEAN.has(f.FromLinter) && isShipped(f.Pos.Filename));
if (unclean.length) {
  for (const f of unclean.slice(0, 40)) console.error(`${f.FromLinter}: ${f.Pos.Filename}:${f.Pos.Line}  ${f.Text}`);
  console.error(`lint-go: ${unclean.length} findings in shipped code for linters it must keep clean`);
  process.exit(1);
}

const raised = Object.entries(counts).filter(([linter, n]) => n > (baseline[linter] ?? 0));
if (raised.length) {
  for (const [linter, n] of raised) {
    console.error(`${linter}: ${n} findings, the baseline allows ${baseline[linter] ?? 0}`);
    for (const f of findings.filter((x) => x.FromLinter === linter).slice(0, 20))
      console.error(`  ${f.Pos.Filename}:${f.Pos.Line}  ${f.Text}`);
  }
  process.exit(1);
}

const lowered = Object.keys(baseline).filter((linter) => (counts[linter] ?? 0) < baseline[linter]);
if (update) {
  const next = Object.fromEntries(Object.entries(counts).toSorted(([a], [b]) => a.localeCompare(b)));
  writeFileSync(baselinePath, JSON.stringify(next, null, 2) + '\n');
  console.log(`lint-go baseline: ${findings.length} findings across ${Object.keys(next).length} linters`);
} else if (lowered.length) {
  // A stale baseline would let the count creep back up unnoticed.
  console.error(`lint-go: ${lowered.join(', ')} improved; run node scripts/lint-go.mjs --update to lock it in`);
  process.exit(1);
} else {
  console.log(`lint-go: ok (${findings.length} findings held by the baseline)`);
}
