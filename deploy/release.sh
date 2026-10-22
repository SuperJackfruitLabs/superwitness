#!/usr/bin/env bash
# Deploy a release tag of Superwitness to production, as root on foundry: run by
# /usr/local/sbin/prod-deploy (`superwitness v<x.y.z>`), which takes this script from the tag itself
# (estate provision/303-prod-deploy-foundry.sh). The manual fallback is estate bin/ship-superwitness
# plus provision/260-superwitness.sh.
#
# In order: download the tag's linux_amd64 tarball and SHA256SUMS from the GitHub release, verify
# the checksum, unpack it into its own directory /opt/superwitness/releases/<tag>, check the binary
# reports the tag, back up the database (pg_dump of superwitness-pg), migrate as the owner role,
# apply the README's runtime grants, point the service at the release with a systemd drop-in,
# restart, wait for /health to report the tag, and run the smoke. Any failure after the switch
# puts the previous unit and drop-in back and restarts, so the previous release serves again.
#
# What a rollback does not undo: migrations are forward-only. The dump taken before them is in
# /opt/superwitness/backups (kept 14 days) for a manual pg_restore.
#
# SW_RELEASE_CHECK=1 stops before anything changes: download, checksum, unpack, version, the
# backup, and /health of the running release. Nothing is migrated, switched or restarted.
set -euo pipefail

REPO=SuperJackfruitLabs/superwitness
ENV_FILE=${SW_ENV_FILE:-/etc/superwitness/env}

# /etc/superwitness/env is a systemd EnvironmentFile, not shell: never source it (agentpod #762).
# Read only the keys this script needs, as systemd would: the first `KEY=` line, the rest of the
# line verbatim, one pair of surrounding quotes removed.
env_get() {
  local v
  v=$(grep -m1 "^$1=" "$ENV_FILE" | cut -d= -f2-) || true
  v=${v%\"}; v=${v#\"}; v=${v%\'}; v=${v#\'}
  printf '%s' "$v"
}
# Sourced by deploy/release.test.sh for env_get alone.
if [ "${SW_RELEASE_LIB:-}" = 1 ]; then return 0; fi

tag=${1:-}
[[ $tag =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]] || { echo "usage: release.sh v<x.y.z>" >&2; exit 2; }
ver=${tag#v}
name=superwitness_${ver}_linux_amd64
base=/opt/superwitness
releases=$base/releases
backups=$base/backups
dir=$releases/$tag
unit=/etc/systemd/system/superwitness.service
dropin_dir=/etc/systemd/system/superwitness.service.d
dropin=$dropin_dir/60-release.conf
PG=superwitness-pg
stamp=$(date -u +%Y%m%dT%H%M%SZ)
log() { echo "== $*"; }

exec 9>/run/lock/superwitness-release.lock
flock -n 9 || { echo "release: another superwitness release is running" >&2; exit 1; }

TS_IP=$(tailscale ip -4 2>/dev/null | head -1) || TS_IP=""
[ -n "$TS_IP" ] || { echo "release: not on the tailnet" >&2; exit 1; }
health_url=http://$TS_IP:8790/health
issuer=$(env_get SW_ORG_PLANE_ISSUER)
migrate_url=$(env_get SW_MIGRATE_DATABASE_URL)
[ -n "$issuer" ] && [ -n "$migrate_url" ] || { echo "release: SW_ORG_PLANE_ISSUER or SW_MIGRATE_DATABASE_URL missing from $ENV_FILE" >&2; exit 1; }

# The service runs as `superwitness` and must reach the binary: traverse-only on the base,
# readable releases, backups root-only.
install -d -m 0711 "$base"
install -d -m 0755 "$releases"
install -d -m 0700 "$backups"

work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT

log "fetch $tag"
if [ -f "$dir/.release-ok" ] && [ -x "$dir/superwitness" ]; then
  echo "   $dir already unpacked and verified"
else
  url=https://github.com/$REPO/releases/download/$tag
  curl -fsSL --retry 3 -o "$work/$name.tar.gz" "$url/$name.tar.gz"
  curl -fsSL --retry 3 -o "$work/SHA256SUMS" "$url/SHA256SUMS"
  expected=$(awk -v f="$name.tar.gz" '$2 == f || $2 == "*" f { print $1 }' "$work/SHA256SUMS")
  actual=$(sha256sum "$work/$name.tar.gz" | cut -d' ' -f1)
  [ -n "$expected" ] && [ "$actual" = "$expected" ] || { echo "release: CHECKSUM MISMATCH for $name.tar.gz — nothing installed" >&2; exit 1; }
  echo "   sha256 $actual matches SHA256SUMS"
  tar --no-same-owner -xzf "$work/$name.tar.gz" -C "$work"
  { [ -x "$work/$name/superwitness" ] && [ -f "$work/$name/deploy/superwitness.service" ]; } \
    || { echo "release: tarball lacks superwitness or deploy/superwitness.service" >&2; exit 1; }
  rm -rf "$dir.tmp" "$dir"
  mv "$work/$name" "$dir.tmp"
  cp "$work/$name.tar.gz" "$work/SHA256SUMS" "$dir.tmp/"
  chmod -R u=rwX,go=rX "$dir.tmp"
  touch "$dir.tmp/.release-ok"
  mv "$dir.tmp" "$dir"
fi
got=$("$dir/superwitness" --version)
[ "$got" = "$tag" ] || { echo "release: $dir/superwitness reports $got, not $tag" >&2; exit 1; }

log "backup"
dump=$backups/db-$stamp-before-$tag.dump
docker exec "$PG" pg_dump -Fc -U postgres superwitness > "$dump.part"
[ -s "$dump.part" ] || { rm -f "$dump.part"; echo "release: pg_dump produced an empty file" >&2; exit 1; }
mv "$dump.part" "$dump"
chmod 600 "$dump"
echo "   $dump ($(stat -c %s "$dump") bytes)"
find "$backups" -maxdepth 1 -name 'db-*.dump' -mtime +14 -delete

if [ "${SW_RELEASE_CHECK:-}" = 1 ]; then
  log "check only"
  echo "   running: $(curl -fsS -m 5 "$health_url")"
  echo "check passed for $tag: fetched, verified, unpacked, backed up; nothing switched"
  exit 0
fi

# The previous unit and drop-in, put back by a rollback.
cp "$unit" "$work/prev.unit"
[ -f "$dropin" ] && cp "$dropin" "$work/prev.dropin"
prev_exec=$(systemctl show -p ExecStart --value superwitness | sed -n 's/.*path=\([^ ;]*\).*/\1/p')
echo "   previous: ${prev_exec:-unknown}"

rollback() {
  trap - ERR
  echo "release: $tag failed ($1); restoring the previous release" >&2
  cp "$work/prev.unit" "$unit"
  if [ -f "$work/prev.dropin" ]; then cp "$work/prev.dropin" "$dropin"; else rm -f "$dropin"; fi
  systemctl daemon-reload
  systemctl restart superwitness
  for _ in $(seq 1 30); do
    if curl -fsS -m 5 "$health_url" >/dev/null 2>&1; then echo "release: previous release healthy again" >&2; exit 1; fi
    sleep 2
  done
  echo "release: ROLLBACK DID NOT COME BACK HEALTHY — journalctl -u superwitness" >&2
  exit 1
}
trap 'rollback "line $LINENO exited $?"' ERR

log "migrate"
# With SW_MIGRATE_DATABASE_URL alone in its environment, exported rather than on a command line.
(
  while read -r v; do unset "$v" 2>/dev/null || true; done < <(compgen -e)
  export SW_MIGRATE_DATABASE_URL="$migrate_url"
  exec "$dir/superwitness" migrate
)

log "runtime grants"
# The README's runtime grant block at this tag, so a migration that adds a table and its grant
# reaches production without editing this script.
curl -fsSL --retry 3 -o "$work/README.md" "https://raw.githubusercontent.com/$REPO/refs/tags/$tag/README.md"
sed -n '/<!-- sql:runtime-grants -->/,/^```$/p' "$work/README.md" | grep -E '^GRANT ' > "$work/grants.sql"
[ -s "$work/grants.sql" ] || rollback "no runtime grants in README at $tag"
docker exec -i "$PG" psql -q -v ON_ERROR_STOP=1 -U superwitness_owner -d superwitness < "$work/grants.sql"

log "switch to $dir"
install -m 0644 "$dir/deploy/superwitness.service" "$unit"
install -d -m 0755 "$dropin_dir"
printf '# Written by superwitness deploy/release.sh (%s).\n[Service]\nExecStart=\nExecStart=%s/superwitness serve\n' "$tag" "$dir" > "$dropin"
systemctl daemon-reload
systemctl restart superwitness

log "health"
up=""
for _ in $(seq 1 60); do
  if body=$(curl -fsS -m 5 "$health_url" 2>/dev/null) && [[ $body == *"\"version\":\"$tag\""* ]]; then up=1; break; fi
  sleep 2
done
[ -n "$up" ] || rollback "no /health reporting $tag after 120s"
echo "   $body"

log "smoke"
curl -fsSL --retry 3 -o "$work/smoke.sh" "https://raw.githubusercontent.com/$REPO/refs/tags/$tag/deploy/staging/smoke.sh"
EXPECT_ISSUER=$issuer sh "$work/smoke.sh" "http://$TS_IP:8790" || rollback "smoke failed"
trap - ERR

# Keep the five newest releases; never the one now live.
# shellcheck disable=SC2010  # release directories are v<x.y.z>, nothing else
ls -1dt "$releases"/v*/ 2>/dev/null | grep -Ev "/$tag/$" | tail -n +5 | xargs -r rm -rf
echo "released $tag"
