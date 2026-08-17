#!/bin/sh

# SPDX-License-Identifier: Apache-2.0
# Copyright 2026 alibaba/open-code-review Contributors

# Install the ocr (Open Code Review) CLI from GitHub releases.
#   curl -fsSL https://open-codereview.ai/install.sh | sh
# Prefer to inspect first:
#   curl -fsSL https://open-codereview.ai/install.sh -o install.sh
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
    *) err "unsupported OS: $os (on Windows use: irm https://open-codereview.ai/install.ps1 | iex)" ;;
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
  install_dir="${2%/}"
  case ":$PATH:" in
    *":$install_dir:"*) ;;
    *) printf 'note: %s is not on your PATH; add it or run %s/%s directly\n' "$install_dir" "$install_dir" "$bin"; return ;;
  esac
  resolved="$(command -v "$bin" 2>/dev/null || true)"
  if [ -z "$resolved" ]; then
    printf 'note: open a new shell so %s resolves on PATH\n' "$bin"
    return
  fi
  if [ "$resolved" != "$install_dir/$bin" ]; then
    printf 'warning: %s resolves to %s, which takes precedence over %s/%s.\n' "$bin" "$resolved" "$install_dir" "$bin" >&2
    printf 'warning: remove/rename %s or move %s earlier in your PATH; verify with: %s version\n' "$resolved" "$install_dir" "$bin" >&2
    return
  fi
  # The new binary wins on PATH, but shells that were open earlier may have
  # cached a different `ocr` location (bash/zsh hash table) and keep running
  # that stale program in the current terminal; only new shells re-resolve.
  other="$(find_other_binary_on_path "$install_dir" "$bin")"
  if [ -n "$other" ]; then
    printf 'note: another %s exists at %s. Terminals opened before this install may have\n' "$bin" "$other"
    printf 'note: cached that location; if %s misbehaves in your current terminal, run\n' "$bin"
    printf 'note:   bash/zsh: hash -r    (zsh also: rehash)    or open a new terminal\n'
  fi
}

# Print the first executable named <bin> found on PATH outside <install_dir>.
find_other_binary_on_path() {
  install_dir="$1"
  bin="$2"
  saved_ifs="$IFS"
  IFS=':'
  set -f
  set -- $PATH
  set +f
  IFS="$saved_ifs"
  for dir in "$@"; do
    [ -n "$dir" ] || continue
    dir="${dir%/}" # PATH entries may carry a trailing slash
    [ "$dir" = "$install_dir" ] && continue
    if [ -x "$dir/$bin" ] && [ ! -d "$dir/$bin" ]; then
      printf '%s' "$dir/$bin"
      return 0
    fi
  done
  return 0
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
