---
title: Tools
sidebar:
  order: 9
---

OCR ships with **six built-in tools** the LLM can call during a review.
This page documents each tool's purpose, input schema, and example
input/output. The base machine-readable definitions live in
[`internal/config/toolsconfig/tools.json`](https://github.com/alibaba/open-code-review/blob/main/internal/config/toolsconfig/tools.json).

## Tool availability per phase

Task templates select tools by name from the shared definitions. In
`task_template.json`, `MAIN_TASK.tools` is the ordered list sent as callable
tools, while `PLAN_TASK.tool_references` is rendered into the planning prompt
as reference text only. Planning remains a single request without callable
tools. Neither field changes a tool's definition.

`scan_template.json` independently selects its main-task tools, omitting
`file_read_diff` because a full-file scan has no diff. An omitted or empty
list selects no tools; it never enables the entire catalog. Repeated names
in either list are included only once, in order of first occurrence in the
array. Unknown references, duplicate catalog entries, and invalid definitions
fail during task setup.

The default diff-review selections are:

| Tool             | Plan | Main | Purpose                                                   |
| ---------------- | ---- | ---- | --------------------------------------------------------- |
| `task_done`      | ✗    | ✓    | Signal "I'm finished" — terminates the loop.              |
| `code_comment`   | ✗    | ✓    | Emit a review comment with line range + suggestion.       |
| `file_read`      | ✓    | ✓    | Read a slice of a file from the selected comparison side. |
| `file_read_diff` | ✓    | ✓    | Read another file's diff to confirm a cross-file concern. |
| `file_find`      | ✓    | ✓    | Locate files by filename keyword.                         |
| `code_search`    | ✓    | ✓    | Grep across the repo (literal or regex).                  |

`task_done` and `code_comment` are intentionally **not** available
during the plan phase: planning is read-only.

> **Context tools are read-only context, not comment targets.** The
> `main_task` prompt explicitly forbids commenting on findings in
> _other_ files. `file_read`, `file_read_diff`, `file_find`, and
> `code_search` exist so the model can understand the current file's
> diff better — any issue spotted while gathering that context is
> ignored by design. Cross-file concerns surface as comments only when
> they're observable from the **current file's diff**.

Built-in tool definitions ship with OCR and cannot be overridden at runtime.
Use [review rules](../review-rules/) for project guidance and [MCP](../mcp/)
for external business context. See [migration guidance](#customizing-tools)
if you previously supplied a custom tools file.

## Comparison sides

Commit and range reviews, including resumed sessions, expose an optional `side` parameter on
`file_read`, `code_search`, and `file_find`. Omit it, or pass `"new"`, to
keep the default behavior and output format. Pass `"old"` to read, search,
or enumerate the previous side of the same comparison:

```json
{"file_path": "src/foo.go", "start_line": 10, "end_line": 80, "side": "old"}
{"search_text": "OldAPI", "file_patterns": ["src/"], "side": "old"}
{"query_name": "foo.go", "side": "old"}
```

| Review input | New side | Old side |
| --- | --- | --- |
| `--from BASE --to HEAD` | Frozen head commit | Merge-base of the frozen endpoints, not the target branch tip |
| `--commit COMMIT` | Frozen commit | First parent, including for merge commits |
| Root commit | Frozen commit | Empty tree |

The diff and all three tools share these immutable endpoints. Paths and line
numbers belong to the selected side. For renamed files, use the old path on
the old side; the tools do not translate names or fall back to another version.
Deleted files remain accessible on the old side. A root commit's old side
has no files: reads report a missing file, while searches and enumeration
return empty results. Unavailable Git objects and Git failures are errors,
not evidence of an empty tree.
Commits at a shallow boundary must be deepened before review, even if their
parent objects are already available locally, so Git's diff and old-side
tools agree on the comparison base.

Old-side results start with `SIDE: old` and `REVISION: <commit-sha>`, or
`SOURCE: empty-tree` for an empty tree. Old-side errors also identify their
source. Existing line and match limits apply to both sides. Only the strings
`"new"` and `"old"` are accepted; null, empty strings, and arbitrary refs are
invalid.

Workspace reviews and scans retain their existing definitions, prompts, and
default tool output formats. They do not expose `side` and reject it if supplied.
The shared `tools.json` catalog remains the source of the current-only
definitions and does not declare `side`. During setup, comparison reviews use
`WithComparisonSides` to derive their callable definitions and planning
references from that catalog, adding `side` and updating the descriptions
without modifying the base definitions.

Resuming a commit or range review uses the current built-in tools and requires
the same resolved old and new endpoints, even when the selected patches are
unchanged: context elsewhere in either tree may have influenced the previous
review. Different ref names that resolve to the same endpoints remain valid.
If either endpoint changes or an older session lacks enough endpoint information
to verify the comparison, start a new review instead of resuming. Completed
files are reused; pending files are reviewed with the current tools. Start a
new review to recheck every file with the current capabilities.

## `task_done`

Terminate the main loop.

```json
{
  "name": "task_done",
  "input": { "state": "DONE" }
}
```

| Field   | Required | Meaning                                                                                                                                |
| ------- | -------- | -------------------------------------------------------------------------------------------------------------------------------------- |
| `state` | yes      | `DONE` (default) or `FAILED`. `FAILED` is for "I literally cannot use the available tools to do this" — almost never the right choice. |

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
captures the model's reasoning and is preserved on the comment; OCR
backfills it from the reasoning content the model emits on the current
turn (it stays empty when the model emits none), and includes it in
the JSON output (the terminal output does not
render it).

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

Read a range of lines from a file in its **post-change** form by default.
Commit and range reviews also support the optional [comparison side](#comparison-sides).

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

| Field        | Required | Default     | Notes                           |
| ------------ | -------- | ----------- | ------------------------------- |
| `file_path`  | yes      | —           | Path relative to the repo root. |
| `start_line` | no       | `1`         | 1-indexed.                      |
| `end_line`   | no       | end of file | Inclusive.                      |

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
- Defaults to the **modified version** of the file. In comparison reviews,
  use `side: "old"` to read the full old file, or `file_read_diff` to inspect
  the patch.

When the model needs surrounding context (for a comment about a
function it can only see in the diff), it should compute the range
from the diff hunk header `@@ -x,y +m,n @@` — typically `m-50` to
`m+n+50`.

## `file_read_diff`

Read the diff for one or more _other_ files in the same change set —
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

A path that isn't in the change set is not silently dropped: it is
named in `Error: diff not found for: <path>`, and diffs for the paths
that *were* found are still returned ahead of it. When the run has
diffs for other files the message lists them (`Diffs exist for: ...`)
so a mis-spelled path can be corrected; when the run has no diffs at
all it points to `file_read` instead. An empty `path_array` returns
`Error: no files found`.

## `file_find`

Find files in the repo by repository-relative path or filename keyword (substring match).

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

| Field            | Required | Default | Notes                                                                                                            |
| ---------------- | -------- | ------- | ---------------------------------------------------------------------------------------------------------------- |
| `query_name`     | yes      | —       | Substring matched against each file's **repository-relative path** (accepting both `/` and `\` path separators). |
| `case_sensitive` | no       | `false` | Set to `true` for exact-case matching.                                                                           |

The candidate set comes from `git ls-files --cached --others
--exclude-standard` in workspace mode, or `git ls-tree -r --name-only
<ref>` in range / commit mode. Extensionless files are skipped except
for `Makefile`, `Dockerfile`, `LICENSE`, `Vagrantfile`, `Containerfile`,
`BUILD`, and `WORKSPACE`.

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

| Field             | Required | Default    | Notes                                                       |
| ----------------- | -------- | ---------- | ----------------------------------------------------------- |
| `search_text`     | yes      | —          | Literal string or PCRE pattern (see `use_perl_regexp`).     |
| `file_patterns`   | no       | whole repo | Array of pathspec entries. Use `:(exclude)pat` to subtract. |
| `case_sensitive`  | no       | `false`    | —                                                           |
| `use_perl_regexp` | no       | `false`    | When `true`, `search_text` is treated as a regex.           |

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

| Goal                      | `file_patterns`                                                    |
| ------------------------- | ------------------------------------------------------------------ |
| Single file               | `["src/main.go"]`                                                  |
| All Go files              | `["*.go"]`                                                         |
| All Go except tests       | `["*.go", ":(exclude)*_test.go"]`                                  |
| Only one directory        | `["src/api/"]`                                                     |
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
`tool.NotAvailableMsg` rather than crashing, so an unknown tool call is
reported back to the model as an unavailable tool.

## Tool customization limits and migration {#customizing-tools}

OCR uses a dedicated review agent whose core workflow and built-in tools are
validated together through tests and benchmarks, rather than a user-defined
tool registry.

**Migration:** `--tools` has been removed from both `ocr review` and `ocr scan`.
Old invocations now fail with `unknown flag: --tools`; remove the flag and its
file argument from scripts, CI jobs, and shell aliases.

- For project-specific review guidance, use [review rules](../review-rules/)
  or `--background` (`review` also supports `--background-file`).
- For requirements, knowledge bases, or other external business context,
  configure [MCP servers](../mcp/). MCP is currently connected only in `review`,
  not `scan`, and cannot replace tools with built-in names.
- Disabling built-in tools or changing their descriptions, schemas, or phase
  availability has no runtime replacement. Core changes require source edits,
  a rebuild, and appropriate tests and benchmark validation.

`--max-tools` still controls the tool-round budget. The MCP setting
`mcp_servers.<name>.tools` still filters external tools; it is unrelated to
the removed flag. Existing session files remain readable, but start a new
review rather than resuming a session that used custom tool definitions.

## See Also

- [Architecture](../architecture/) — how the agent loop drives tools.
- [Review Rules](../review-rules/) — what the LLM is told to focus on.
- [Session Viewer](../viewer/) — see exactly which tools fired in past
  reviews.
