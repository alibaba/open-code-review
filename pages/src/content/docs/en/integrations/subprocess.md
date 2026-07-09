---
title: Direct Subprocess
sidebar:
  order: 3
---

Shell out to `ocr` and parse the JSON. This is the lowest-level
integration path — every other method on this site ultimately reduces
to it. The [Agent Skill](../agent-skill/) and [Command](../claude-code/)
methods are prompt templates that tell a calling agent to do exactly
this; the [CI/CD](../ci/) recipes are GitHub Actions and GitLab CI
pipelines that do the same thing from a script — no orchestrating
agent, just subprocess invocation, JSON parsing, and posting comments
back to the PR / MR. Use this page directly when you're calling OCR
from a custom script, a LangChain tool, or any other framework that
isn't already covered.

## Bash

```bash
result=$(ocr review --format json --audience agent)
status=$(echo "$result" | jq -r '.status')
total=$(echo "$result" | jq '.comments | length')
echo "Status: $status — $total comments"
echo "$result" | jq -r '.comments[] | "\(.path):\(.start_line) — \(.content)"'
```

## Python

```python
import json, subprocess

proc = subprocess.run(
    ["ocr", "review", "--format", "json", "--audience", "agent",
     "--from", "origin/main", "--to", "HEAD",
     "--background", pr_description],
    capture_output=True, text=True, check=True,
)
data = json.loads(proc.stdout)
for c in data["comments"]:
    if c["start_line"] > 0:
        post_line_comment(c["path"], c["start_line"], c["content"])
```

## JSON shape

OCR emits a single top-level **object** (not a bare array). Here is a
complete `success` envelope with one finding:

```json
{
  "status": "success",
  "summary": {
    "files_reviewed": 1,
    "comments": 1,
    "total_tokens": 12770,
    "input_tokens": 12450,
    "output_tokens": 320,
    "elapsed": "9s"
  },
  "comments": [
    {
      "path": "internal/cache/store.go",
      "content": "Concurrent map access without a lock — wrap reads and writes with `sync.RWMutex` to avoid a race on the shared cache.",
      "start_line": 42,
      "end_line": 47,
      "existing_code": "func (s *Store) Get(k string) string {\n    return s.m[k]\n}",
      "suggestion_code": "func (s *Store) Get(k string) string {\n    s.mu.RLock()\n    defer s.mu.RUnlock()\n    return s.m[k]\n}",
      "thinking": "The struct exposes `m map[string]string` without a guarding mutex, and Get/Set are called from concurrent request handlers."
    }
  ]
}
```

### Top-level fields

| Field | Type | Always present | Notes |
|---|---|---|---|
| `status` | string | Yes | One of `success`, `completed_with_warnings`, `completed_with_errors`, `skipped`. |
| `message` | string | No | Short human-readable summary. Set on empty / skipped runs, e.g. `"No comments generated. Looks good to me."`. |
| `summary` | object | No | Run aggregates. Present on completed runs; omitted on `skipped`. Fields below. |
| `comments` | array | Yes | Possibly empty. Per-comment schema below. |
| `warnings` | array | No | Present only when one or more sub-agents failed or were skipped. Schema below. |

### Summary shape (`summary`)

| Field | Type | Notes |
|---|---|---|
| `files_reviewed` | int | Number of files that survived all filters and were sent to the model. |
| `comments` | int | Total comments emitted across all files (matches `comments.length`). |
| `total_tokens` | int | Sum of prompt + completion tokens across every LLM call in the run. |
| `input_tokens` | int | Prompt tokens (including cache-read tokens) across every LLM call. |
| `output_tokens` | int | Completion tokens (including cache-write tokens) across every LLM call. |
| `cache_read_tokens` | int | Total cache-read tokens across every LLM call. Omitted (`omitempty`) when zero. |
| `cache_write_tokens` | int | Total cache-write tokens across every LLM call. Omitted (`omitempty`) when zero. |
| `elapsed` | string | Wall-clock duration rounded to whole seconds, formatted by Go's `time.Duration.String()` (e.g. `"1m12s"`). |

### Per-comment fields (`comments[]`)

| Field | Type | Always present | Notes |
|---|---|---|---|
| `path` | string | Yes | Repo-relative file path. |
| `content` | string | Yes | The review comment, in Markdown. |
| `start_line` | int | Yes | First line of the affected range. A value `< 1` means the comment has no line anchor (file-level) — fold these into the summary instead of trying to post them inline. |
| `end_line` | int | Yes | Last line of the affected range. Equal to `start_line` for single-line comments. |
| `existing_code` | string | No | Original code snippet to be replaced. Omitted for advisory comments with no diff. |
| `suggestion_code` | string | No | Proposed replacement for `existing_code`. Always paired with `existing_code` when present. |
| `thinking` | string | No | Model's reasoning trail. Useful for triage / debugging; safe to drop before displaying to humans. |

### Warnings shape (`warnings[]`)

A run where some files were skipped or failed looks like this:

```json
{
  "status": "completed_with_errors",
  "message": "Some files could not be reviewed due to errors.",
  "comments": [],
  "warnings": [
    {
      "file": "src/very_long_file.go",
      "message": "diff size exceeds 80% of MAX_TOKENS; skipped",
      "type": "token_threshold_exceeded"
    },
    {
      "file": "src/broken.py",
      "message": "sub-agent failed: context deadline exceeded",
      "type": "subtask_error"
    }
  ]
}
```

| Field | Type | Notes |
|---|---|---|
| `file` | string | Repo-relative path of the file that triggered the warning. |
| `message` | string | Short human-readable description. |
| `type` | string | Stable kind for filtering. Currently emitted: `subtask_error` (a sub-agent run failed) and `token_threshold_exceeded` (diff too large for the model). |

When `warnings` contains at least one `subtask_error`, `status` is
`completed_with_errors`; otherwise it's `completed_with_warnings`.

### No severity / priority field

OCR does **not** emit a `severity` or `priority` field. The
High/Medium/Low triage you see in the [Agent Skill](../agent-skill/)
and [Command](../claude-code/) docs is added by the calling agent
after it receives the raw comments — don't try to `jq '.comments[].severity'`,
it won't exist.

## Empty-result handling

A workspace with **no eligible files** is reported via `status`, so
callers can distinguish "nothing changed" from "no findings":

```json
{
  "status": "skipped",
  "message": "No supported files changed.",
  "comments": []
}
```

Always check `status == "skipped"` before declaring "all clean".

## See Also

- [CI/CD](../ci/) — ready-made GitHub Actions and pre-commit recipes
  built on top of subprocess invocation.
- [Agent Skill](../agent-skill/) — when the caller is an Anthropic
  SDK agent rather than a plain script.
