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
need_cmd go

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

TMP_DIR="$(mktemp -d)"
trap 'rm -rf "$TMP_DIR"' EXIT

ARCHIVE="$TMP_DIR/source.tar.gz"
SRC_ROOT="$TMP_DIR/src"
BUILD_DIR="$TMP_DIR/build"
mkdir -p "$SRC_ROOT" "$BUILD_DIR"

echo "Downloading source for $REPO@$VERSION"
curl -fL "https://github.com/$REPO/archive/refs/tags/$VERSION.tar.gz" -o "$ARCHIVE"
tar -xzf "$ARCHIVE" -C "$SRC_ROOT"

SRC_DIR="$(find "$SRC_ROOT" -mindepth 1 -maxdepth 1 -type d | head -n 1)"
if [ -z "$SRC_DIR" ] || [ ! -f "$SRC_DIR/go.mod" ]; then
  echo "could not locate Go source directory" >&2
  exit 1
fi

echo "Building sharehere for $OS/$ARCH"
(
  cd "$SRC_DIR"
  GOOS="$OS" GOARCH="$ARCH" go build -trimpath -o "$BUILD_DIR/sharehere" ./cmd/sharehere
)
BIN="$BUILD_DIR/sharehere"
if [ ! -f "$BIN" ]; then
  echo "build failed: binary not found" >&2
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

if ! command -v sharehere >/dev/null 2>&1; then
  case ":$PATH:" in
    *":$INSTALL_DIR:"*) ;;
    *)
      echo "Note: $INSTALL_DIR is not in your PATH." >&2
      echo "Add this to your shell profile:" >&2
      echo "  export PATH=\"$INSTALL_DIR:\$PATH\"" >&2
      ;;
  esac
fi

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
