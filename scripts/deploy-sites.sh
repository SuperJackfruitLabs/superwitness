#!/usr/bin/env bash
# Build superwitness.dev and docs.superwitness.dev and deploy both as Cloudflare
# Workers with static assets.
#
# Run from the operator's machine with an existing `wrangler login`; no API token is
# created or stored anywhere. Each site's wrangler.jsonc names its Worker and its
# custom domains, so this attaches the domains too.
#
# --config is explicit and wrangler runs from a scratch directory: in a site directory
# without a config, wrangler's project detection scaffolds an adapter into the site and
# rewrites astro.config.mjs (superpipeline learned this against the live account).
#
# Usage: scripts/deploy-sites.sh [landing|docs|all]   (default: all)
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
which="${1:-all}"
case "$which" in landing|docs|all) ;; *) echo "usage: $0 [landing|docs|all]" >&2; exit 2 ;; esac

scratch="$(mktemp -d)"
trap 'rm -rf "$scratch"' EXIT

ship() { # <dir> <url>
  echo "==> building $1"
  (cd "$root/$1" && npm ci && npm run build)
  echo "==> deploying $1 -> $2"
  (cd "$scratch" && npx --yes wrangler@4 deploy --config "$root/$1/wrangler.jsonc")
  echo "==> $1 deployed: $2"
}

if [ "$which" = landing ] || [ "$which" = all ]; then ship landing https://superwitness.dev; fi
if [ "$which" = docs ] || [ "$which" = all ]; then ship docs-site https://docs.superwitness.dev; fi
