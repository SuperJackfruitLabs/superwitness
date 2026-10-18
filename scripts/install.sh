#!/usr/bin/env bash
# install.sh: curl-based installer for superwitness.
#
#   curl -fsSL https://github.com/SuperJackfruitLabs/superwitness/releases/latest/download/install.sh | bash
#
# Optional env:
#   VERSION   pin a release tag (e.g. v0.0.8); default: the latest release
#   BIN_DIR   where to install; default: ~/.local/bin
#   SKILL_DIR also install the agent skill there (e.g. ~/.claude/skills), by running the freshly
#             installed `superwitness skill install --dir "$SKILL_DIR"`; omitted by default
#
# Installs one binary under two names: `superwitness`, and `suwi`, a symlink to it, as a short
# name for a person typing.
#
# superwitness is mostly a SERVER (serve, migrate, rubric-add, canary). This installer puts the
# binary in place and nothing else: it installs no service, writes no configuration and starts
# nothing. The systemd unit and env example for a server live in deploy/ in the repository.
# It never asks for sudo, and it never overwrites a program it did not install.
#
# Test-only: SUPERWITNESS_RELEASE_BASE replaces https://github.com/SuperJackfruitLabs/superwitness/releases
# so a test can serve a fake release from a local server. It must keep GitHub's layout:
# <base>/latest redirects to <base>/tag/<tag>, assets live at <base>/download/<tag>/<file>.
# Do not set it in real use.

set -euo pipefail

REPO="SuperJackfruitLabs/superwitness"
RELEASES="${SUPERWITNESS_RELEASE_BASE:-https://github.com/${REPO}/releases}"

err() { printf '%s\n' "$*" >&2; exit 1; }

# Prints "<os> <arch>" for this machine, or fails with a plain message.
detect_platform() {
  local os arch
  os="$(uname -s)"
  arch="$(uname -m)"
  case "$os" in
    Darwin) os="darwin" ;;
    Linux)  os="linux" ;;
    *) err "superwitness publishes no binary for ${os}. Build from source: make build" ;;
  esac
  case "$arch" in
    arm64|aarch64) arch="arm64" ;;
    x86_64|amd64)  arch="amd64" ;;
    *) err "superwitness publishes no binary for ${arch}." ;;
  esac
  printf '%s %s\n' "$os" "$arch"
}

# The release tag: $VERSION, or what /releases/latest redirects to. No jq, no gh.
resolve_tag() {
  if [ -n "${VERSION:-}" ]; then
    printf '%s\n' "$VERSION"
    return
  fi
  local final
  final="$(curl -fsSL -o /dev/null -w '%{url_effective}' "${RELEASES}/latest")" \
    || err "Could not resolve the latest release from ${RELEASES}/latest"
  case "$final" in
    */tag/*) printf '%s\n' "${final##*/tag/}" ;;
    *) err "Could not find a release tag in ${final}" ;;
  esac
}

tarball_name() { # tag os arch
  printf 'superwitness_%s_%s_%s.tar.gz\n' "${1#v}" "$2" "$3"
}

sha256_of() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$1" | awk '{print $1}'
  elif command -v shasum >/dev/null 2>&1; then
    shasum -a 256 "$1" | awk '{print $1}'
  else
    err "Neither sha256sum nor shasum is available; cannot verify the download."
  fi
}

# verify_checksum <sums-file> <asset-file>: the digest listed for the file's name must match.
verify_checksum() {
  local name expected actual
  name="$(basename "$2")"
  expected="$(awk -v a="$name" '$2 == a || $2 == "*" a { print $1 }' "$1")"
  [ -n "$expected" ] || err "${name} has no digest in SHA256SUMS; refusing to install it."
  actual="$(sha256_of "$2")"
  [ "$actual" = "$expected" ] || err "${name} failed its checksum; nothing was installed."
}

# A file already at one of our names is only replaced if it is ours. The binary is ours if it is
# a regular file carrying this module's path (Go embeds it); the alias if it links to superwitness.
check_binary_slot() {
  local f="$1"
  [ -e "$f" ] || [ -L "$f" ] || return 0
  if [ -f "$f" ] && [ ! -L "$f" ] && grep -aq "SuperJackfruitLabs/superwitness" "$f"; then
    return 0
  fi
  err "${f} already exists and is not a superwitness binary; refusing to overwrite it. Move it aside or set BIN_DIR."
}
check_alias_slot() {
  local f="$1" dir="$2"
  [ -e "$f" ] || [ -L "$f" ] || return 0
  if [ -L "$f" ]; then
    case "$(readlink "$f")" in
      superwitness|"${dir}/superwitness") return 0 ;;
    esac
  fi
  err "${f} already exists and is not a link to superwitness; refusing to overwrite it. Move it aside or set BIN_DIR."
}

main() {
  local bin_dir="${BIN_DIR:-$HOME/.local/bin}"
  local os arch tag asset base tmp dir

  read -r os arch <<<"$(detect_platform)"
  tag="$(resolve_tag)"
  asset="$(tarball_name "$tag" "$os" "$arch")"
  base="${RELEASES}/download/${tag}"

  check_binary_slot "${bin_dir}/superwitness"
  check_alias_slot "${bin_dir}/suwi" "$bin_dir"

  tmp="$(mktemp -d)"
  # shellcheck disable=SC2064  # expand now: $tmp is fixed
  trap "rm -rf '$tmp'" EXIT

  # A download that cannot be verified is not installed.
  curl -fsSL "${base}/SHA256SUMS" -o "${tmp}/SHA256SUMS" \
    || err "Release ${tag} publishes no SHA256SUMS; refusing to install unverified."

  if ! awk -v a="$asset" '$2 == a || $2 == "*" a { f = 1 } END { exit !f }' "${tmp}/SHA256SUMS"; then
    err "Release ${tag} has no ${os}/${arch} build (${asset} is not in its SHA256SUMS). Build from source: make build"
  fi

  printf 'Downloading %s (%s)...\n' "$asset" "$tag"
  curl -fsSL "${base}/${asset}" -o "${tmp}/${asset}" \
    || err "Release ${tag} lists ${asset} but it could not be downloaded from ${base}/${asset}"
  verify_checksum "${tmp}/SHA256SUMS" "${tmp}/${asset}"

  mkdir -p "${tmp}/unpack"
  tar -xzf "${tmp}/${asset}" -C "${tmp}/unpack"
  dir="${tmp}/unpack/${asset%.tar.gz}"
  [ -f "${dir}/superwitness" ] || err "${asset} does not contain ${asset%.tar.gz}/superwitness."

  mkdir -p "$bin_dir"
  chmod 0755 "${dir}/superwitness"
  # Rename from a staging file in the SAME directory: atomic, so a running superwitness is never
  # handed a half-written binary.
  cp "${dir}/superwitness" "${bin_dir}/.superwitness.incoming.$$"
  mv -f "${bin_dir}/.superwitness.incoming.$$" "${bin_dir}/superwitness"
  ln -s superwitness "${bin_dir}/.suwi.incoming.$$"
  mv -f "${bin_dir}/.suwi.incoming.$$" "${bin_dir}/suwi"

  printf 'Installed %s\n' "$("${bin_dir}/superwitness" --version)"

  if [ -n "${SKILL_DIR:-}" ]; then
    if "${bin_dir}/superwitness" skill install --dir "$SKILL_DIR"; then
      :
    else
      printf 'This superwitness predates the skill verb, so no skill was installed (the binary is fine).\n' >&2
    fi
  fi

  # shellcheck disable=SC2016  # $PATH is literal text for the reader to type
  case ":${PATH}:" in
    *":${bin_dir}:"*) ;;
    *) printf '\n%s is not on your PATH. Add it:\n  export PATH="%s:$PATH"\n' "$bin_dir" "$bin_dir" ;;
  esac

  printf '\nsuperwitness is a server binary. This installed the binary and the suwi alias only: no service,\nno configuration, nothing started. To run it as a server see deploy/ in the repository\n(https://github.com/%s/tree/main/deploy).\n' "$REPO"
}

# Run unless sourced (tests source it to call the functions). Under `curl | bash` BASH_SOURCE is empty.
if [ -z "${BASH_SOURCE[0]:-}" ] || [ "${BASH_SOURCE[0]}" = "$0" ]; then
  main "$@"
fi
