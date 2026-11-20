#!/usr/bin/env bash
# Install r9000p-fan-manager (binary: fan).
#
# One-liner:
#   curl -fsSL https://raw.githubusercontent.com/crayonlu/r9000p-fan-manager/main/install.sh | sudo bash
#
# Different repo:
#   curl -fsSL https://raw.githubusercontent.com/OWNER/REPO/main/install.sh | sudo REPO=OWNER/REPO bash
set -euo pipefail

REPO="${REPO:-crayonlu/r9000p-fan-manager}"
BIN="/usr/local/bin/fan"

if [ "$(id -u)" -ne 0 ]; then
  echo "Please run as root:  curl ... | sudo bash" >&2
  exit 1
fi

arch="$(uname -m)"
case "$arch" in
  x86_64|amd64) target=amd64 ;;
  aarch64|arm64) target=arm64 ;;
  *) echo "Unsupported architecture: $arch" >&2; exit 1 ;;
esac

url="https://github.com/${REPO}/releases/latest/download/fan-linux-${target}"
echo "Downloading ${url}"
tmp="$(mktemp)"
trap 'rm -f "$tmp"' EXIT
curl -fL --retry 3 -o "$tmp" "$url"
install -m 0755 "$tmp" "$BIN"

echo "Installed: ${BIN}"
"$BIN" version || true
echo "Try:  fan status   |   fan   (opens the TUI)"
