#!/usr/bin/env bash

# SPDX-License-Identifier: Apache-2.0
# Copyright 2026 alibaba/open-code-review Contributors

set -euo pipefail

REPO_ROOT="$(git rev-parse --show-toplevel)"
cd "$REPO_ROOT"

MIN_YEAR=2026
SPDX_REGEX="SPDX-License-Identifier: Apache-2.0"
COPYRIGHT_REGEX="Copyright [0-9]{4} alibaba/open-code-review Contributors"

SLASH_HEADER='// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors'

HASH_HEADER='# SPDX-License-Identifier: Apache-2.0
# Copyright 2026 alibaba/open-code-review Contributors'

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

has_header() {
  local header
  header="$(head -20 "$1")"
  echo "$header" | grep -qF "$SPDX_REGEX" && echo "$header" | grep -qE "$COPYRIGHT_REGEX"
}

add_header() {
  local file="$1"
  local header="$2"
  local tmp
  tmp=$(mktemp)
  if stat -f '%Lp' "$file" >/dev/null 2>&1; then
    chmod "$(stat -f '%Lp' "$file")" "$tmp"
  else
    chmod "$(stat -c '%a' "$file")" "$tmp"
  fi

  local first_line
  first_line="$(head -1 "$file")"

  if [[ "$first_line" =~ ^//go:build ]]; then
    echo "$first_line" > "$tmp"
    echo "" >> "$tmp"
    echo "$header" >> "$tmp"
    echo "" >> "$tmp"
    tail -n +2 "$file" >> "$tmp"
  elif [[ "$first_line" =~ ^#! ]]; then
    echo "$first_line" > "$tmp"
    echo "" >> "$tmp"
    echo "$header" >> "$tmp"
    echo "" >> "$tmp"
    tail -n +2 "$file" >> "$tmp"
  else
    echo "$header" > "$tmp"
    echo "" >> "$tmp"
    cat "$file" >> "$tmp"
  fi

  mv "$tmp" "$file"
}

ADDED=0

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

  has_header "$file" && continue

  case "$ext" in
    sh) add_header "$file" "$HASH_HEADER" ;;
    *)  add_header "$file" "$SLASH_HEADER" ;;
  esac

  ADDED=$((ADDED + 1))
  echo "  Added: $file"
done < <(git ls-files)

echo "Done. Added license headers to $ADDED file(s)."
