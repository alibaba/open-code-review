---
title: Architecture
sidebar:
  order: 8
---

A walk-through of how `ocr review` actually works inside, from the moment
you press Enter to the JSON that lands in your terminal. The goal is to
give you enough mental model to debug behaviour, tune flags, and read
the source code with confidence.

## High-level pipeline

```mermaid
flowchart TD
    A["<b>ocr review</b>"]
    B["<b>bootstrap</b><br/><span style='font-size:0.85em'>Resolve LLM endpoint (config → env → rc files)<br/>Load template, tool registry, system rules</span>"]
    C["<b>diff provider</b><br/><span style='font-size:0.85em'>git diff / ls-files / show — produce []model.Diff<br/>Modes: Workspace · Commit · Range</span>"]
    D["<b>filter & rules</b><br/><span style='font-size:0.85em'>5-gate filter (preview.go) — drop binaries,<br/>excluded paths, unsupported extensions. Pick rule per file.</span>"]
    D2["<b>semantic grouping</b><br/><span style='font-size:0.85em'>One LLM call over file metadata — bundle related<br/>files into groups (max 10 files each)</span>"]
    E["<b>subtask dispatch</b><br/><span style='font-size:0.85em'>For every group in parallel (concurrency=N):<br/>Plan phase (optional) → Main loop × rounds → Comments</span>"]
    F["<b>output writer</b><br/><span style='font-size:0.85em'>Synchronous line-resolution & review-filter; renders text<br/>or JSON depending on --format / --audience.</span>"]

    A --> B --> C --> D --> D2 --> E --> F
```

The orchestration lives in the
[`internal/agent/`](https://github.com/alibaba/open-code-review/blob/main/internal/agent/)
package, whose main files are `agent.go` (dispatch & per-group
orchestration), `grouping.go` (semantic file grouping), `preview.go`
(the file filter), and `util.go` (helpers); the tool-use loop and memory
compression live alongside it in
[`internal/llmloop/`](https://github.com/alibaba/open-code-review/blob/main/internal/llmloop/).
Two entry points matter: `Agent.Run` (top of pipeline) and
`Agent.dispatchSubtasks` (per-group fan-out).

## The diff provider

`internal/diff/git.go` defines a `Provider` struct whose unexported
`mode` field (of type `Mode`, an `int` enum) selects one of three modes
that mirror the CLI flags:

| Mode | Triggered by | What it returns |
|---|---|---|
| `Workspace` | no flags | staged + unstaged + untracked changes |
| `Commit` | `--commit <sha>` / `-c <sha>` | the changes introduced by `<sha>` (via `git show <sha>`, equivalent to the `<sha>^..<sha>` diff) |
| `Range` | `--from <a> --to <b>` | `merge-base(a, b)..b` |

Each diff carries: old/new path, old/new hunks, insertion/deletion counts,
binary flag, and rename detection. `DiffContextLines` is fixed at **3** —
the same default Git uses.

Untracked files are read from disk and treated as full-file additions so
they're reviewed pre-commit.

## The five-gate file filter

Once diffs are loaded, every file passes through
[`whyExcluded`](https://github.com/alibaba/open-code-review/blob/main/internal/agent/preview.go).
The function returns one of:

```
binary          — file is binary
user_exclude    — matched a pattern in your `exclude` list
unsupported_ext — extension is not in supported_file_types.json
default_path    — matched a built-in test-file exclude pattern
```

…or empty if the file is kept. `deleted` is **not** returned by
`whyExcluded`; it's computed afterwards in `Preview()` when a kept
file's diff reports `IsDeleted`. The gates run in this order:

1. `binary` — binary files are dropped first.
2. `user_exclude` — your project's `exclude` always wins.
3. `user_include` — if the filter has include patterns **and** the file
   matches one, it's kept immediately (returns empty), bypassing the
   `unsupported_ext` and `default_path` gates below.
4. `unsupported_ext` filters by extension allowlist.
5. `default_path` is the last gate: it matches built-in **test-file**
   exclude patterns (`**/*_test.go`, `**/*.test.{js,jsx,ts,tsx}`,
   `**/__tests__/**`, `**/*_test.py`, `**/*_spec.rb`, `**/*.test.ets`, …).
   Every pattern is rooted with a `**/` prefix.

The noisy-directory filtering (`vendor/`, `node_modules/`, `target/`, …)
happens earlier, at the diff-provider level, via the
`providerDirIgnoreDirs` list in `internal/diff/git.go` — diffs for those
directories are parsed and then stripped out by `filterDiffs` before
they ever reach the per-file filter.

Run `ocr review --preview` to see the full filter result without spending
a token. See [Review Rules](../review-rules/#how-files-are-filtered) for
the full algorithm.

## Semantic file grouping

Files that survive filtering are **not** reviewed one at a time. Before
dispatch, `groupDiffs` (in
[`grouping.go`](https://github.com/alibaba/open-code-review/blob/main/internal/agent/grouping.go))
makes a single `GROUPING_TASK` LLM call carrying only file *metadata* —
path, status (`ADDED` / `MODIFIED` / `DELETED` / `RENAMED`), and
insertion/deletion counts — never diff content. The model returns a JSON
array of `{label, files}` objects, and each group is then reviewed in one
shared conversation so the agent can reason across related changes
(handler + service + test, or a rename and its call sites).

Three guards keep groups sane:

| Guard | Effect |
|---|---|
| `maxFilesPerGroup = 10` | Oversized groups are split into 10-file chunks. |
| Token budget | A group whose combined diffs exceed the prompt limit is split back into single-file groups. |
| Coverage | Any file the model failed to assign gets its own single-file group. |

Grouping is a best-effort optimisation, never a correctness gate: a
failed, empty, or unparseable response logs a warning and falls back to
one-file-per-group dispatch — exactly the old behaviour. The resulting
grouping is also surfaced in JSON output.

## Per-group subtask: plan + main

For every group, OCR fires a sub-agent. Each sub-agent runs in its own
goroutine, bounded by `--concurrency` (default **8**), and has its own
LLM message buffer.

A subtask has up to **two phases**:

### Phase 1 — Plan (optional)

The plan phase is gated by `Template.PlanRequired`, which cooperates two
thresholds:

```go
// PLAN_MODE_LINE_THRESHOLD = 50, PLAN_MODE_GROUP_LINE_THRESHOLD = 100
if maxFileChanged >= PlanModeLineThreshold           { plan }  // one big rewrite
if fileCount >= 2 && total >= PlanModeGroupLineThreshold { plan }  // several moderate files
```

The per-file threshold catches a single large rewrite; the group
threshold catches several moderate files that together warrant
structured guidance. The group threshold is deliberately the larger of
the two so the plan phase doesn't become unconditional for multi-file
groups.

For small changes the plan adds latency without value, so it's skipped
silently and the main loop runs straight away. Otherwise OCR
makes a **single** `PLAN_TASK` LLM call — no `Tools` field is sent, so
the model cannot call tools during planning. The read-only tool subset
(`code_search`, `file_read_diff`, `file_find` — the three tools whose
`plan_task` flag is `true` in `tools.json`) is embedded as plain text
via the `{{plan_tools}}` placeholder (rendered by
`formatToolDefs`) so the model knows what's available later. The model
returns a checklist that becomes `{{plan_guidance}}`
in the main prompt.

### Phase 2 — Main loop

The main loop assembles the `MAIN_TASK` prompt and runs a tool-use
conversation with the model. The full tool set adds **`task_done`**,
**`code_comment`**, and **`file_read`** to the plan-phase tools — see
[Tools](../tools/) for the full catalogue.

```
loop up to MAX_TOOL_REQUEST_TIMES (default 100):
    response = llm.complete(messages, tools)
    if response.toolCalls is empty:
        nudge model with "You did not successfully call any tools.
                          Please try again or use task_done if finished."
        continue
    for each call: execute → collect result
    if any call was task_done: break
    addNextMessage(...)              # may trigger compression
```

The loop has five exit conditions:

1. `task_done` was called.
2. `MAX_TOOL_REQUEST_TIMES` ran out.
3. 3 consecutive rounds produced no valid tool results
   (`maxConsecutiveEmptyRounds = 3`).
4. The context was cancelled.
5. `addNextMessage` returned false — compression couldn't bring the
   message buffer back under the warning threshold.

In all cases collected `code_comment` calls become review comments.

### Review rounds

The main loop is not run once but up to `MAX_REVIEW_ROUNDS` times per
group, to improve recall on large groups. The round count is set by the
`--effort` preset:

| `--effort` | Rounds |
|---|---|
| `low` | 1 |
| `medium` (default) | 2 |
| `high` | 3 |

Each round after the first re-runs `MAIN_TASK` with the findings already
confirmed by earlier rounds injected as `{{confirmed_comments}}`, and
**without** the plan — a plan tends to act as a coverage ceiling once the
obvious issues are found. Rounds stop early when a round adds no new
findings, when the confirmed-comment cap is reached, or when the
aggregate token budget (`--max-tokens-budget`) is exhausted.

## Memory compression

A long tool-use loop will eventually overflow the context window. OCR
manages this with a **three-zone partitioning** strategy that triggers
on the prompt budget defined by `MAX_TOKENS = 200000`:

| Threshold | Constant | Action |
|---|---|---|
| 60 % of MAX_TOKENS | `tokenSoftThreshold` | Kick off **async** background compression; current loop continues uninterrupted. |
| 80 % of MAX_TOKENS | `tokenWarningThreshold` | Run compression **synchronously** before sending the next request. |

> **`MAX_TOKENS` is an *input* ceiling.** It bounds the prompt — the
> context-window budget the message buffer is compressed against — and
> nothing else. The model's *output* cap is a separate knob,
> `MAX_COMPLETION_TOKENS = 16384`, sent as `max_completion_tokens` on
> every request (`Template.CompletionTokenLimit()`). Keeping them apart
> means raising the prompt ceiling with `--max-tokens` for a
> large-context model never silently inflates the output budget. When
> `MAX_COMPLETION_TOKENS` is unset, `MAX_TOKENS` is used as the output
> cap for backwards compatibility.

### The three zones

```mermaid
flowchart LR
    subgraph messages["messages"]
        direction LR
        F["<b>frozen</b><br/>first 2 msgs<br/>(system +<br/>initial user)"]
        C["<b>compress</b><br/>summarized<br/>into one<br/>user msg"]
        A["<b>active</b><br/>K most recent<br/>complete<br/>rounds"]
    end
    F --- C --- A
```

A "round" is one assistant message plus the tool result messages that
followed it. `partitionMessages` walks rounds from the end, keeping as
many as fit within `(0.80 × MAX_TOKENS) - reservedTokens`. Everything
older becomes the **compress zone**.

The compress zone is rendered as XML and fed to the model with the
`MEMORY_COMPRESSION_TASK` prompt; the returned summary is appended to
the original user message inside `<previous_review_summary>` tags.

After compression: `messages = frozen[2] + compressed_user_msg + active`.

```go
// compression.go
func (a *Agent) runCompression(ctx context.Context, msgs []llm.Message, filePath string) ([]llm.Message, error) {
    part := partitionMessages(msgs, a.args.Template.MaxTokens, 0)
    contextXML := buildMessageXML(msgs[part.frozenEnd:part.compressEnd])
    // … call MEMORY_COMPRESSION_TASK …
    rebuilt[1] = llm.NewTextMessage(role, currentText+
        "\n\n<previous_review_summary>\n"+rawSummary+"\n</previous_review_summary>")
    for i := part.compressEnd; i < len(msgs); i++ {
        rebuilt = append(rebuilt, msgs[i])
    }
    return rebuilt, nil
}
```

### Async vs sync

The async path lets the main loop keep emitting tool calls while
compression runs in the background; when the next token check happens, a
ready summary is swapped in via `tryApplyPendingCompression`. If the
ratio crosses the warning threshold before the async job finishes, the
loop stalls and runs `runCompression` synchronously — guaranteeing the
next request always fits.

## Comment processing pipeline

Every `code_comment` tool call produces one or more raw comments. They
go through a **CommentWorkerPool** (a fixed-size goroutine pool) so the
main tool-use loop never blocks on post-processing:

1. **Line resolution** (in-worker) — `existing_code` is matched against
   the diff using a sliding-window algorithm to compute precise
   `start_line` / `end_line`. If matching fails, both default to `0` — a
   `0` line range is the implicit signal for an "unanchored" comment the
   user must locate manually (there is no stored flag; downstream
   consumers check `start_line == 0`).
2. **Re-location task** *(optional fallback)* — when line resolution
   fails on a non-trivial diff, OCR runs the `RE_LOCATION_TASK` prompt
   asking the model to re-anchor the snippet. Useful for paraphrased
   `existing_code` strings.
3. **Review filter** — after the main loop finishes (and the worker pool
   drains), the `REVIEW_FILTER_TASK` LLM call inspects the collected
   comments against the diff and removes ones that are provably
   incorrect. Errors here are logged and ignored.
4. **Second line-resolution pass** — once `Agent.Run` returns, the
   top-level command re-runs `diff.ResolveLineNumbers` over the full
   comment set (see `cmd/opencodereview/review_cmd.go`) to catch
   comments whose `existing_code` spans multiple files or was updated by
   the re-location step.
5. **Render** — into text or JSON depending on `--format`.

## Token budget guards

Before the LLM is even called, OCR runs a fail-fast check:

```go
tokenLimit := MaxTokens * 4 / 5     // 80 %
if countMessagesTokens(messages) > tokenLimit {
    record warning "token_threshold_exceeded"
    return nil      // skip this group
}
```

This catches monstrous diffs (auto-generated lock files, refactors
touching thousands of lines) before they cost a request. The skipped
group is reported as a non-fatal warning in stdout and added to the JSON
`warnings` array.

A second check runs in `filterLargeDiffs`: if the diff alone exceeds
80 % of `MAX_TOKENS` it's filtered out before grouping and dispatch even
happen. A third guard runs inside grouping — see
`enforceGroupTokenBudget` above.

## The template & placeholders

`internal/config/template/task_template.json` holds **six prompts**:

| Key | Purpose |
|---|---|
| `GROUPING_TASK` | Bundles the changed files into semantic groups. |
| `PLAN_TASK` | Planning phase — produces a checklist. |
| `MAIN_TASK` | Main review loop — emits `code_comment` calls. |
| `MEMORY_COMPRESSION_TASK` | Summarises the compress zone. |
| `REVIEW_FILTER_TASK` | Post-loop pass that removes provably-incorrect comments. |
| `RE_LOCATION_TASK` | Re-anchors a comment whose `existing_code` couldn't be matched. |

Each prompt is a list of `{role, prompt_file}` references that point to
`.md` files in the template directory (e.g.
`{"role": "system", "prompt_file": "main_task_system.md"}`). At load
time `resolveConversation` reads those files into in-memory
`{role, content}` messages, and template placeholders are then resolved
per-group:

| Placeholder | Replaced with |
|---|---|
| `{{system_rule}}` | The rule bodies resolved from the four-layer chain, merged across the group's files. |
| `{{change_files}}` | Status + path of every changed file in the PR *outside* this group. |
| `{{diffs}}` | The group's diffs, one XML element per file. |
| `{{plan_guidance}}` | Output of the plan phase, or removed when plan is skipped or on rounds 2+. |
| `{{confirmed_comments}}` | Findings confirmed by earlier review rounds (empty on round 1). |
| `{{plan_tools}}` | Plan-phase tool definitions as plain text (rendered by `formatToolDefs`), used in the `PLAN_TASK` system prompt. |
| `{{requirement_background}}` | The effective background from `--background` or `--background-file` (file takes precedence). |
| `{{current_system_date_time}}` | Local timestamp for the run, formatted `YYYY-MM-DD HH:MM` (no seconds or timezone). |
| `{{file_list}}` | (grouping only) file metadata — path, status, `+/-` counts. |
| `{{context}}` | (compression only) the XML-rendered messages to summarise. |
| `{{path}}` | Group key (comma-joined sorted paths), used in `REVIEW_FILTER_TASK`. |
| `{{comments}}` | Accumulated comments (JSON), used in `REVIEW_FILTER_TASK`. |

The placeholder substitution lives in
[`agent.go`](https://github.com/alibaba/open-code-review/blob/main/internal/agent/agent.go).
The template itself isn't a CLI override — to change prompts you edit
[`task_template.json`](https://github.com/alibaba/open-code-review/blob/main/internal/config/template/task_template.json)
and rebuild. The `--tools` flag is a *tool-registry* override (it
swaps the JSON consumed by `internal/config/toolsconfig`), not a
template override — see [Tools](../tools/#customizing-tools).

> **Placeholder syntax caveat.** All the placeholders above use
> double-brace `{{…}}` syntax *except* `RE_LOCATION_TASK`, which
> substitutes single-brace `{diff}`, `{existing_code}`, and
> `{suggestion_content}` (see `internal/diff/relocation.go`).

## Persistence

Every review is written to disk as JSONL:

```
~/.opencodereview/sessions/<encoded-repo-path>/<session-id>.jsonl
```

The repo path is **not** base64-encoded; `encodeRepoPath` (in
`internal/session/persist.go`) replaces `/` and `\` with `-` and `:` with
`_` so the path is filesystem-safe.

Each line is one event: prompt sent, LLM response, tool call, tool
result, comment emitted, etc. The Web UI (`ocr viewer`) reads these
files directly — there's no database, just append-only logs. See
[Session Viewer](../viewer/) for the UI tour and event schema.

## Telemetry

When telemetry is enabled the agent emits three pipeline-level spans
(`review.run` wrapping the whole job, `diff.parse` wrapping diff
loading, and one `subtask.execute.group.<group-key>` per reviewed
group) plus a
short-lived `event.<name>` span at each decision point (`plan.skipped`,
`token.threshold.exceeded`, `subtask.error`, …). LLM round trips and
tool calls are recorded only as metrics — not as spans. Prompt and
response content is **never** attached to telemetry; the
`OCR_CONTENT_LOGGING` flag is plumbed but currently dead. See
[Telemetry](../telemetry/) for the full schema.

## What's *not* automated

A few decisions are deliberately manual:

- **Endpoint discovery has no fallback.** If your config + env + rc
  files don't yield a complete `(URL, token, model)` triple, OCR exits
  with a non-zero code rather than guessing.
- **Sub-agent failures are isolated, not retried.** One failing group
  produces a warning; the rest continue. Retries belong in the wrapping
  CI pipeline, not the agent.
- **Cross-file reasoning is bounded by the group.** Files in the same
  semantic group share one LLM conversation, so the agent can reason
  across them directly. Files in *other* groups are reachable only
  through `file_read_diff` / `code_search` tool calls, not shared
  context, and findings in them are off-limits as comment targets — the
  `main_task` prompt instructs the model to use context tools for
  understanding only, and to ignore issues that surface outside the
  diffs it was given.

These choices keep the run **deterministic per-group** and keep cost
predictable.

## Source-code map

If you want to read along:

| Concern | File |
|---|---|
| Top-level command dispatch | `cmd/opencodereview/main.go` |
| `review` flag parsing | `cmd/opencodereview/shared_flags.go` |
| Agent orchestration | `internal/agent/` (agent.go, util.go) |
| Semantic file grouping | `internal/agent/grouping.go` |
| Tool-use loop & memory compression | `internal/llmloop/` (loop.go, compression.go) |
| Effort presets | `internal/config/template/effort.go` |
| File filter / preview | `internal/agent/preview.go` |
| Diff loading (Git modes) | `internal/diff/git.go` |
| Rule resolution chain | `internal/config/rules/system_rules.go` |
| Tool registry & impls | `internal/tool/` |
| LLM endpoint resolver | `internal/llm/resolver.go` |
| Session JSONL writer | `internal/session/persist.go` |
| Web viewer | `internal/viewer/server.go` |

See [Contributing](../contributing/) for build & test instructions.

## See Also

- [Tools](../tools/) — the six tools the agent loop calls.
- [Review Rules](../review-rules/) — how per-file rule text is resolved.
- [Session Viewer](../viewer/) — inspect the transcripts this pipeline writes.
