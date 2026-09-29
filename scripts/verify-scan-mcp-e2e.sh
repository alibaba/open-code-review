#!/usr/bin/env bash

# SPDX-License-Identifier: Apache-2.0
# Copyright 2026 alibaba/open-code-review Contributors

set -euo pipefail

repo_root="$(cd "$(dirname "$0")/.." && pwd)"
cd "$repo_root"
demo_dir="$(mktemp -d)"
llm_pid=""
cleanup() {
  if [[ -n "$llm_pid" ]]; then
    kill "$llm_pid" 2>/dev/null || true
    wait "$llm_pid" 2>/dev/null || true
  fi
  rm -rf "$demo_dir"
}
trap cleanup EXIT

mkdir -p "$demo_dir/home/.opencodereview" "$demo_dir/repo"
go build -o "$demo_dir/ocr" "$repo_root/cmd/opencodereview"
go build -o "$demo_dir/scan-mcp-e2e" "$repo_root/cmd/opencodereview/testdata/scan_mcp_e2e"

printf 'package sample\n' > "$demo_dir/repo/sample.go"
git -C "$demo_dir/repo" init -q
git -C "$demo_dir/repo" add sample.go
git -C "$demo_dir/repo" -c user.name='Scan MCP Test' -c user.email='scan-mcp@example.invalid' commit -qm 'add sample'

OCR_DEMO_URL_FILE="$demo_dir/llm-url" "$demo_dir/scan-mcp-e2e" llm > "$demo_dir/llm.stdout" 2> "$demo_dir/llm.stderr" &
llm_pid=$!
for _ in {1..50}; do
  [[ -s "$demo_dir/llm-url" ]] && break
  sleep 0.1
done
[[ -s "$demo_dir/llm-url" ]] || { cat "$demo_dir/llm.stderr" >&2; exit 1; }

cat > "$demo_dir/home/.opencodereview/config.json" <<EOF
{"mcp_servers":{"demo":{"command":"$demo_dir/scan-mcp-e2e","args":["mcp"],"tools":["lookup_context"],"env":["OCR_DEMO_TOOL_MARKER=$demo_dir/tool-called"]}}}
EOF

env HOME="$demo_dir/home" \
  OCR_LLM_URL="$(cat "$demo_dir/llm-url")" \
  OCR_LLM_TOKEN=test-token \
  OCR_LLM_MODEL=claude-test \
  OCR_LLM_PROTOCOL=anthropic \
  OCR_LLM_AUTH_HEADER=x-api-key \
  "$demo_dir/ocr" scan --repo "$demo_dir/repo" --path sample.go \
  --no-plan --no-dedup --no-summary --format json --output "$demo_dir/result.json" \
  > "$demo_dir/scan.stdout" 2> "$demo_dir/scan.stderr"

grep -q '^MCP_TOOL_CALLED$' "$demo_dir/tool-called"
grep -q '^MCP_RESULT_REACHED_SCAN_AGENT$' "$demo_dir/llm.stderr"
grep -q '"files_reviewed": 1' "$demo_dir/result.json"
grep -q '"lookup_context": 1' "$demo_dir/result.json"

cat "$demo_dir/tool-called"
grep '^MCP_RESULT_REACHED_SCAN_AGENT$' "$demo_dir/llm.stderr"
grep -E '"files_reviewed": 1|"lookup_context": 1|"comments": 0' "$demo_dir/result.json"
