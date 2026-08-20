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
# The whole line must be a pinned reference: only list/indent syntax before
# `uses:`, a 40-hex SHA, and a strict `# vX.Y.Z` comment with nothing after
# it. Without the anchors a floating tag would be accepted whenever a
# pinned-looking fragment appeared anywhere in the line (e.g. inside a
# trailing comment), and a `# v7` comment would satisfy the format the
# check claims to enforce. Anything unusual (quoted values, extra trailing
# content) fails closed rather than being guessed at.
# The directive filter also matches flow-mapping openers ("- {uses: …}"),
# which the pinned pattern below never accepts — so flow-style entries fail
# closed as unusual syntax instead of being silently skipped.
directive='^[[:space:]]*(-[[:space:]]+)?(\{[[:space:]]*)?uses:'
pinned='^[[:space:]]*(-[[:space:]]+)?uses:[[:space:]]+[A-Za-z0-9_.-]+/[A-Za-z0-9_./-]+@[0-9a-f]{40}[[:space:]]+#[[:space:]]*v[0-9]+\.[0-9]+\.[0-9]+[[:space:]]*$'
local_ref='^[[:space:]]*(-[[:space:]]+)?uses:[[:space:]]*\./'

bad=""
for file in "${files[@]}"; do
  if [ ! -f "$file" ]; then
    echo "ERROR: $file not found; the pin check cannot run." >&2
    exit 1
  fi
  hits="$(grep -nE "$directive" "$file" || true)"
  [ -n "$hits" ] || continue
  while IFS= read -r line; do
    # Strip the NN: line-number prefix grep -n added; the patterns above
    # anchor on the start of the actual line.
    value="${line#*:}"
    if printf '%s' "$value" | grep -qE "$local_ref"; then
      continue
    fi
    if ! printf '%s' "$value" | grep -qE "$pinned"; then
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
