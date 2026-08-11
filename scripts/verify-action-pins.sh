#!/usr/bin/env bash

# SPDX-License-Identifier: Apache-2.0
# Copyright 2026 alibaba/open-code-review Contributors

# Verify that every external action referenced by the published composite
# action (action.yml) is pinned to a full 40-hex commit SHA with a trailing
# "# vX.Y.Z" version comment. A floating tag inside action.yml silently
# undermines consumers who SHA-pin alibaba/open-code-review itself: the
# outer pin freezes this repository, but a moved inner tag still changes
# what actually runs (see issue #816).
set -euo pipefail

cd "$(dirname "$0")/.."

files=("action.yml")
pinned='uses:[[:space:]]*[A-Za-z0-9_.-]+/[A-Za-z0-9_./-]+@[0-9a-f]{40}[[:space:]]+#[[:space:]]*v[0-9]'
local_ref='uses:[[:space:]]*\./'

bad=""
for file in "${files[@]}"; do
  hits="$(grep -nE 'uses:' "$file" || true)"
  [ -n "$hits" ] || continue
  while IFS= read -r line; do
    if printf '%s' "$line" | grep -qE "$local_ref"; then
      continue
    fi
    if ! printf '%s' "$line" | grep -qE "$pinned"; then
      bad="${bad}${file}:${line}"$'\n'
    fi
  done <<< "$hits"
done

if [ -n "$bad" ]; then
  echo "The following action references are not pinned to a full commit SHA"
  echo "with a '# vX.Y.Z' comment:"
  printf '%s' "$bad"
  echo "Pin them like: uses: owner/repo@<40-hex-sha> # vX.Y.Z"
  exit 1
fi

echo "All external action references in ${files[*]} are SHA-pinned."
