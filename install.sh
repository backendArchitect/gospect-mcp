#!/usr/bin/env bash
# gospect-mcp installer — downloads the latest release binary for your platform.
#
#   curl -fsSL https://raw.githubusercontent.com/backendArchitect/gospect-mcp/main/install.sh | bash
#
# Override the install dir with GOSPECT_INSTALL_DIR (default: /usr/local/bin, falls back to
# ~/.local/bin if that isn't writable). Override the version with GOSPECT_VERSION (default: latest).
set -euo pipefail

REPO="backendArchitect/gospect-mcp"
BIN="gospect-mcp"

os="$(uname -s | tr '[:upper:]' '[:lower:]')"
case "$os" in
  linux | darwin) ;;
  *) echo "gospect-mcp: unsupported OS '$os' (Linux and macOS only; on Windows use install.ps1)" >&2; exit 1 ;;
esac

arch="$(uname -m)"
case "$arch" in
  x86_64 | amd64) arch="amd64" ;;
  aarch64 | arm64) arch="arm64" ;;
  *) echo "gospect-mcp: unsupported architecture '$arch'" >&2; exit 1 ;;
esac

tag="${GOSPECT_VERSION:-}"
if [ -z "$tag" ]; then
  tag="$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" | grep -m1 '"tag_name"' | cut -d '"' -f4)"
fi
if [ -z "$tag" ]; then
  echo "gospect-mcp: could not determine the latest release (no releases published yet?)" >&2
  exit 1
fi

asset="${BIN}_${tag}_${os}_${arch}.tar.gz"
url="https://github.com/${REPO}/releases/download/${tag}/${asset}"

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
echo "gospect-mcp: downloading ${tag} for ${os}/${arch} …"
curl -fsSL "$url" -o "$tmp/$asset"
tar -xzf "$tmp/$asset" -C "$tmp"

# Pick an install dir we can write to.
dir="${GOSPECT_INSTALL_DIR:-/usr/local/bin}"
if [ ! -d "$dir" ] || [ ! -w "$dir" ]; then
  if [ -w "${dir%/*}" ] 2>/dev/null; then :; else
    dir="$HOME/.local/bin"
    mkdir -p "$dir"
  fi
fi

if [ -w "$dir" ]; then
  install -m 0755 "$tmp/$BIN" "$dir/$BIN"
else
  echo "gospect-mcp: $dir needs elevated permissions; using sudo"
  sudo install -m 0755 "$tmp/$BIN" "$dir/$BIN"
fi

echo "gospect-mcp: installed to $dir/$BIN"
case ":$PATH:" in
  *":$dir:"*) ;;
  *) echo "gospect-mcp: add $dir to your PATH to use it directly" ;;
esac
"$dir/$BIN" version 2>/dev/null || true
