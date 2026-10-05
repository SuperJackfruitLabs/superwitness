#!/bin/sh
# Runs the built binary for the end-to-end test: fake sources, sign-in through the stub hub,
# and a development reporter. E2E_DATABASE_URL is a throwaway Postgres (make e2e starts one).
set -eu
: "${E2E_DATABASE_URL:?set E2E_DATABASE_URL, or run make e2e}"
dir=$(mktemp -d)
umask 077
head -c 48 /dev/urandom | base64 > "$dir/session-secret"
export SW_FAKE_SOURCES=1 SW_LISTEN=127.0.0.1:8790 SW_PUBLIC_URL=http://127.0.0.1:8790 \
  SW_HUB_URL=http://127.0.0.1:8791 SW_APP_CLIENT_ID=superwitness-console SW_ALLOWED_PRINCIPALS=prn_human01 \
  SW_SESSION_SECRET_FILE="$dir/session-secret" SW_RUN_SOURCES=prn_reporter01=superpipeline \
  SW_DATABASE_URL="$E2E_DATABASE_URL"
exec ../bin/superwitness serve
