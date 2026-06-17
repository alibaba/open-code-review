#!/usr/bin/env bash
#
# publish.sh — Environment-driven publish pipeline.
#
# Builds binaries, optionally uploads to a git-based release repo,
# patches package.json with override values, and publishes to npm.
#
# Usage:
#   source scripts/publish/.env.internal && ./scripts/publish/publish.sh
#
# Environment variables (all optional — defaults come from package.json):
#   OCR_PKG_NAME             Override package name
#   OCR_PUBLISH_REGISTRY     Override npm registry URL
#   OCR_URL_PATTERN          Override ocrConfig.urlPattern
#   OCR_CHECKSUM_PATTERN     Override ocrConfig.checksumPattern
#   OCR_RELEASE_GIT_REPO     Git repo URL for binary uploads (skip if unset)
#   OCR_VERSION_OVERRIDE     Use this version instead of latest git tag
#   OCR_FORCE_YES            Skip confirmation prompts (CI mode)
#   OCR_SKIP_BUILD           Skip build step (use existing dist/)
#   OCR_SKIP_UPLOAD          Skip git repo upload step
#   OCR_SKIP_PUBLISH         Skip npm publish step
#

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
source "$SCRIPT_DIR/_common.sh"

PROJECT_ROOT="$(resolve_project_root)"
cd "$PROJECT_ROOT"

# ── Configuration ─────────────────────────────────────────────────────────────
SKIP_BUILD="${OCR_SKIP_BUILD:-0}"
SKIP_UPLOAD="${OCR_SKIP_UPLOAD:-0}"
SKIP_PUBLISH="${OCR_SKIP_PUBLISH:-0}"

# ── Banner ────────────────────────────────────────────────────────────────────
echo ""
echo -e "${BOLD}============================================${RESET}"
echo -e "${BOLD}  OpenCodeReview — Publish Pipeline        ${RESET}"
echo -e "${BOLD}============================================${RESET}"
echo ""

# ── Resolve version from git tag ─────────────────────────────────────────────
VERSION_TAG="${OCR_VERSION_OVERRIDE:-}"
if [ -z "$VERSION_TAG" ]; then
    VERSION_TAG=$(git describe --tags --abbrev=0 2>/dev/null || die "No git tag found. Create one: git tag vX.Y.Z && git push origin --tags")
fi
VERSION_TAG="$(normalize_version "$VERSION_TAG")"
NPM_VERSION="$(npm_version_from_tag "$VERSION_TAG")"
export VERSION_TAG NPM_VERSION

info "Release plan:"
info "  Git tag:     ${VERSION_TAG}"
info "  npm version: ${NPM_VERSION}"
[ -n "${OCR_PKG_NAME:-}" ]          && info "  Package:     ${OCR_PKG_NAME}"
[ -n "${OCR_PUBLISH_REGISTRY:-}" ]  && info "  Registry:    ${OCR_PUBLISH_REGISTRY}"
[ -n "${OCR_RELEASE_GIT_REPO:-}" ]  && info "  Upload repo: ${OCR_RELEASE_GIT_REPO}"
echo ""

# ── Pre-flight checks ────────────────────────────────────────────────────────
preflight() {
    local missing=0
    info "Checking prerequisites..."
    for cmd in go jq npm git shasum; do
        if command -v "$cmd" >/dev/null 2>&1; then
            info "  ${cmd}: found"
        else
            error "  ${cmd}: NOT FOUND"
            missing=1
        fi
    done
    [ "$missing" -eq 0 ] || die "Missing required tools."
    success "All tools available"
    echo ""

    local status
    status=$(git status --porcelain 2>/dev/null || true)
    if [ -n "$status" ]; then
        warn "Working tree has uncommitted changes:"
        echo "$status"
        die "Please commit or stash changes before publishing."
    fi
    success "Git working tree is clean"
}

run_step "Pre-flight checks" "preflight"

# ── Confirm ───────────────────────────────────────────────────────────────────
if ! confirm "Ready to publish ${VERSION_TAG}. Continue?"; then
    info "Aborted by user."
    exit 0
fi

# ── Build ─────────────────────────────────────────────────────────────────────
if [ "$SKIP_BUILD" = "1" ]; then
    warn "Skipping build (OCR_SKIP_BUILD=1)"
else
    run_step "Building binaries (make dist)" \
        "make -C \"$PROJECT_ROOT\" dist"
fi

# ── Publish platform-specific packages ────────────────────────────────────────
if [ "$SKIP_PUBLISH" = "1" ]; then
    warn "Skipping platform packages (OCR_SKIP_PUBLISH=1)"
else
    run_step "Publishing platform packages" \
        "bash \"$SCRIPT_DIR/publish-platform.sh\""
fi

# ── Upload to git-based release repo (optional) ──────────────────────────────
upload_to_git_repo() {
    local repo_url="$OCR_RELEASE_GIT_REPO"
    local dist_dir="$PROJECT_ROOT/dist"
    local tmp_repo
    tmp_repo=$(mktemp -d -t "ocr-release-repo.XXXXXX")

    info "Cloning release repo (sparse)..."
    git clone --depth 1 --filter=blob:none --sparse "$repo_url" "$tmp_repo"
    (cd "$tmp_repo" && git sparse-checkout set --no-cone VERSION)
    success "Clone ready"

    local bin_dir="${tmp_repo}/bin/${VERSION_TAG}"
    mkdir -p "$bin_dir"

    local count=0
    for f in "$dist_dir"/opencodereview-*; do
        [ -f "$f" ] || continue
        [[ "$(basename "$f")" == *.txt ]] && continue
        local base
        base=$(basename "$f")
        info "  Copying ${base}"
        cp "$f" "$bin_dir/${base}"
        count=$((count + 1))
    done
    if [ "$count" -eq 0 ]; then
        rm -rf "$tmp_repo"
        die "No binaries found matching opencodereview-*"
    fi
    success "Copied ${count} binaries"

    (cd "$bin_dir" && shasum -a 256 opencodereview-* | sort > sha256sum.txt)
    success "Checksums generated"

    local version_file="${tmp_repo}/VERSION"
    if [ -f "$version_file" ]; then
        local last
        last=$(tail -1 "$version_file" 2>/dev/null || true)
        [ "$last" = "$VERSION_TAG" ] || printf "%s\n" "$VERSION_TAG" >> "$version_file"
    else
        printf "%s\n" "$VERSION_TAG" > "$version_file"
    fi

    (
        cd "$tmp_repo"
        git add --sparse -A
        if ! git diff --cached --quiet; then
            git config user.name "open-code-review-bot"
            git config user.email "bot@open-code-review.local"
            git commit -m "release: ${VERSION_TAG}"
            git push
            success "Pushed to release repo"
        else
            info "No changes to push — binaries already exist."
        fi
    )

    rm -rf "$tmp_repo"
}

if [ "$SKIP_UPLOAD" = "1" ]; then
    warn "Skipping upload (OCR_SKIP_UPLOAD=1)"
elif [ -n "${OCR_RELEASE_GIT_REPO:-}" ]; then
    run_step "Uploading to release repo" "upload_to_git_repo"
else
    info "No OCR_RELEASE_GIT_REPO set, skipping binary upload."
    echo ""
fi

# ── Patch package.json for publish ────────────────────────────────────────────
PACKAGE_BACKUP=""

patch_package_json() {
    local pkg="$PROJECT_ROOT/package.json"

    PACKAGE_BACKUP=$(mktemp)
    cp "$pkg" "$PACKAGE_BACKUP"

    local tmp
    tmp=$(mktemp)
    cp "$pkg" "$tmp"

    # Always inject version from git tag (temporary, not committed)
    jq --arg v "$NPM_VERSION" '.version = $v' "$tmp" > "${tmp}.new" && mv "${tmp}.new" "$tmp"
    info "  version → ${NPM_VERSION}"

    # Pin optionalDependencies (platform packages) to the same version
    if jq -e '.optionalDependencies' "$tmp" >/dev/null 2>&1; then
        jq --arg v "$NPM_VERSION" '.optionalDependencies |= with_entries(.value = $v)' "$tmp" > "${tmp}.new" && mv "${tmp}.new" "$tmp"
        info "  optionalDependencies → all pinned to ${NPM_VERSION}"
    fi

    if [ -n "${OCR_PKG_NAME:-}" ]; then
        local new_scope="${OCR_PKG_NAME%%/*}"
        jq --arg n "$OCR_PKG_NAME" --arg s "$new_scope" '
          .name = $n |
          if .optionalDependencies then
            .optionalDependencies |= with_entries(.key |= sub("^@[^/]+"; $s))
          else . end
        ' "$tmp" > "${tmp}.new" && mv "${tmp}.new" "$tmp"
        info "  name → ${OCR_PKG_NAME}"
        info "  optionalDependencies scope → ${new_scope}"
    fi

    if [ -n "${OCR_PUBLISH_REGISTRY:-}" ]; then
        jq --arg r "$OCR_PUBLISH_REGISTRY" '.publishConfig.registry = $r | .publishConfig.access = "public"' "$tmp" > "${tmp}.new" && mv "${tmp}.new" "$tmp"
        info "  publishConfig.registry → ${OCR_PUBLISH_REGISTRY}"
    fi

    if [ -n "${OCR_URL_PATTERN:-}" ]; then
        jq --arg u "$OCR_URL_PATTERN" '.ocrConfig.urlPattern = $u' "$tmp" > "${tmp}.new" && mv "${tmp}.new" "$tmp"
        info "  ocrConfig.urlPattern → ${OCR_URL_PATTERN}"
    fi

    if [ -n "${OCR_CHECKSUM_PATTERN:-}" ]; then
        jq --arg c "$OCR_CHECKSUM_PATTERN" '.ocrConfig.checksumPattern = $c' "$tmp" > "${tmp}.new" && mv "${tmp}.new" "$tmp"
        info "  ocrConfig.checksumPattern → ${OCR_CHECKSUM_PATTERN}"
    fi

    mv "$tmp" "$pkg"
    success "package.json patched for publish"
}

run_step "Patching package.json" "patch_package_json"

# ── Publish ───────────────────────────────────────────────────────────────────
do_publish() {
    local registry_args=()
    if [ -n "${OCR_PUBLISH_REGISTRY:-}" ]; then
        registry_args=(--registry "$OCR_PUBLISH_REGISTRY")
    fi

    local pkg_name
    pkg_name=$(jq -r '.name' "$PROJECT_ROOT/package.json")
    local already
    already=$(npm view "${pkg_name}@${NPM_VERSION}" version ${registry_args[@]+"${registry_args[@]}"} 2>/dev/null || true)
    if [ "$already" = "$NPM_VERSION" ]; then
        warn "Version ${NPM_VERSION} already published!"
        if confirm "Skip publish and continue?"; then
            return 0
        else
            die "Aborted."
        fi
    fi

    info "Publishing ${pkg_name}@${NPM_VERSION} ..."
    npm publish ${registry_args[@]+"${registry_args[@]}"} || die "npm publish failed"
    success "Published ${pkg_name}@${NPM_VERSION}"
}

if [ "$SKIP_PUBLISH" = "1" ]; then
    warn "Skipping publish (OCR_SKIP_PUBLISH=1)"
else
    run_step "Publishing to registry" "do_publish"
fi

# ── Restore package.json ─────────────────────────────────────────────────────
if [ -n "$PACKAGE_BACKUP" ]; then
    cp "$PACKAGE_BACKUP" "$PROJECT_ROOT/package.json"
    rm -f "$PACKAGE_BACKUP"
    success "package.json restored"
    echo ""
fi

# ── Done ──────────────────────────────────────────────────────────────────────
echo ""
echo -e "${GREEN}${BOLD}============================================${RESET}"
echo -e "${GREEN}${BOLD}  PUBLISH COMPLETE                         ${RESET}"
echo -e "${GREEN}${BOLD}============================================${RESET}"
echo ""
echo -e "  Version: ${BOLD}${VERSION_TAG}${RESET}"
[ -n "${OCR_PKG_NAME:-}" ] && echo -e "  Package: ${OCR_PKG_NAME}" || echo -e "  Package: $(jq -r '.name' "$PROJECT_ROOT/package.json")"
echo ""
