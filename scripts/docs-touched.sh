#!/usr/bin/env bash
# docs-touched.sh: fail a pull request that changes the public surface without touching docs.
#
#   git diff --name-only BASE...HEAD | LABELS='["no-docs-needed"]' scripts/docs-touched.sh
#
# Reads the changed paths on stdin and the PR's label names, as a JSON array, from LABELS.
# Exits 0 when no public-surface file changed, when a docs file changed too, or when the PR has
# the no-docs-needed label; otherwise 1, naming the surface files. scripts/docs-touched.test.sh
# holds it to that.
set -euo pipefail

# The public surface: routes, settings, the CLI, the MCP tools, the published contracts, the
# installer and the app's pages. internal/docsclaims checks the names; this checks the habit.
is_surface() {
  case "$1" in
    internal/api/server.go | internal/session/login.go) return 0 ;; # HTTP routes
    internal/config/*.go | internal/canary/config.go) return 0 ;;   # settings
    cmd/superwitness/*.go | internal/canary/cmd.go) return 0 ;;     # the CLI
    internal/mcp/*.go) return 0 ;;                                  # MCP tools
    internal/contracts/*) return 0 ;;                               # report and document schemas
    scripts/install.sh | deploy/env.example | deploy/canary/canary.env.example) return 0 ;;
    web/src/routes/*) return 0 ;;                                   # the app's pages
  esac
  return 1
}

is_docs() {
  case "$1" in
    docs/undocumented-baseline.json) return 1 ;; # listing a new item as undocumented is not documenting it
    docs-site/* | docs/* | README.md) return 0 ;;
  esac
  return 1
}

surface=()
docs=0
while IFS= read -r path; do
  [ -n "$path" ] || continue
  case "$path" in *_test.go) continue ;; esac # a test changes no surface
  if is_docs "$path"; then docs=1; fi
  if is_surface "$path"; then surface+=("$path"); fi
done

if [ "${#surface[@]}" -eq 0 ]; then
  echo "docs-touched: no public-surface file changed"
  exit 0
fi
if [ "$docs" -eq 1 ]; then
  echo "docs-touched: public surface changed and docs changed with it"
  exit 0
fi
if printf '%s' "${LABELS:-[]}" | grep -q '"no-docs-needed"'; then
  echo "docs-touched: public surface changed; the no-docs-needed label waives docs"
  exit 0
fi
echo "docs-touched: these public-surface files changed, and no docs (docs-site/, docs/, README.md) did:" >&2
printf '  %s\n' "${surface[@]}" >&2
echo "Update the docs, or add the no-docs-needed label if nothing a reader sees changed." >&2
exit 1
