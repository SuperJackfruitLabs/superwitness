#!/bin/sh
# The release's build (`build_command` in .github/release-config.json), run at the tag: the
# release tarballs for linux and darwin, amd64 and arm64, with the tag stamped in. The tag is
# the version, so the tarballs cannot come from main's CI. The web app is the committed
# internal/web/dist, which main's CI (`make web-check`) proved matches web/.
set -eu
: "${TAG:?TAG is the release tag}"
make dist VERSION="$TAG"
# A binary whose -X stamp silently failed would report a development version.
ver=${TAG#v}
check=$(mktemp -d)
tar -xzf "dist/superwitness_${ver}_linux_amd64.tar.gz" -C "$check"
got=$("$check/superwitness_${ver}_linux_amd64/superwitness" --version)
rm -rf "$check"
[ "$got" = "$TAG" ] || { echo "the built binary reports $got, not $TAG" >&2; exit 1; }
echo "build: superwitness $got"
