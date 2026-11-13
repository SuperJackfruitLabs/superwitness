#!/usr/bin/env bash
# Tests for docs-touched.sh: each case is changed paths, labels, and the exit code it must give.
set -uo pipefail
here=$(cd "$(dirname "$0")" && pwd)
fail=0

expect() { # expect <code> <labels-json> <paths...>
  local want=$1 labels=$2
  shift 2
  local got
  printf '%s\n' "$@" | LABELS="$labels" bash "$here/docs-touched.sh" >/dev/null 2>&1
  got=$?
  if [ "$got" -ne "$want" ]; then
    echo "FAIL: want $want, got $got: labels=$labels paths=$*"
    fail=1
  fi
}

expect 0 '[]' internal/verdicts/service.go                                 # no surface
expect 1 '[]' internal/api/server.go                                       # a route, no docs
expect 0 '[]' internal/api/server.go docs-site/src/content/docs/build/api.md
expect 0 '[]' internal/config/config.go docs/history.md
expect 0 '[]' cmd/superwitness/main.go README.md
expect 1 '[]' internal/canary/config.go internal/canary/run.go             # a setting, no docs
expect 1 '[]' web/src/routes/rubrics/+page.svelte                          # a page, no docs
expect 1 '[{"name":"bug"}]' internal/mcp/server.go                         # another label
expect 1 '["documentation"]' scripts/install.sh
expect 0 '["no-docs-needed"]' internal/mcp/server.go                       # waived
expect 0 '["bug","no-docs-needed"]' internal/contracts/run-report.schema.json
expect 0 '[]' internal/api/server_test.go internal/config/config_test.go   # tests only
expect 0 '[]' docs-site/src/content/docs/install.md                        # docs only
expect 1 '[]' internal/api/server.go docs/undocumented-baseline.json        # the baseline is not docs
expect 0 '[]'                                                              # nothing changed

[ "$fail" -eq 0 ] && echo "docs-touched: all cases pass"
exit $fail
