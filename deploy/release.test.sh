#!/usr/bin/env bash
# Tests for deploy/release.sh that need no foundry: env_get reads an EnvironmentFile as systemd
# does (never by sourcing it), and the script refuses anything but v<x.y.z>.
set -euo pipefail
here=$(cd "$(dirname "$0")" && pwd)
tmp=$(mktemp -d); trap 'rm -rf "$tmp"' EXIT
fail() { echo "FAIL: $*" >&2; exit 1; }

cat > "$tmp/env" <<'ENV'
SW_LISTEN=100.1.2.3:8790
SW_MIGRATE_DATABASE_URL=postgres://o:p@127.0.0.1:5433/superwitness?sslmode=disable
SW_QUOTED="has spaces; touch /tmp/sw-release-test-pwned"
SW_LISTEN=second-ignored
ENV
# shellcheck disable=SC1091
SW_RELEASE_LIB=1 SW_ENV_FILE="$tmp/env" . "$here/release.sh"
[ "$(env_get SW_LISTEN)" = 100.1.2.3:8790 ] || fail "first line wins"
[ "$(env_get SW_MIGRATE_DATABASE_URL)" = "postgres://o:p@127.0.0.1:5433/superwitness?sslmode=disable" ] || fail "value with = and ?"
[ "$(env_get SW_QUOTED)" = "has spaces; touch /tmp/sw-release-test-pwned" ] || fail "quotes stripped, nothing run"
[ ! -e /tmp/sw-release-test-pwned ] || fail "a value was executed"
[ -z "$(env_get SW_MISSING)" ] || fail "missing key is empty"

for bad in "" v1.2 1.2.3 "v1.2.3 extra" "v1.2.3;id" v1.2.3-rc1; do
  code=0; bash "$here/release.sh" "$bad" >/dev/null 2>&1 || code=$?
  [ "$code" = 2 ] || fail "tag '$bad' exited $code, want 2"
done
echo "release.test.sh: ok"
