#!/bin/sh
# Install the ocr (Open Code Review) CLI from GitHub releases.
#   curl -fsSL https://raw.githubusercontent.com/alibaba/open-code-review/main/install.sh | sh
# Prefer to inspect first:
#   curl -fsSL https://raw.githubusercontent.com/alibaba/open-code-review/main/install.sh -o install.sh
#   less install.sh && sh install.sh
# Env: OCR_INSTALL_DIR (default /usr/local/bin), OCR_VERSION (default latest).
set -eu

main() {
  REPO="alibaba/open-code-review"
  BIN="ocr"
  ASSET_PREFIX="opencodereview"
  INSTALL_DIR="${OCR_INSTALL_DIR:-/usr/local/bin}"
  VERSION="${OCR_VERSION:-}"

  command -v curl >/dev/null 2>&1 || err "curl is required"

  os="$(uname -s | tr '[:upper:]' '[:lower:]')"
  case "$os" in
    darwin|linux) ;;
    *) err "unsupported OS: $os (on Windows use: irm https://raw.githubusercontent.com/alibaba/open-code-review/main/install.ps1 | iex)" ;;
  esac

  arch="$(uname -m)"
  case "$arch" in
    x86_64|amd64) arch="amd64" ;;
    arm64|aarch64) arch="arm64" ;;
    *) err "unsupported architecture: $arch" ;;
  esac

  if [ -z "$VERSION" ]; then
    release_json="$(curl -fsSL "https://api.github.com/repos/$REPO/releases/latest")" ||
      err "failed to fetch latest release info from github api"
    VERSION="$(printf '%s' "$release_json" |
      sed -n 's/.*"tag_name"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' | head -n1)"
    [ -n "$VERSION" ] || err "could not resolve latest release tag"
  fi

  asset="${ASSET_PREFIX}-${os}-${arch}"
  base="https://github.com/$REPO/releases/download/$VERSION"
  tmp="$(mktemp -d)"
  trap 'rm -rf "$tmp"' INT TERM EXIT

  printf 'downloading %s %s (%s/%s)...\n' "$BIN" "$VERSION" "$os" "$arch"
  curl -fsSL -o "$tmp/$asset" "$base/$asset" || err "download failed: $base/$asset"
  curl -fsSL -o "$tmp/sha256sum.txt" "$base/sha256sum.txt" || err "sha256sum.txt download failed"

  want="$(awk -v a="$asset" '$2 == a {print tolower($1)}' "$tmp/sha256sum.txt")"
  [ -n "$want" ] || err "no checksum entry for $asset in sha256sum.txt"
  got="$(sha256 "$tmp/$asset" | awk '{print tolower($1)}')"
  [ "$got" = "$want" ] || err "checksum mismatch for $asset (got $got, want $want)"

  install_binary "$tmp/$asset" "$INSTALL_DIR" "$BIN"

  printf 'installed %s %s -> %s\n' "$BIN" "$VERSION" "$INSTALL_DIR/$BIN"
  post_install_path_notice "$BIN" "$INSTALL_DIR"
}

# Install the staged binary (mode 0755), escalating with sudo only when needed.
# Using install(1) under sudo gives the binary root ownership in system dirs.
install_binary() {
  src="$1"
  dir="$2"
  bin="$3"
  if mkdir -p "$dir" 2>/dev/null && [ -w "$dir" ]; then
    install -m 0755 "$src" "$dir/$bin"
  elif command -v sudo >/dev/null 2>&1; then
    printf 'note: %s is not writable; escalating with sudo\n' "$dir"
    sudo mkdir -p "$dir"
    sudo install -m 0755 "$src" "$dir/$bin"
  else
    err "$dir is not writable and sudo is unavailable; set OCR_INSTALL_DIR to a writable path"
  fi
}

post_install_path_notice() {
  bin="$1"
  install_dir="$2"
  case ":$PATH:" in
    *":$install_dir:"*) ;;
    *) printf 'note: %s is not on your PATH; add it or run %s/%s directly\n' "$install_dir" "$install_dir" "$bin"; return ;;
  esac
  command -v "$bin" >/dev/null 2>&1 || printf 'note: open a new shell so %s resolves on PATH\n' "$bin"
}

# Print the SHA-256 of a file, preferring shasum (macOS) over sha256sum (Linux).
sha256() {
  if command -v shasum >/dev/null 2>&1; then
    shasum -a 256 "$1"
  elif command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$1"
  else
    err "shasum or sha256sum is required for checksum verification"
  fi
}

err() { printf 'error: %s\n' "$1" >&2; exit 1; }

main "$@"
