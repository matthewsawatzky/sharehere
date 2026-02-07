#!/usr/bin/env sh
set -eu

REPO="${SHAREHERE_REPO:-matthewsawatzky/sharehere}"
VERSION="${SHAREHERE_VERSION:-latest}"
INSTALL_DIR="${SHAREHERE_INSTALL_DIR:-/usr/local/bin}"
RUN_INIT="${SHAREHERE_RUN_INIT:-}"

need_cmd() {
  command -v "$1" >/dev/null 2>&1 || {
    echo "missing required command: $1" >&2
    exit 1
  }
}

need_cmd curl
need_cmd tar

if command -v sha256sum >/dev/null 2>&1; then
  SHA_CMD="sha256sum"
elif command -v shasum >/dev/null 2>&1; then
  SHA_CMD="shasum -a 256"
else
  echo "missing sha256 utility (sha256sum or shasum)" >&2
  exit 1
fi

OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
ARCH="$(uname -m)"
case "$OS" in
  linux) OS="linux" ;;
  darwin) OS="darwin" ;;
  *)
    echo "Unsupported OS for this installer: $OS" >&2
    echo "Use GitHub release artifacts for your platform." >&2
    exit 1
    ;;
esac

case "$ARCH" in
  x86_64|amd64) ARCH="amd64" ;;
  arm64|aarch64) ARCH="arm64" ;;
  *)
    echo "Unsupported architecture: $ARCH" >&2
    exit 1
    ;;
esac

if [ "$VERSION" = "latest" ]; then
  VERSION="$(curl -fsSL "https://api.github.com/repos/$REPO/releases/latest" | sed -n 's/.*"tag_name"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' | head -n1)"
  if [ -z "$VERSION" ]; then
    echo "Could not determine latest version" >&2
    exit 1
  fi
fi

VERSION_STRIPPED="${VERSION#v}"
ASSET="sharehere_${VERSION_STRIPPED}_${OS}_${ARCH}.tar.gz"
BASE_URL="https://github.com/$REPO/releases/download/$VERSION"
TMP_DIR="$(mktemp -d)"
trap 'rm -rf "$TMP_DIR"' EXIT

ARCHIVE="$TMP_DIR/$ASSET"
CHECKSUMS="$TMP_DIR/checksums.txt"

echo "Downloading $ASSET from $REPO@$VERSION"
curl -fL "$BASE_URL/$ASSET" -o "$ARCHIVE"
curl -fL "$BASE_URL/checksums.txt" -o "$CHECKSUMS"

EXPECTED="$(grep "  $ASSET$" "$CHECKSUMS" | awk '{print $1}')"
if [ -z "$EXPECTED" ]; then
  echo "Checksum entry missing for $ASSET" >&2
  exit 1
fi

ACTUAL="$(eval "$SHA_CMD \"$ARCHIVE\"" | awk '{print $1}')"
if [ "$EXPECTED" != "$ACTUAL" ]; then
  echo "Checksum mismatch" >&2
  echo "expected: $EXPECTED" >&2
  echo "actual:   $ACTUAL" >&2
  exit 1
fi

echo "Checksum verified"

tar -xzf "$ARCHIVE" -C "$TMP_DIR"
BIN="$TMP_DIR/sharehere"
if [ ! -f "$BIN" ]; then
  echo "Binary not found in archive" >&2
  exit 1
fi

mkdir -p "$INSTALL_DIR" 2>/dev/null || true
if cp "$BIN" "$INSTALL_DIR/sharehere" 2>/dev/null; then
  chmod +x "$INSTALL_DIR/sharehere"
else
  FALLBACK_DIR="$HOME/.local/bin"
  mkdir -p "$FALLBACK_DIR"
  cp "$BIN" "$FALLBACK_DIR/sharehere"
  chmod +x "$FALLBACK_DIR/sharehere"
  INSTALL_DIR="$FALLBACK_DIR"
fi

echo "Installed to $INSTALL_DIR/sharehere"

echo "Run: sharehere --help"

if [ -z "$RUN_INIT" ] && [ -t 0 ]; then
  printf "Run interactive setup now? [y/N]: "
  read -r ans
  case "$ans" in
    y|Y|yes|YES) RUN_INIT="1" ;;
    *) RUN_INIT="" ;;
  esac
fi

if [ -n "$RUN_INIT" ]; then
  "$INSTALL_DIR/sharehere" init || true
fi
