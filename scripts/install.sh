#!/bin/sh
# ⚡ redstone installer — one static binary on your PATH, verified first.
#
#   curl -fsSL https://raw.githubusercontent.com/tomblancdev/redstone/main/scripts/install.sh | sh
#
# Pin a version:     ... | sh -s -- --version v0.1.0
# Custom directory:  REDSTONE_INSTALL=$HOME/.local/bin ... | sh
# Updating IS installing: run it again, the binary is swapped (state is
# files and is never touched).
set -eu

REPO=tomblancdev/redstone
VERSION="${REDSTONE_VERSION:-}"
DEST="${REDSTONE_INSTALL:-/usr/local/bin}"

while [ $# -gt 0 ]; do
  case "$1" in
    --version) VERSION="$2"; shift 2 ;;
    --dest)    DEST="$2"; shift 2 ;;
    *) echo "unknown flag: $1 (only --version, --dest)" >&2; exit 2 ;;
  esac
done

os="$(uname -s)"
case "$os" in
  Linux) os=linux ;;
  Darwin) echo "🧨 no darwin builds yet — go install github.com/$REPO/cmd/redstone@latest (or docker)" >&2; exit 1 ;;
  *) echo "🧨 unsupported OS: $os (windows: scripts/install.ps1)" >&2; exit 1 ;;
esac
arch="$(uname -m)"
case "$arch" in
  x86_64|amd64) arch=amd64 ;;
  aarch64|arm64) arch=arm64 ;;
  *) echo "🧨 unsupported arch: $arch (amd64 and arm64 only)" >&2; exit 1 ;;
esac

if [ -z "$VERSION" ]; then
  VERSION="$(curl -fsSL "https://api.github.com/repos/$REPO/releases/latest" |
    sed -n 's/.*"tag_name": *"\([^"]*\)".*/\1/p')"
  [ -n "$VERSION" ] || { echo "🧨 could not resolve the latest version — pass --version vX.Y.Z" >&2; exit 1; }
fi

bin="redstone-$os-$arch"
base="https://github.com/$REPO/releases/download/$VERSION"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

echo "⛏ fetching redstone $VERSION ($os/$arch)"
curl -fsSL -o "$tmp/$bin" "$base/$bin"
curl -fsSL -o "$tmp/checksums.txt" "$base/checksums.txt"

# verify before placing — never trust a download you didn't test
grep "[[:space:]]$bin\$" "$tmp/checksums.txt" > "$tmp/sum"
(cd "$tmp" && sha256sum -c sum >/dev/null)
echo "⚡ checksum verified"

mkdir -p "$DEST" 2>/dev/null || true
if [ -w "$DEST" ]; then
  cp "$tmp/$bin" "$DEST/redstone" && chmod 0755 "$DEST/redstone"
elif command -v sudo >/dev/null 2>&1; then
  echo "  ($DEST needs sudo)"
  sudo cp "$tmp/$bin" "$DEST/redstone" && sudo chmod 0755 "$DEST/redstone"
else
  echo "🧨 $DEST is not writable and there is no sudo — rerun with REDSTONE_INSTALL=\$HOME/.local/bin" >&2
  exit 1
fi

echo "⚡ placed: $("$DEST/redstone" version)"
case ":$PATH:" in
  *":$DEST:"*) ;;
  *) echo "  note: $DEST is not on your PATH" ;;
esac
