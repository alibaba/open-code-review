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
  ocr review --background "Focus on auth"                                           Review with inline context
  ocr review -B ./docs/requirements.md                                              Review with context file
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

## Global flags

Available on every command, and accepted either before or after the subcommand
(`ocr --color=never review` and `ocr review --color=never` are equivalent).

| Flag | Default | What it does |
|---|---|---|
| `--color <auto\|always\|never>` | `auto` | When to emit ANSI color. `auto` colorizes only when stdout is a terminal, so piping or redirecting yields plain text. `always` keeps color through a pipe (useful for `\| less -R`). |

Text output is plain whenever stdout is not a terminal, so it can be piped
safely:

```bash
ocr review --commit HEAD | gh issue comment 123 --body-file -
```

`TERM=dumb` also disables color.

## Command summary

| Command | Alias | What it does |
|---|---|---|
| `ocr review` | `ocr r` | Run a code review and emit comments. |
| `ocr scan` | `ocr s` | Scan complete files without requiring a Git diff. |
| `ocr rules check <file>` | — | Show which rule applies to a given file path and where it came from. |
| `ocr config set <key> <value>` | — | Persist a config value to `~/.opencodereview/config.json`. |
| `ocr config unset custom_providers.<name>` | — | Delete a custom provider (clears active `provider`/`model` if it was active). |
| `ocr config provider` | — | Interactive provider-setup TUI. |
| `ocr config model` | — | Interactive model-selection TUI. |
| `ocr llm test` | — | Send a small chat request to verify the configured endpoint. |
| `ocr llm providers` | — | List all built-in LLM providers. |
| `ocr session list` | `ocr sessions list`, `ocr session ls` | List saved review sessions. |
| `ocr session show <id>` | `ocr sessions show <id>` | Inspect one session and its per-file checkpoints. |
| `ocr session comments <id>` | `ocr sessions comments <id>` | Print the review comments recorded in one session. |
| `ocr session compare <before> <after>` | `ocr session diff <before> <after>` | Compare two sessions' findings: new, persisting, resolved, not reviewed. |
| `ocr viewer` | — | Launch the local web UI for past review sessions (`localhost:5483`). |
| `ocr version` | — | Print version, commit, platform, build date, and GitHub URL. |

`ocr` and `ocr -h` print top-level usage. Each subcommand also accepts
`-h` / `--help`.

## `ocr review`

The main command. Resolves a Git diff, groups the changed files
semantically, dispatches one sub-agent per group, collects review
comments, and prints them.

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
| `--preview` | `-p` | `false` | Run the filter pipeline but skip the LLM. Prints the file list and exclusion reasons. Honors `--format json`; `--format sarif` is not supported (a preview has no completed findings to emit). |
| `--no-filter` | — | `false` | Keep all review comments and skip the per-group `REVIEW_FILTER_TASK` LLM post-processing call. |
| `--resume <session-id>` | — | — | Resume from a previous compatible range or commit review session. |
| `--format <fmt>` | `-f` | `text` | `text` (human-readable), `json` (machine-readable comment array), or `sarif` (SARIF 2.1.0 report for GitHub Code Scanning). |
| `--output <path>` | `-o` | stdout | Write review results to a UTF-8 file (`-` means stdout). Lazily created on first write so failed runs leave existing files untouched. Text format automatically strips ANSI color codes. |
| `--audience <who>` | — | `human` | `human` streams progress lines (to stderr when `--format` is `json`/`sarif`, so stdout stays a single parseable document); `agent` suppresses progress entirely and prints only the final summary / JSON. |
| `--background <text>` | `-b` | — | Optional requirement / business context injected into the plan + main prompts. |
| `--background-file <path>` | `-B` | — | Path to a Markdown file used as review background. Takes precedence over `--background` when both are set. |
| `--exclude <patterns>` | — | — | Comma-separated gitignore-style patterns to exclude; merged with the `excludes` section of `rule.json` |
| `--concurrency <n>` | — | `8` | Maximum number of file groups reviewed in parallel. |
| `--timeout <minutes>` | — | `15` | Per-group deadline. `0` disables the timeout. Scaled linearly by the number of effort review rounds (e.g. 15/30/45 min for low/medium/high). |
| `--effort <level>` | — | `medium` | Review effort preset: `low` (1 review round), `medium` (2 rounds), `high` (3 rounds). More rounds improve recall at proportionally higher cost. Overrides the saved `effort` setting for this run. |
| `--rule <path>` | — | — | Path to a custom JSON review rule file. Overrides the project-level and global `rule.json`. |
| `--max-tools <n>` | — | template default | Max tool-call rounds per group. `0` uses the template default (`100`); values 1–49 are clamped up to `50`. The flag only ever *raises* the cap — a value below the template default is ignored. |
| `--max-tokens <n>` | — | config or template default | Prompt (input) token ceiling per group; the template default is `200000`. Overrides the saved `max_tokens` setting for this run. Does not change the output cap — see `MAX_COMPLETION_TOKENS`. |
| `--max-tokens-budget <n>` | — | `0` (unlimited) | Cap total input + output token usage for the review. Dispatch stops once the budget is exceeded and partial results are still published. |
| `--provider <name>` | — | — | Select a configured provider for this run. Names under both `providers` and `custom_providers` are accepted. |
| `--model <name>` | — | — | Override the resolved LLM model for this run (e.g., `claude-opus-4-6`). |
| `--max-git-procs <n>` | — | `16` | Maximum number of concurrent git subprocesses. |
| `--tools <path>` | — | embedded | Path to a custom JSON tool-config file. Overrides the embedded tool definitions. |

> Mode flags are mutually exclusive: pass either `--from`/`--to`, or
> `--commit`, or neither (workspace mode). Mixing them is a hard error.
> `--resume` supports only range or commit reviews and cannot be combined
> with `--preview`.

### Per-run LLM selection

Both `review` and `scan` accept `--provider` and `--model`. The overrides
apply only to the current invocation and do not modify saved configuration:

```bash
ocr review --provider anthropic --model claude-opus-4-6 --format json
ocr scan --provider openai --model gpt-5.4 --format json
```

An explicit `--provider` selects a saved entry from `providers` or
`custom_providers` before normal source resolution. Without `--provider`, OCR
preserves the legacy source order: saved configuration, complete `OCR_LLM_*`
environment configuration, complete Claude Code environment configuration, then
shell rc files. `--model` overrides the model within whichever source wins; it
does not change that source order. Incomplete strategies fall through without
being mixed. A selected built-in provider's credentials may still come from its
supported environment variable.

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
ocr session comments <session-id>
ocr review --from main --to feature-branch --resume <session-id>
ocr review --commit abc123 --resume <session-id>
```

Resume is strict by design. Checkpoints are only reused when the resumed run
would review the same thing the parent did:

- workspace reviews cannot be resumed
- the review mode must match: a range session cannot be resumed as a commit one
- the resolved input must match. Ref *spellings* are not compared — `abc1234`
  and `abc1234def` name the same commit — but if the same refs now resolve to a
  different diff, or the rules or filters changed the selected file set, the
  whole resume is rejected rather than partially reused
- a provider or model change must be asked for explicitly with `--provider` /
  `--model`. A change that arrived through config or the environment is rejected
- the parent must carry a run manifest, which is what its input is verified
  against. After file dispatch begins, Ctrl-C cancels the review gracefully and
  records one, so completed checkpoints remain resumable. A process killed
  before graceful shutdown and sessions older than run manifests do not have one
- only files the parent's manifest settled are reused. A checkpoint the manifest
  does not account for, or one that is unreadable, costs that file its
  checkpoint and nothing more — it is simply reviewed again
- `--preview` and `--resume` cannot be used together

A rejected resume writes nothing: no session, no manifest, no LLM call.

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

The document is always written to stdout on its own. With the default
`--audience human` the `[ocr]` progress lines stream to **stderr** as the review
runs, so you can watch a long run and still pipe stdout straight into a parser:

```bash
ocr review --format json > result.json   # progress still visible on the terminal
ocr review --format json | jq .summary   # stdout is a single JSON document
```

Pass `--audience agent` to drop the progress lines altogether, or `2>/dev/null`
to discard them at the shell.

```json
{
  "status": "success",
  "llm": {
    "provider": "anthropic",
    "model": "claude-opus-4-6"
  },
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
| `llm` | Resolved LLM identity. The normalized `model` is always present; `provider` is present only for a named configured provider. |
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
  "llm": {
    "provider": "anthropic",
    "model": "claude-opus-4-6"
  },
  "comments": []
}
```

### Exit codes

| Code | Meaning |
|---|---|
| `0` | Review completed (possibly with zero comments, possibly with non-fatal warnings). |
| `1` | Fatal error — bad flags, can't resolve LLM endpoint, all per-group sub-agents failed, etc. The error text is printed to stderr. |

Non-fatal warnings (a single sub-agent failed, a file exceeded the token
threshold, etc.) are printed inline; in JSON mode they're added to the
`warnings` array.

## `ocr scan`

Full-file review without a Git diff. Each file's current content is read
from the working tree and sent to the LLM — useful for auditing an
unfamiliar codebase or a directory with no meaningful diff.

```text
ocr scan [flags]
ocr s      [flags]   (alias)
```

With no `--path`, the whole repository is scanned.

### Flags

| Flag | Short | Default | Description |
|---|---|---|---|
| `--path <list>` | - | whole repo | Comma-separated repo-relative directories or files to scan (e.g., `internal/agent`, `internal/llm/client.go`). |
| `--exclude <patterns>` | - | - | Comma-separated gitignore-style patterns to skip (e.g., `**/generated/*,*.pb.go`); merged with `rule.json` excludes. |
| `--output <path>` | `-o` | stdout | Write scan results to a UTF-8 file (`-` means stdout). Lazily created on first write so failed runs leave existing files untouched. Text format automatically strips ANSI color codes. |
| `--preview` | `-p` | `false` | Enumerate and filter files without calling the LLM. Prints the file list, reviewable/excluded counts, total lines, and per-file exclusion reasons. Honors `--format json`; `--format sarif` is not supported. |

```bash
ocr scan --preview                              # see what would be scanned
ocr scan --path internal/agent                  # scan one directory
ocr scan --path internal/agent,internal/llm/client.go
ocr scan --exclude '**/generated/*,*.pb.go'
```

See `ocr scan -h` for the full flag list.

## `ocr session`

Lists and inspects local review session logs saved under
`~/.opencodereview/sessions/`. Use it to find a session ID, inspect
per-file checkpoint status, and resume interrupted range or commit reviews.

```text
ocr session <sub-command>
ocr sessions <sub-command>   (alias)

Sub-commands:
  list, ls        List recent review sessions for the current repo
  show <id>       Show one session's metadata and per-file items
  comments <id>   Show the review comments recorded in one session
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

A resumed run also prints the run it continued, and the provider/model
transition when the resume crossed one.

```bash
ocr session show <session-id>
ocr session show --json <session-id>
ocr session show --repo /path/to/repo <session-id>
```

| Flag | Default | Description |
|---|---|---|
| `--repo <path>` | current dir | Repository whose session should be inspected. |
| `--json` | `false` | Emit session metadata and per-file items as JSON. |

### `ocr session comments`

Prints every review comment persisted in a session, rendered in the same
style as `ocr review` terminal output (path, line range, severity badge,
suggestion diff).

```bash
ocr session comments <session-id>
ocr session comments --json <session-id>
ocr session comments --severity high <session-id>
ocr session comments --severity critical,high --category bug,security <session-id>
```

| Flag | Default | Description |
|---|---|---|
| `--repo <path>` | current dir | Repository whose session should be inspected. |
| `--json` | `false` | Emit the comments as a JSON array. |
| `--severity <list>` | all | Comma-separated severities to include (`critical`, `high`, `medium`, `low`). |
| `--category <list>` | all | Comma-separated categories to include (e.g. `bug`, `security`). |

### `ocr session compare`

Groups the findings of two sessions into four buckets: **new** (only in the
after session), **persisting** (in both), **resolved** (only in the before
session) and **not reviewed** (in the before session, in files the after
session never looked at, so they are not counted as resolved).

Findings are matched on path, category and the offending snippet, not on line
numbers, so a finding that only moved down the file still counts as
persisting.

```bash
ocr session compare <before-session-id> <after-session-id>
ocr session diff <before-session-id> <after-session-id>
ocr session compare --json <before-session-id> <after-session-id>
```

Both sessions must belong to the same repository; otherwise the command
fails. Different review modes only print a warning on stderr, so `--json`
output stays pipeable.

| Flag | Default | Description |
|---|---|---|
| `--repo <path>` | current dir | Repository whose sessions should be compared. |
| `--json` | `false` | Emit the comparison as JSON (`new`, `persisting`, `resolved`, `not_reviewed`). |

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
ocr config unset <key>                     Clear a saved key
ocr config provider                        Interactive provider setup
ocr config model                           Interactive model selection
```

- **`set`** — write a single config value non-interactively. `effort`
  accepts `low` / `medium` / `high` and sets the default review effort for
  every run; `--effort` overrides it per invocation.
- **`unset`** — clear a saved key. `provider`, `max_tokens`, `effort`,
  `custom_providers.<name>`, and `mcp_servers.<name>` are supported.
  Clearing `effort` restores the default `medium` preset. If a deleted
  provider was the active one, `provider` and `model` are cleared (run
  `ocr config provider` to pick a new one).
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


## ocr completion

Generate shell completion scripts for `ocr`, so command names, flags,
and arguments can be tab-completed in your shell.

### Bash

One-shot, current session only:

```bash
source <(ocr completion bash)
```

Persistent (Linux):

```bash
ocr completion bash > /etc/bash_completion.d/ocr
```

Persistent (macOS):

```bash
ocr completion bash > $(brew --prefix)/etc/bash_completion.d/ocr
```

### Zsh

If shell completion isn't already enabled in your environment, enable
it once:

```bash
echo "autoload -U compinit; compinit" >> ~/.zshrc
```

Then load completions persistently:

```bash
ocr completion zsh > "${fpath[1]}/_ocr"
```

Start a new shell for this to take effect.

### Fish

One-shot, current session only:

```bash
ocr completion fish | source
```

Persistent:

```bash
ocr completion fish > ~/.config/fish/completions/ocr.fish
```

### PowerShell

One-shot, current session only:

```powershell
ocr completion powershell | Out-String | Invoke-Expression
```

Persistent — generate the script once, then source it from your profile:

```powershell
ocr completion powershell > ocr.ps1
```

Add a line to your PowerShell profile that dot-sources `ocr.ps1`.


## Tips & gotchas

- `--audience agent` does **not** imply `--format json`, and `--format json`
  does not imply a quiet terminal. They control different things — quiet UI vs
  structured payload. `--format json` alone keeps progress visible on stderr;
  add `--audience agent` when you want it gone.
- `--background` is one of the highest-leverage flags for review quality —
  always pass the requirement / PR description when invoking from another
  agent.
- A file whose diff alone exceeds 80 % of `MAX_TOKENS` (`200000` by default)
  is dropped before the LLM is called. This is logged but does not fail
  the run.
- `MAX_TOKENS` caps the **prompt** only. The model's output is capped
  separately by `MAX_COMPLETION_TOKENS` (`16384`), so raising
  `--max-tokens` for a large-context model does not inflate output cost.
- The plan phase is **automatically skipped** when changed lines fall below
  both `PLAN_MODE_LINE_THRESHOLD` (`50`, applied to the largest single file
  in the group) and `PLAN_MODE_GROUP_LINE_THRESHOLD` (`100`, applied to the
  combined churn of a group with 2+ files).
- Files are bundled into semantic groups by one cheap metadata-only LLM
  call before review, so related changes (handler + service + test) are
  reviewed in a single conversation. Grouping failures fall back silently
  to one-file-per-group.
- `--effort high` is the cheapest quality lever when a review feels
  shallow; `--effort low` roughly halves cost versus the default when you
  just want a fast sanity pass.

## See Also

- [QuickStart](../quickstart/) — install and run your first review.
- [Configuration](../configuration/) — env vars and config keys behind the flags.
- [Review Rules](../review-rules/) — the `--rule` flag and rule resolution.
- [Integrations](../integrations/agent-skill/) — calling `ocr review` from agents and CI.
