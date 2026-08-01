---
title: Tools
sidebar:
  order: 9
---

OCR ships with **six built-in tools** the LLM can call during a review.
This page documents each tool's purpose, input schema, and example
input/output. The full machine-readable definitions live in
[`internal/config/toolsconfig/tools.json`](https://github.com/alibaba/open-code-review/blob/main/internal/config/toolsconfig/tools.json).

## Tool availability per phase

Each tool declares whether it's exposed during the **plan phase**, the
**main task**, or both:

| Tool | Plan | Main | Purpose |
|---|---|---|---|
| `task_done` | ✗ | ✓ | Signal "I'm finished" — terminates the loop. |
| `code_comment` | ✗ | ✓ | Emit a review comment with line range + suggestion. |
| `file_read` | ✗ | ✓ | Read a slice of a file from the post-change snapshot. |
| `file_read_diff` | ✓ | ✓ | Read another file's diff to confirm a cross-file concern. |
| `file_find` | ✓ | ✓ | Locate files by filename keyword. |
| `code_search` | ✓ | ✓ | Grep across the repo (literal or regex). |

`task_done` and `code_comment` are intentionally **not** available
during the plan phase: planning is read-only.

> **Context tools are read-only context, not comment targets.** The
> `main_task` prompt explicitly forbids commenting on findings in
> *other* files. `file_read`, `file_read_diff`, `file_find`, and
> `code_search` exist so the model can understand the current file's
> diff better — any issue spotted while gathering that context is
> ignored by design. Cross-file concerns surface as comments only when
> they're observable from the **current file's diff**.

To override the tool registry, pass `--tools <path>` to a JSON file with
the same shape as the embedded one. This lets you disable a tool, edit
a description, or add a new tool backed by an existing provider.

## `task_done`

Terminate the main loop.

```json
{
  "name": "task_done",
  "input": { "state": "DONE" }
}
```

| Field | Required | Meaning |
|---|---|---|
| `state` | yes | `DONE` (default) or `FAILED`. `FAILED` is for "I literally cannot use the available tools to do this" — almost never the right choice. |

When the agent sees `task_done`, it stops calling the LLM and starts
processing accumulated `code_comment` calls. `task_done` returns
immediately (before the result is recorded in the session log), so the
`state` value is accepted but **not** persisted — it doesn't affect exit
codes either.

## `code_comment`

Emit one or more review comments. Each comment is anchored to a code
snippet (`existing_code`) so OCR can compute line numbers automatically.

### Schema

```json
{
  "name": "code_comment",
  "input": {
    "path": "string — optional, override the file path for this comment",
    "comments": [
      {
        "content": "string — the comment in the configured language",
        "existing_code": "string — snippet from the diff to anchor on",
        "suggestion_code": "string — optional fix snippet",
        "thinking": "string — optional, the model's reasoning for this comment"
      }
    ]
  }
}
```

`comments` is an array, so the model can emit several comments in one
tool call. `content` and `existing_code` are required; `suggestion_code`
is optional but encouraged. `path` is a top-level optional override —
if omitted, the agent injects the file currently under review. The
agent also injects `path` automatically when the model leaves it out, so
the model rarely needs to set it explicitly. `thinking` (per-comment)
captures the model's reasoning and is preserved on the comment but not
shown in the final review output.

> **`thinking` is a runtime-only field.** OCR parses and stores it, but
> it is deliberately **not** listed in the `code_comment` schema
> advertised to the model in `tools.json` (only `content`,
> `existing_code`, and `suggestion_code` are). Stronger models that
> emit a `thinking` block anyway will have it persisted; most won't send
> it, which is fine.

### Anchoring algorithm

OCR walks the diff looking for the text in `existing_code` using a
**dynamic sliding window**. The match tries, in order:

1. **Hunk new-side** — a run of consecutive **context + added** lines
   (not deleted-only, not unchanged-only), yielding new-file line
   numbers. If that fails, OCR retries the **hunk old-side** — context +
   deleted lines — yielding old-file line numbers.
2. **Full new-file scan** — if no hunk matched, OCR scans the entire
   post-change file content line-by-line for consecutive matches
   (`resolveFromFileContent`).
3. **Re-location task** — if text matching still fails on a non-trivial
   diff, OCR runs the `RE_LOCATION_TASK` prompt asking the model to
   re-anchor the snippet.

Matching is **whitespace-insensitive**: lines are trimmed and diff
`+`/`-` markers stripped before comparison, so indentation need not
match exactly. As a last resort the comment is delivered with
`start_line=0`, telling the user "the issue is real but you'll need to
find the spot yourself".

### Example

```json
{
  "comments": [
    {
      "content": "`tx.Rollback()` is never deferred — early returns leak the transaction.",
      "existing_code": "tx, err := db.Begin()\nif err != nil {\n    return err\n}",
      "suggestion_code": "tx, err := db.Begin()\nif err != nil {\n    return err\n}\ndefer tx.Rollback()"
    }
  ]
}
```

## `file_read`

Read a range of lines from a file in its **post-change** form.

### Schema

```json
{
  "name": "file_read",
  "input": {
    "file_path": "src/foo.go",
    "start_line": 10,
    "end_line": 80
  }
}
```

| Field | Required | Default | Notes |
|---|---|---|---|
| `file_path` | yes | — | Path relative to the repo root. |
| `start_line` | no | `1` | 1-indexed. |
| `end_line` | no | end of file | Inclusive. |

### Output

```
File: src/foo.go (Total lines: 220)
IS_TRUNCATED: false
LINE_RANGE: 10-80
10|package foo
11|
12|import (
13|    "fmt"
…
```

Each content line is prefixed with its 1-indexed line number and a `|`
separator so the model can quote line numbers precisely in subsequent
`code_comment` calls.

### Limits

- **500 lines max per call.** Larger ranges are truncated, `IS_TRUNCATED:
  true` is set, and a trailing `Note: Results truncated to 500 lines.
  Please narrow your line range.` is appended.
- Reads only the **modified version** of the file. To see the old
  version, use `file_read_diff`.

When the model needs surrounding context (for a comment about a
function it can only see in the diff), it should compute the range
from the diff hunk header `@@ -x,y +m,n @@` — typically `m-50` to
`m+n+50`.

## `file_read_diff`

Read the diff for one or more *other* files in the same change set —
useful when a comment hinges on whether a related file was updated.

### Schema

```json
{
  "name": "file_read_diff",
  "input": {
    "path_array": ["src/api/handler.go", "src/db/queries.go"]
  }
}
```

### Output

```
==== FILE: src/api/handler.go ====
--- a/src/api/handler.go
+++ b/src/api/handler.go
@@ -10,1 +10,2 @@
- old line
+ new line 1
+ new line 2

==== FILE: src/db/queries.go ====
@@ -5,1 +5,1 @@
- query := "SELECT *"
+ query := "SELECT id"
```

If a path isn't in the change set, that entry is silently omitted. If
**none** of the requested paths are in the change set the tool returns
`Error: diff not found for the requested paths`; an empty `path_array`
returns `Error: no files found`.

## `file_find`

Find files in the repo by filename keyword (substring match).

### Schema

```json
{
  "name": "file_find",
  "input": {
    "query_name": "UserService",
    "case_sensitive": false
  }
}
```

| Field | Required | Default | Notes |
|---|---|---|---|
| `query_name` | yes | — | Substring matched against each file's **basename** (the part after the last `/`), not the full path. |
| `case_sensitive` | no | `false` | Set to `true` for exact-case matching. |

The candidate set comes from `git ls-files --cached --others
--exclude-standard` in workspace mode, or `git ls-tree -r --name-only
<ref>` in range / commit mode. Extensionless files are skipped except
for `Makefile`, `Dockerfile`, `LICENSE`, `Vagrantfile`, `Containerfile`.

### Output

A newline-separated list of paths:

```
src/main/java/com/example/UserService.java
src/test/java/com/example/UserServiceTest.java
src/main/java/com/example/internal/UserServiceImpl.java
```

When no file matches (or `query_name` is blank), the tool returns the
literal string `// The file was not found`.

### Limits

Returns up to **100** matches; excess is silently truncated. If the
model needs broader search, it should fall through to `code_search`.

## `code_search`

Full-text search across the repo. Backed by `git grep`, so it
understands `pathspec` syntax and respects `.gitignore`.

### Schema

```json
{
  "name": "code_search",
  "input": {
    "search_text": "TODO|FIXME",
    "file_patterns": ["*.go", ":(exclude)vendor/"],
    "case_sensitive": false,
    "use_perl_regexp": true
  }
}
```

| Field | Required | Default | Notes |
|---|---|---|---|
| `search_text` | yes | — | Literal string or PCRE pattern (see `use_perl_regexp`). |
| `file_patterns` | no | whole repo | Array of pathspec entries. Use `:(exclude)pat` to subtract. |
| `case_sensitive` | no | `false` | — |
| `use_perl_regexp` | no | `false` | When `true`, `search_text` is treated as a regex. |

### Output

Results are grouped by file. Each group starts with `File: <path>` and
`Match lines: <n>`, followed by one `line|content` line per hit:

```
File: path/to/example.java
Match lines: 2
433|      String name = toolRequest.get().getName();
438|      logToolRequest(newPath, tool, toolRequest.get());

File: path/to/other.java
Match lines: 1
22|      var req = new ToolRequest();
```

When there are no matches, the tool returns the literal string
`No matches found`.

### Pathspec cookbook

| Goal | `file_patterns` |
|---|---|
| Single file | `["src/main.go"]` |
| All Go files | `["*.go"]` |
| All Go except tests | `["*.go", ":(exclude)*_test.go"]` |
| Only one directory | `["src/api/"]` |
| Multiple types, no vendor | `["*.go", "*.ts", ":(exclude)vendor/", ":(exclude)node_modules/"]` |

### Limits

- Caps matches at **100 per file** via `git grep --max-count 100`, so
  total output across many files can exceed 100. When the per-file cap
  is hit the output is prefixed with `Note: The results have been
  truncated. Only showing first 100 results.`.
- Empty / whitespace-only `search_text` returns `Error: search_text is
  blank` instead of expanding to every line.
- Searches the **current working tree** in workspace mode, or the
  resolved target ref in range / commit mode (the `FileReader.Ref` is
  passed as a positional argument to `git grep`).

## Tool execution & errors

Tools execute synchronously inside the agent loop, with two exceptions:

- `code_comment` is dispatched to the **CommentWorkerPool** so the loop
  doesn't block on line-resolution + reflection.
- `task_done` short-circuits — it returns immediately without invoking
  any provider.

When a tool errors (network failure, malformed args, file not found),
the result is delivered to the model as a regular tool result with text
like `"Error: file not found: src/missing.go"`. The model then decides
whether to retry, ask for a different file, or call `task_done`.

If a tool name isn't in the registry, OCR returns the constant
`tool.NotAvailableMsg` rather than crashing. This makes runtime tool
disabling (via `--tools`) safe.

## Customizing tools

Two paths to extend:

### 1. Disable a tool

Copy `tools.json`, drop the entry you don't want, then run:

```bash
ocr review --tools ./my-tools.json
```

For example, if you want a "comment-only" reviewer that never reads
extra context, keep only `code_comment` and `task_done`.

### 2. Re-describe a tool

Keep the `name` (the providers are looked up by name internally) but
change the `description` to nudge the model. This is the easiest way to
inject project-specific guidance — e.g., "When using `file_read`,
always read at least 30 lines around the change."

> Adding **new** tool *names* requires Go-side wiring; see
> `internal/tool/definitions.go` and the providers under
> `internal/tool/`. The JSON file alone can't add new behaviour.

## See Also

- [Architecture](../architecture/) — how the agent loop drives tools.
- [Review Rules](../review-rules/) — what the LLM is told to focus on.
- [Session Viewer](../viewer/) — see exactly which tools fired in past
  reviews.
