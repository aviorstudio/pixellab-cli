#!/usr/bin/env sh
set -eu

REPO="${PXLB_REPO:-aviorstudio/pixellab-cli}"
VERSION="${VERSION:-${PXLB_VERSION:-latest}}"
INSTALL_DIR="${INSTALL_DIR:-}"

need_cmd() {
  if ! command -v "$1" >/dev/null 2>&1; then
    printf 'missing required command: %s\n' "$1" >&2
    exit 1
  fi
}

detect_os() {
  os="$(uname -s | tr '[:upper:]' '[:lower:]')"
  case "$os" in
    darwin) printf 'Darwin' ;;
    linux) printf 'Linux' ;;
    *)
      printf 'unsupported OS: %s\n' "$os" >&2
      exit 1
      ;;
  esac
}

detect_arch() {
  arch="$(uname -m)"
  case "$arch" in
    x86_64|amd64) printf 'x86_64' ;;
    arm64|aarch64) printf 'arm64' ;;
    *)
      printf 'unsupported architecture: %s\n' "$arch" >&2
      exit 1
      ;;
  esac
}

latest_tag() {
  curl -fsSL "https://api.github.com/repos/$REPO/releases/latest" \
    | sed -n 's/.*"tag_name": *"\([^"]*\)".*/\1/p'
}

pick_install_dir() {
  if [ -n "$INSTALL_DIR" ]; then
    printf '%s' "$INSTALL_DIR"
    return
  fi

  if [ -d /usr/local/bin ] && [ -w /usr/local/bin ]; then
    printf '/usr/local/bin'
    return
  fi

  printf '%s/.local/bin' "$HOME"
}

verify_checksum() {
  artifact="$1"
  checksums="$2"
  artifact_name="$(basename "$artifact")"
  checksum_line="$(grep "  $artifact_name$" "$checksums" || true)"
  expected="${checksum_line%% *}"

  if command -v sha256sum >/dev/null 2>&1; then
    actual_line="$(sha256sum "$artifact")"
    actual="${actual_line%% *}"
  elif command -v shasum >/dev/null 2>&1; then
    actual_line="$(shasum -a 256 "$artifact")"
    actual="${actual_line%% *}"
  else
    printf 'missing checksum command: install sha256sum or shasum\n' >&2
    exit 1
  fi

  if [ -z "$expected" ]; then
    printf 'checksum not found for %s\n' "$artifact_name" >&2
    exit 1
  fi
  if [ "$expected" != "$actual" ]; then
    printf 'checksum mismatch for %s\n' "$artifact_name" >&2
    exit 1
  fi
}

need_cmd curl
need_cmd basename
need_cmd chmod
need_cmd grep
need_cmd mktemp
need_cmd mkdir
need_cmd mv
need_cmd sed
need_cmd tar
need_cmd tr
need_cmd uname

OS="$(detect_os)"
ARCH="$(detect_arch)"

if [ "$VERSION" = "latest" ]; then
  TAG="$(latest_tag)"
  if [ -z "$TAG" ]; then
    printf 'failed to resolve latest release for %s\n' "$REPO" >&2
    exit 1
  fi
else
  case "$VERSION" in
    v*) TAG="$VERSION" ;;
    *) TAG="v$VERSION" ;;
  esac
fi

ARTIFACT="pxlb_${OS}_${ARCH}.tar.gz"
BASE_URL="https://github.com/$REPO/releases/download/$TAG"
TMP_DIR="$(mktemp -d)"
trap 'rm -rf "$TMP_DIR"' EXIT INT TERM

curl -fsSL "$BASE_URL/$ARTIFACT" -o "$TMP_DIR/$ARTIFACT"
curl -fsSL "$BASE_URL/checksums.txt" -o "$TMP_DIR/checksums.txt"
verify_checksum "$TMP_DIR/$ARTIFACT" "$TMP_DIR/checksums.txt"

tar -xzf "$TMP_DIR/$ARTIFACT" -C "$TMP_DIR" pxlb

DEST_DIR="$(pick_install_dir)"
mkdir -p "$DEST_DIR"
if [ ! -w "$DEST_DIR" ]; then
  printf 'install directory is not writable: %s\n' "$DEST_DIR" >&2
  printf 'rerun with INSTALL_DIR=$HOME/.local/bin or use sudo with INSTALL_DIR=/usr/local/bin\n' >&2
  exit 1
fi

mv "$TMP_DIR/pxlb" "$DEST_DIR/pxlb"
chmod +x "$DEST_DIR/pxlb"

printf 'Installed pxlb %s to %s/pxlb\n' "$TAG" "$DEST_DIR"
case ":$PATH:" in
  *":$DEST_DIR:"*) ;;
  *) printf 'Add %s to PATH if pxlb is not found.\n' "$DEST_DIR" ;;
esac
