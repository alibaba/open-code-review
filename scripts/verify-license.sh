#!/usr/bin/env bash

# SPDX-License-Identifier: Apache-2.0
# Copyright 2026 alibaba/open-code-review Contributors

set -euo pipefail

REPO_ROOT="$(git rev-parse --show-toplevel)"
cd "$REPO_ROOT"

CURRENT_YEAR=$(date +%Y)
MIN_YEAR=2026
SPDX_REGEX="SPDX-License-Identifier: Apache-2.0"
COPYRIGHT_REGEX="Copyright [0-9]{4} alibaba/open-code-review Contributors"

LICENSE_EXTS=(go sh js mjs ts tsx)

IGNORED_PATHS=(
  "vendor/"
  "dist/"
  "node_modules/"
  "testdata/"
)

is_ignored() {
  local file="$1"
  for ignore in "${IGNORED_PATHS[@]}"; do
    if [[ "$file" == */"$ignore"* ]] || [[ "$file" == "$ignore"* ]]; then
      return 0
    fi
  done
  return 1
}

FAILED=()

while IFS= read -r file; do
  [ -z "$file" ] && continue
  is_ignored "$file" && continue

  ext="${file##*.}"
  match=false
  for e in "${LICENSE_EXTS[@]}"; do
    if [[ "$ext" == "$e" ]]; then
      match=true
      break
    fi
  done
  $match || continue

  header="$(head -20 "$file")"

  if ! echo "$header" | grep -qF "$SPDX_REGEX"; then
    FAILED+=("$file (missing SPDX identifier)")
    continue
  fi

  if ! echo "$header" | grep -qE "$COPYRIGHT_REGEX"; then
    FAILED+=("$file (missing copyright notice)")
    continue
  fi

  year="$(echo "$header" | grep -oE 'Copyright ([0-9]{4})' | grep -oE '[0-9]{4}' | head -1)"
  if [ -z "$year" ] || [ "$year" -lt "$MIN_YEAR" ] || [ "$year" -gt "$CURRENT_YEAR" ]; then
    FAILED+=("$file (invalid year: ${year:-none})")
    continue
  fi
done < <(git ls-files)

if [ "${#FAILED[@]}" -gt 0 ]; then
  echo "ERROR: The following files are missing or have invalid license headers:"
  printf '  %s\n' "${FAILED[@]}"
  echo ""
  echo "Run 'make license-add' to fix."
  exit 1
fi

echo "All source files have valid license headers."
