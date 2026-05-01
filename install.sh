#!/usr/bin/env bash
set -euo pipefail

REPO="charlesnpx/mise-en-place"
BINARY="mise-en-place"
INSTALL_DIR="${MISE_EN_PLACE_INSTALL_DIR:-$HOME/.local/bin}"
VERSION="${MISE_EN_PLACE_VERSION:-latest}"

log() { printf '%s\n' "$*" >&2; }
fail() { log "error: $*"; exit 1; }

need() {
  command -v "$1" >/dev/null 2>&1 || fail "required command not found: $1"
}

need uname
need tar
need mktemp

OS="$(uname -s)"
ARCH="$(uname -m)"
case "$OS" in
  Darwin) OS_NAME="Darwin" ;;
  Linux) OS_NAME="Linux" ;;
  *) fail "unsupported OS: $OS (supported: macOS, Linux)" ;;
esac
case "$ARCH" in
  arm64|aarch64) ARCH_NAME="arm64" ;;
  x86_64|amd64) ARCH_NAME="x86_64" ;;
  *) fail "unsupported architecture: $ARCH (supported: arm64, x86_64)" ;;
esac

if command -v curl >/dev/null 2>&1; then
  FETCH=(curl -fsSL)
elif command -v wget >/dev/null 2>&1; then
  FETCH=(wget -qO-)
else
  fail "required command not found: curl or wget"
fi

if [[ "$VERSION" == "latest" ]]; then
  URL="https://github.com/${REPO}/releases/latest/download/${BINARY}_${OS_NAME}_${ARCH_NAME}.tar.gz"
else
  URL="https://github.com/${REPO}/releases/download/${VERSION}/${BINARY}_${OS_NAME}_${ARCH_NAME}.tar.gz"
fi

TMPDIR="$(mktemp -d)"
trap 'rm -rf "$TMPDIR"' EXIT

log "Downloading $BINARY ($OS_NAME/$ARCH_NAME) from:"
log "  $URL"
"${FETCH[@]}" "$URL" | tar -xz -C "$TMPDIR"

if [[ ! -x "$TMPDIR/$BINARY" ]]; then
  fail "release archive did not contain executable $BINARY"
fi

mkdir -p "$INSTALL_DIR"
install -m 0755 "$TMPDIR/$BINARY" "$INSTALL_DIR/$BINARY"

log "Installed $BINARY to $INSTALL_DIR/$BINARY"

case ":$PATH:" in
  *":$INSTALL_DIR:"*) ;;
  *)
    log ""
    log "Note: $INSTALL_DIR is not on your PATH. Add this to your shell profile:"
    log "  export PATH=\"$INSTALL_DIR:\$PATH\""
    ;;
esac

log ""
"$INSTALL_DIR/$BINARY" version || true
