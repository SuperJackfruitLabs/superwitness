#!/usr/bin/env bash
# Deploy a commit of Superwitness to the staging plane, as root on workbench: run by
# /usr/local/sbin/staging-deploy (`superwitness <sha>`). Idempotent. Builds the binary for that
# commit in a golang container, makes any missing staging secret, migrates as the owner role,
# applies the README's runtime grants, recreates serve, waits for /health and runs the smoke.
#
# Made here once and kept (all under /opt/suite-staging/env/superwitness, mode 600, never printed):
#   postgres.env    the Postgres superuser password
#   db.env          SW_DATABASE_URL (superwitness_app) and SW_MIGRATE_DATABASE_URL (superwitness_owner)
#   session-secret  the login cookie key
#   access.env      SW_ALLOWED_PRINCIPALS / SW_RUN_SOURCES, from operator-principal and
#                   reporter-principal (prn_ ids at staging-accounts); delete it to rewrite it
# Minted by hand at staging-accounts (see the staging host's runbook), never here:
#   org-plane-credential  svc_…:<secret> of service `superwitness` (evidence:read, transcripts:read)
set -euo pipefail
sha=${1:-$(git -C /opt/suite-staging/superwitness rev-parse origin/main)}
cd /opt/suite-staging/superwitness
git fetch -q origin
git checkout -q --detach "$sha"
sha=$(git rev-parse HEAD)
env=/opt/suite-staging/env/superwitness
compose="docker compose -f deploy/staging/compose.yml"

[ -s "$env/org-plane-credential" ] || { echo "$env/org-plane-credential missing: mint the superwitness service at staging-accounts first"; exit 1; }

echo "== build $sha"
out=/opt/suite-staging/superwitness-bin
if [ ! -x "$out/$sha/superwitness" ]; then
  src=$(mktemp -d)
  git archive "$sha" | tar -x -C "$src"
  docker run --rm -v "$src:/src" -v superwitness-gocache:/root/.cache -w /src -e CGO_ENABLED=0 golang:1.26-bookworm \
    go build -trimpath -ldflags "-s -w -X main.version=staging-${sha:0:12}" -o /src/bin/superwitness ./cmd/superwitness
  rm -rf "$out/$sha.tmp" && mkdir -p "$out/$sha.tmp" && cp "$src/bin/superwitness" "$out/$sha.tmp/" && mv "$out/$sha.tmp" "$out/$sha"
  rm -rf "$src"
fi
ln -sfn "$sha" "$out/current"
# Keep the five newest builds. Only <sha> directories: `current` is a link to one of them.
ls -1dt "$out"/*/ | grep -E '/[0-9a-f]{40}/$' | tail -n +6 | xargs -r rm -rf

echo "== secrets"
install -d -m 700 "$env"
(
  umask 077
  [ -s "$env/postgres.env" ] || echo "POSTGRES_PASSWORD=$(openssl rand -hex 24)" > "$env/postgres.env"
  if [ ! -s "$env/db.env" ]; then
    owner=$(openssl rand -hex 24) app=$(openssl rand -hex 24)
    { echo "SW_OWNER_PASSWORD=$owner"; echo "SW_APP_PASSWORD=$app"
      echo "SW_DATABASE_URL=postgres://superwitness_app:$app@postgres:5432/superwitness?sslmode=disable"
      echo "SW_MIGRATE_DATABASE_URL=postgres://superwitness_owner:$owner@postgres:5432/superwitness?sslmode=disable"; } > "$env/db.env"
  fi
  if [ ! -s "$env/session-secret" ]; then
    secret=$(openssl rand -hex 32)
    printf '%s' "$secret" > "$env/session-secret"
  fi
  if [ ! -f "$env/access.env" ]; then
    op=$(cat "$env/operator-principal" 2>/dev/null || true) rep=$(cat "$env/reporter-principal" 2>/dev/null || true)
    { echo "SW_ALLOWED_PRINCIPALS=$op"; echo "SW_RUN_SOURCES=${rep:+$rep=superpipeline}"; } > "$env/access.env"
  fi
)
chmod 600 "$env"/*
# shellcheck disable=SC1091
. "$env/db.env"

echo "== postgres"
$compose up -d --wait postgres victoria-logs victoria-traces victoria-metrics otelcol
psql_as() { $compose exec -T postgres psql -q -v ON_ERROR_STOP=1 -U "$1" -d "$2"; }
# On stdin, so the passwords are in no process's arguments; failed statements are not logged.
psql_as postgres postgres <<SQL
SET log_min_error_statement = panic;
SELECT 'CREATE ROLE superwitness_owner LOGIN' WHERE NOT EXISTS (SELECT FROM pg_roles WHERE rolname = 'superwitness_owner')\gexec
SELECT 'CREATE ROLE superwitness_app LOGIN' WHERE NOT EXISTS (SELECT FROM pg_roles WHERE rolname = 'superwitness_app')\gexec
ALTER ROLE superwitness_owner LOGIN PASSWORD '$SW_OWNER_PASSWORD';
ALTER ROLE superwitness_app LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE PASSWORD '$SW_APP_PASSWORD';
SELECT 'CREATE DATABASE superwitness OWNER superwitness_owner' WHERE NOT EXISTS (SELECT FROM pg_database WHERE datname = 'superwitness')\gexec
REVOKE ALL ON DATABASE superwitness FROM PUBLIC;
GRANT CONNECT ON DATABASE superwitness TO superwitness_app;
\c superwitness
ALTER SCHEMA public OWNER TO superwitness_owner;
SQL

echo "== migrate"
$compose run --rm -T migrate
# The README's runtime grant block, verbatim, so a migration that adds a table and its grant
# reaches staging without editing this script.
sed -n '/<!-- sql:runtime-grants -->/,/^```$/p' README.md | grep -E '^GRANT ' | psql_as superwitness_owner superwitness

echo "== serve"
$compose up -d --force-recreate superwitness
for _ in $(seq 1 60); do curl -sf http://127.0.0.1:8790/health >/dev/null && break; sleep 1; done
sh deploy/staging/smoke.sh http://127.0.0.1:8790
git log -1 --format='staged %h %s'
