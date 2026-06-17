#!/usr/bin/env bash
#
# publish-platform.sh — Publish platform-specific npm packages.
#
# Expects NPM_VERSION to be exported by the caller (publish.sh).
# Each platform package ships the Go binary for a single OS/arch combination.
#

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/_common.sh"

PROJECT_ROOT="$(resolve_project_root)"

[ -z "${NPM_VERSION:-}" ] && die "NPM_VERSION is not set. This script should be called from publish.sh."

# Go dist/ binary name → npm platform directory
# Format: "npm_dir:dist_binary:dest_filename"
PLATFORMS=(
  "darwin-arm64:opencodereview-darwin-arm64:opencodereview"
  "darwin-x64:opencodereview-darwin-amd64:opencodereview"
  "linux-arm64:opencodereview-linux-arm64:opencodereview"
  "linux-x64:opencodereview-linux-amd64:opencodereview"
  "win32-arm64:opencodereview-windows-arm64.exe:opencodereview.exe"
  "win32-x64:opencodereview-windows-amd64.exe:opencodereview.exe"
)

REGISTRY_ARGS=()
if [ -n "${OCR_PUBLISH_REGISTRY:-}" ]; then
    REGISTRY_ARGS=(--registry "$OCR_PUBLISH_REGISTRY")
fi

# Derive scope override from OCR_PKG_NAME (e.g. @ali/open-code-review → @ali)
SCOPE_OVERRIDE=""
if [ -n "${OCR_PKG_NAME:-}" ]; then
    SCOPE_OVERRIDE="${OCR_PKG_NAME%%/*}"
fi

# Pre-check all binaries exist before publishing anything
for entry in "${PLATFORMS[@]}"; do
    IFS=':' read -r _ dist_binary _ <<< "$entry"
    [ -f "$PROJECT_ROOT/dist/$dist_binary" ] || die "Binary not found: $PROJECT_ROOT/dist/$dist_binary"
done

FAILED_PLATFORMS=()
CURRENT_BACKUP=""
CURRENT_PKG_JSON=""
CURRENT_BIN_DIR=""

cleanup_current() {
    if [ -n "$CURRENT_BACKUP" ] && [ -n "$CURRENT_PKG_JSON" ]; then
        cp "$CURRENT_BACKUP" "$CURRENT_PKG_JSON" 2>/dev/null || true
        rm -f "$CURRENT_BACKUP"
    fi
    if [ -n "$CURRENT_BIN_DIR" ]; then
        rm -rf "$CURRENT_BIN_DIR"
    fi
    CURRENT_BACKUP=""
    CURRENT_PKG_JSON=""
    CURRENT_BIN_DIR=""
}

trap cleanup_current EXIT

for entry in "${PLATFORMS[@]}"; do
    IFS=':' read -r platform_dir dist_binary dest_name <<< "$entry"

    pkg_dir="$PROJECT_ROOT/npm/$platform_dir"
    bin_dir="$pkg_dir/bin"
    dist_path="$PROJECT_ROOT/dist/$dist_binary"
    pkg_json="$pkg_dir/package.json"

    info "Preparing $platform_dir..."
    mkdir -p "$bin_dir"
    cp "$dist_path" "$bin_dir/$dest_name"
    chmod 755 "$bin_dir/$dest_name" 2>/dev/null || true

    CURRENT_BACKUP=$(mktemp)
    CURRENT_PKG_JSON="$pkg_json"
    CURRENT_BIN_DIR="$bin_dir"
    cp "$pkg_json" "$CURRENT_BACKUP"

    if [ -n "$SCOPE_OVERRIDE" ]; then
        jq --arg v "$NPM_VERSION" --arg s "$SCOPE_OVERRIDE" \
            '.version = $v | .name = (.name | sub("^@[^/]+"; $s))' \
            "$pkg_json" > "${pkg_json}.tmp" && mv "${pkg_json}.tmp" "$pkg_json" || { rm -f "${pkg_json}.tmp"; false; }
    else
        jq --arg v "$NPM_VERSION" '.version = $v' "$pkg_json" > "${pkg_json}.tmp" && mv "${pkg_json}.tmp" "$pkg_json" || { rm -f "${pkg_json}.tmp"; false; }
    fi

    pkg_name=$(jq -r '.name' "$pkg_json")
    already=$(npm view "${pkg_name}@${NPM_VERSION}" version ${REGISTRY_ARGS[@]+"${REGISTRY_ARGS[@]}"} 2>/dev/null || true)
    if [ "$already" = "$NPM_VERSION" ]; then
        warn "  ${pkg_name}@${NPM_VERSION} already published, skipping."
    else
        info "  Publishing ${pkg_name}@${NPM_VERSION}..."
        if ! npm publish "$pkg_dir" --access public ${REGISTRY_ARGS[@]+"${REGISTRY_ARGS[@]}"}; then
            warn "  Failed to publish ${pkg_name}@${NPM_VERSION}"
            FAILED_PLATFORMS+=("$platform_dir")
        else
            success "  Published ${pkg_name}@${NPM_VERSION}"
        fi
    fi

    cleanup_current
done

trap - EXIT

if [ ${#FAILED_PLATFORMS[@]} -gt 0 ]; then
    die "Failed to publish platforms: ${FAILED_PLATFORMS[*]}"
fi

success "All platform packages processed."
