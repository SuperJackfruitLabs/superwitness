#!/bin/sh
# Smoke a staging Superwitness at $1 (default http://127.0.0.1:8790): health answers, the API
# refuses an unauthenticated caller, the app page loads, and sign-in goes to staging-accounts as
# the superwitness-web client. Exits non-zero on the first failure.
set -eu
base=${1:-http://127.0.0.1:8790}
issuer=${EXPECT_ISSUER:-https://staging-accounts.superjackfruit.com}
fail() { echo "smoke: $*" >&2; exit 1; }

code=$(curl -s -o /dev/null -w '%{http_code}' "$base/health")
[ "$code" = 200 ] || fail "/health answered $code"
echo "smoke: /health 200 $(curl -s "$base/health")"

code=$(curl -s -o /dev/null -w '%{http_code}' "$base/v1/runs")
[ "$code" = 401 ] || fail "/v1/runs without a token answered $code, want 401"
echo "smoke: /v1/runs unauthenticated 401"

code=$(curl -s -o /tmp/sw-smoke-page -w '%{http_code}' "$base/")
[ "$code" = 200 ] && grep -qi '<html' /tmp/sw-smoke-page || fail "/ answered $code, or not a page"
rm -f /tmp/sw-smoke-page
echo "smoke: / 200 (app page)"

loc=$(curl -s -o /dev/null -w '%{redirect_url}' "$base/auth/login?next=/")
case "$loc" in
  "$issuer"/*client_id=superwitness-web*) echo "smoke: /auth/login -> ${loc%%\?*} (client_id=superwitness-web)" ;;
  *) fail "/auth/login redirected to '$loc', want $issuer with client_id=superwitness-web" ;;
esac
