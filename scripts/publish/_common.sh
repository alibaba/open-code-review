#!/usr/bin/env bash
# _common.sh — Shared utilities for the publish pipeline.
# Sourced by other scripts, not executed directly.

# ── Colors ────────────────────────────────────────────────────────────────────
if [ -t 1 ]; then
    RED='\033[0;31m' GREEN='\033[0;32m' YELLOW='\033[0;33m'
    BLUE='\033[0;34m' BOLD='\033[1m' RESET='\033[0m'
else
    RED='' GREEN='' YELLOW='' BLUE='' BOLD='' RESET=''
fi

# ── Logging ───────────────────────────────────────────────────────────────────
info()    { echo -e "${BLUE}[INFO]${RESET}  $*"; }
success() { echo -e "${GREEN}[OK]${RESET}    $*"; }
warn()    { echo -e "${YELLOW}[WARN]${RESET}  $*" >&2; }
error()   { echo -e "${RED}[ERROR]${RESET} $*" >&2; }
die()     { error "$*"; exit 1; }

# ── Path resolution ──────────────────────────────────────────────────────────
resolve_project_root() {
    local script_dir
    script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
    (cd "$script_dir/../.." && pwd)
}

# ── Version helpers ──────────────────────────────────────────────────────────
normalize_version() {
    local v="$1"
    [[ "$v" != v* ]] && v="v${v}"
    echo "$v"
}

npm_version_from_tag() {
    echo "${1#v}"
}

# ── Confirmation prompt ──────────────────────────────────────────────────────
confirm() {
    local message="${1:-Proceed?}"
    if [ "${OCR_FORCE_YES:-0}" = "1" ]; then
        return 0
    fi
    printf "\n%b%s (y/n): ${RESET}" "$BOLD" "$message"
    read -r answer
    case "$answer" in
        [yY]|[yY][eE][sS]) return 0 ;;
        *) return 1 ;;
    esac
}

# ── Step execution with timing ───────────────────────────────────────────────
run_step() {
    local step_name="$1"
    local step_cmd="$2"
    info "--- ${step_name} ---"
    local start_time
    start_time=$(date +%s)
    eval "$step_cmd" || die "Step '${step_name}' failed."
    local elapsed
    elapsed=$(( $(date +%s) - start_time ))
    success "${step_name} completed (${elapsed}s)"
    echo ""
}
