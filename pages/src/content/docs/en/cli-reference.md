---
title: CLI Reference
sidebar:
  order: 6
---

The complete reference for every `ocr` subcommand, flag, and exit
behaviour.

## Global usage

```text
OpenCodeReview - AI-Powered Code Review CLI

Usage:
  ocr [command]

Commands:
  review, r    Start a code review
  rules        Inspect and debug review rules
  config       Manage configuration settings
  llm          LLM utility commands
  viewer       Start the WebUI session viewer
  session, sessions  List and inspect saved review sessions
  version      Show version information

Examples:
  ocr review --from master --to dev        Review diff range
  ocr review --commit abc123               Review a single commit
  ocr config provider                      Interactive provider setup
  ocr config model                         Interactive model selection
  ocr config set llm.model opus-4-6        Set a config value
  ocr llm test                             Test LLM connectivity
  ocr llm providers                        List built-in providers
  ocr session list                         List saved review sessions
  ocr version                              Show version info

Use "ocr review -h" for more information about review.
Use "ocr rules -h" for more information about rules.
Use "ocr config" for more information about config.
Use "ocr llm" for more information about LLM utilities.
Use "ocr session -h" for more information about session inspection.

GitHub: https://github.com/alibaba/open-code-review
```

## Command summary

| Command | Alias | What it does |
|---|---|---|
| `ocr review` | `ocr r` | Run a code review and emit comments. |
| `ocr rules check <file>` | — | Show which rule applies to a given file path and where it came from. |
| `ocr config set <key> <value>` | — | Persist a config value to `~/.opencodereview/config.json`. |
| `ocr config unset custom_providers.<name>` | — | Delete a custom provider (clears active `provider`/`model` if it was active). |
| `ocr config provider` | — | Interactive provider-setup TUI. |
| `ocr config model` | — | Interactive model-selection TUI. |
| `ocr llm test` | — | Send a small chat request to verify the configured endpoint. |
| `ocr llm providers` | — | List all built-in LLM providers. |
| `ocr session list` | `ocr sessions list`, `ocr session ls` | List saved review sessions. |
| `ocr session show <id>` | `ocr sessions show <id>` | Inspect one session and its per-file checkpoints. |
| `ocr viewer` | — | Launch the local web UI for past review sessions (`localhost:5483`). |
| `ocr version` | — | Print version, commit, platform, build date, and GitHub URL. |

`ocr` and `ocr -h` print top-level usage. Each subcommand also accepts
`-h` / `--help`.

## `ocr review`

The main command. Resolves a Git diff, dispatches per-file sub-agents,
collects review comments, and prints them.

### Synopsis

```text
ocr review [flags]
ocr r      [flags]   (alias)
```

If no flags are passed, OCR runs in **workspace mode** — review of all
staged + unstaged + untracked changes in the current directory's repo.

### Flags

| Flag | Short | Default | Description |
|---|---|---|---|
| `--repo <path>` | — | current dir | Git repository root. |
| `--from <ref>` | — | — | Source ref to start the diff from (e.g., `main`). |
| `--to <ref>` | — | — | Target ref to end the diff at (e.g., `feature-branch`). When set, OCR computes `merge-base(from, to)..to`. |
| `--commit <sha>` | `-c` | — | Single commit to review (vs its parent). |
| `--preview` | `-p` | `false` | Run the filter pipeline but skip the LLM. Prints the file list and exclusion reasons. |
| `--resume <session-id>` | — | — | Resume from a previous compatible range or commit review session. |
| `--format <fmt>` | `-f` | `text` | `text` (human-readable) or `json` (machine-readable comment array). |
| `--audience <who>` | — | `human` | `human` streams progress lines; `agent` quiets stdout and prints only the final summary / JSON. |
| `--background <text>` | `-b` | — | Optional requirement / business context injected into the plan + main prompts. |
| `--concurrency <n>` | — | `8` | Maximum number of files reviewed in parallel. |
| `--timeout <minutes>` | — | `10` | Per-file deadline. `0` disables the timeout. |
| `--rule <path>` | — | — | Path to a custom JSON review rule file. Overrides the project-level and global `rule.json`. |
| `--max-tools <n>` | — | template default | Max tool-call rounds per file. `0` uses the template default (`30`); values 1–9 are clamped up to `10`; any value `≥ 10` overrides the template default (even if smaller than `30`). |
| `--model <name>` | — | — | Override the resolved LLM model for this review (e.g., `claude-opus-4-6`). |
| `--max-git-procs <n>` | — | `16` | Maximum number of concurrent git subprocesses. |
| `--tools <path>` | — | embedded | Path to a custom JSON tool-config file. Overrides the embedded tool definitions. |

> Mode flags are mutually exclusive: pass either `--from`/`--to`, or
> `--commit`, or neither (workspace mode). Mixing them is a hard error.
> `--resume` supports only range or commit reviews and cannot be combined
> with `--preview`.

### Modes

#### Workspace mode (default)

```bash
ocr review
```

OCR assembles the working-tree changes from two git commands:

- tracked changes via `git diff HEAD` (staged + unstaged combined against
  `HEAD`; if that comes back empty, OCR falls back to `git diff --staged`)
- untracked files via `git ls-files --others --exclude-standard`, read
  from disk and treated as full-file additions

This is what you usually want pre-commit. Stage selectively if you want
narrower scope.

#### Range mode

```bash
ocr review --from main --to feature-branch
```

OCR computes `merge-base(main, feature-branch)..feature-branch` so you only
see the diff *introduced by* the feature branch — not unrelated changes
that landed on `main` since branching.

#### Commit mode

```bash
ocr review --commit abc123
ocr review -c abc123
```

Reviews the diff produced by `git show abc123` (i.e., the changes that
single commit introduced).

### Resuming interrupted reviews

Every `ocr review` run persists a local session log under
`~/.opencodereview/sessions/`. Successful text output stays focused on review
results and does not print the session ID; use `ocr session list/show` to find
saved sessions, or `--format json` to include `session_id` in machine-readable
output. If a range or commit review is interrupted, list saved sessions and
resume from one that matches the same review target:

```bash
ocr session list
ocr session show <session-id>
ocr review --from main --to feature-branch --resume <session-id>
ocr review --commit abc123 --resume <session-id>
```

Resume is strict by design:

- workspace reviews cannot be resumed
- range reviews must use the same `--from` and `--to`
- commit reviews must use the same `--commit`
- `--preview` and `--resume` cannot be used together

### Output

#### Text (default, `--audience human`)

Progress lines stream as the review runs, followed by one block per
comment (a dim Unicode-rule header with `path:start-end`, the comment
body wrapped to 100 columns, and — when present — a colored inline diff
of the suggested replacement). A run summary lands on stdout at the end:

```
[ocr] 17 file(s) changed, reviewing 9 in /path/to/repo
[ocr] Skipping image.png — filtered by path/extension rules
[ocr]   ▶ file_read "src/foo.go"
[ocr]   ✔ file_read (12ms)
[ocr] Plan completed for src/foo.go
…

─── src/foo.go:42-47 ───
Concurrent map access without a lock — wrap with sync.RWMutex.

- m[k] = v
+ mu.Lock(); defer mu.Unlock(); m[k] = v

…
[ocr] Summary: 9 file(s) reviewed, 14 comment(s), ~21344 token(s) used (input: ~18012, output: ~3332), 1m12s elapsed
```

#### Text (agent, `--audience agent`)

Identical comment output, but progress lines are suppressed via an internal
quiet-able stdout writer ([`internal/stdout`](https://github.com/alibaba/open-code-review/blob/main/internal/stdout/stdout.go)).
Use this in CI / when piping into another agent.

#### JSON

```bash
ocr review --format json --audience agent
```

```json
{
  "status": "success",
  "summary": {
    "files_reviewed": 9,
    "comments": 1,
    "total_tokens": 21344,
    "input_tokens": 18012,
    "output_tokens": 3332,
    "elapsed": "1m12s"
  },
  "comments": [
    {
      "path": "src/foo.go",
      "content": "Concurrent map access without a lock — wrap with sync.RWMutex.",
      "start_line": 42,
      "end_line": 47,
      "existing_code": "m[k] = v",
      "suggestion_code": "mu.Lock(); defer mu.Unlock(); m[k] = v",
      "thinking": "Looking at line 42, the map …"
    }
  ]
}
```

Top-level fields:

| Field | Notes |
|---|---|
| `status` | `success`, `completed_with_warnings`, `completed_with_errors`, or `skipped`. |
| `message` | Optional. Human-readable summary, e.g. `"No comments generated. Looks good to me."`. |
| `summary` | Optional. Run aggregates: `files_reviewed`, `comments`, `total_tokens`, `input_tokens`, `output_tokens`, `cache_read_tokens` (omitempty), `cache_write_tokens` (omitempty), `elapsed`. Omitted for `skipped` runs. |
| `comments` | Always present, possibly empty. Per-comment fields are the ones in the example above. |
| `warnings` | Optional. Present when one or more sub-agents failed; each entry describes the affected file and the error. |
| `session_id` | Optional. Present on persisted review runs; pass this to `ocr review --resume <session-id>` when retrying compatible range or commit reviews. |
| `resume` | Optional. Present on resumed runs with `resumed_from`, `reused_files`, `rerun_files`, `previous_model`, and `current_model`. |

When no files were eligible for review, JSON mode emits a `skipped`
envelope instead so callers can distinguish "no changes" from "no findings":

```json
{
  "status": "skipped",
  "message": "No supported files changed.",
  "comments": []
}
```

### Exit codes

| Code | Meaning |
|---|---|
| `0` | Review completed (possibly with zero comments, possibly with non-fatal warnings). |
| `1` | Fatal error — bad flags, can't resolve LLM endpoint, all per-file sub-agents failed, etc. The error text is printed to stderr. |

Non-fatal warnings (a single sub-agent failed, a file exceeded the token
threshold, etc.) are printed inline; in JSON mode they're added to the
`warnings` array.

## `ocr session`

Lists and inspects local review session logs saved under
`~/.opencodereview/sessions/`. Use it to find a session ID, inspect
per-file checkpoint status, and resume interrupted range or commit reviews.

```text
ocr session <sub-command>
ocr sessions <sub-command>   (alias)

Sub-commands:
  list, ls    List recent review sessions for the current repo
  show <id>   Show one session's metadata and per-file items
```

### `ocr session list`

```bash
ocr session list
ocr session list --limit 50
ocr session list --json
```

| Flag | Default | Description |
|---|---|---|
| `--repo <path>` | current dir | Repository whose sessions should be listed. |
| `--json` | `false` | Emit session summaries as JSON. |
| `--limit <n>` | `20` | Cap the number of listed sessions. Use `0` for unlimited. |

### `ocr session show`

```bash
ocr session show <session-id>
ocr session show --json <session-id>
ocr session show --repo /path/to/repo <session-id>
```

| Flag | Default | Description |
|---|---|---|
| `--repo <path>` | current dir | Repository whose session should be inspected. |
| `--json` | `false` | Emit session metadata and per-file items as JSON. |

## `ocr rules`

Rule introspection. There is exactly one subcommand:

```text
ocr rules check [flags] <file-path>

Flags:
  --repo <path>    Git repository root (default: current dir)
  --rule <path>    Path to a custom rule JSON file
```

For the given file path, OCR:

1. Walks the four-layer rule chain (custom → project → global → system).
2. Picks the first match.
3. Prints the **source layer**, the **glob pattern** that matched, and the
   resolved **rule text**.

```bash
$ ocr rules check src/main/java/com/example/Foo.java
File: src/main/java/com/example/Foo.java
Source: System built-in
Pattern: **/*.java
Rule:
────────────────────────────────────────
<contents of internal/config/rules/rule_docs/java.md>
────────────────────────────────────────
```

Useful for debugging "why isn't my custom rule firing?" — see
[Review Rules](../review-rules/) for the full priority story.

## `ocr config`

Persists keys to `~/.opencodereview/config.json` and offers interactive
setup TUIs. Four subcommands:

```text
ocr config set <key> <value>
ocr config unset custom_providers.<name>   Delete a custom provider
ocr config provider                        Interactive provider setup
ocr config model                           Interactive model selection
```

- **`set`** — write a single config value non-interactively.
- **`unset`** — delete a custom provider. Only
  `custom_providers.<name>` is supported. If the deleted provider was the
  active one, `provider` and `model` are cleared (run `ocr config provider`
  to pick a new one).
- **`provider`** — launch the interactive provider-setup TUI (no extra
  arguments; use `ocr config set provider <name>` for non-interactive
  setup).
- **`model`** — launch the interactive model-selection TUI (no extra
  arguments; use `ocr config set model <name>` for non-interactive
  setup).

See [Configuration](../configuration/) for the full key reference,
schemas, and examples.

## `ocr llm`

LLM utility commands. Two subcommands:

```text
ocr llm <sub-command>

Sub-commands:
  test         Send a test conversation to the configured LLM model
  providers    List all built-in LLM providers
```

### `ocr llm test`

```text
ocr llm test
```

Resolves the LLM endpoint exactly the way `ocr review` does, sends a single
canned chat request from
[`internal/config/testconnection/task.json`](https://github.com/alibaba/open-code-review/blob/main/internal/config/testconnection/task.json),
and prints:

```
Source: <which strategy was used>
URL:    <endpoint URL>
Model:  <effective model>
<the model's reply>
✓ Connection test successful
```

A non-zero exit means either the endpoint isn't fully configured or the
request failed (network / auth / model error). The error message tells you
which.

### `ocr llm providers`

```text
ocr llm providers
```

Lists every built-in LLM provider in a three-column table:

```
Built-in providers:
  NAME        PROTOCOL    BASE URL
  ----        --------    --------
  anthropic   anthropic   https://api.anthropic.com
  …
```

Followed by a hint to configure one interactively with `ocr config
provider` or non-interactively with `ocr config set provider <name>`.

## `ocr viewer`

```text
ocr viewer [flags]

Flags:
  --addr <address>   listen address (default: localhost:5483)

Examples:
  ocr viewer                     # start on default port
  ocr viewer --addr :3000        # bind to all interfaces on port 3000
```

Starts an embedded HTTP server that reads
`~/.opencodereview/sessions/...` and renders past review sessions in a
browser-friendly UI. See [Session Viewer](../viewer/).

## `ocr version`

```text
ocr version
ocr --version
ocr -V
```

Prints the version stamped at build time, the short Git commit (when
present), the platform (`<GOOS>/<GOARCH>`), the build date (when present),
and the GitHub URL (`https://github.com/alibaba/open-code-review`).

## Tips & gotchas

- `--audience agent` does **not** imply `--format json`. They control
  different things — quiet UI vs structured payload. Combine them when you
  want both.
- `--background` is one of the highest-leverage flags for review quality —
  always pass the requirement / PR description when invoking from another
  agent.
- A file whose diff alone exceeds 80 % of `MAX_TOKENS` (`58888` by default)
  is dropped before the LLM is called. This is logged but does not fail
  the run.
- The plan phase is **automatically skipped** when changed lines for a file
  fall below `PLAN_MODE_LINE_THRESHOLD` (`50`).

## See Also

- [QuickStart](../quickstart/) — install and run your first review.
- [Configuration](../configuration/) — env vars and config keys behind the flags.
- [Review Rules](../review-rules/) — the `--rule` flag and rule resolution.
- [Integrations](../integrations/agent-skill/) — calling `ocr review` from agents and CI.
