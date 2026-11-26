#!/usr/bin/env bash

# SPDX-License-Identifier: Apache-2.0
# Copyright 2026 alibaba/open-code-review Contributors

set -euo pipefail

cd "$(dirname "$0")/.."
plugin_dir=$(mktemp -d)
trap 'rm -rf -- "$plugin_dir"' EXIT

git archive HEAD -- .antigravity-plugin/plugin.json skills imgs/logo.svg LICENSE |
  tar -xf - -C "$plugin_dir"
agy plugin install "$plugin_dir"
