---
title: Command（Claude Code Plugin）
sidebar:
  order: 2
---

Install the bundled command so OCR runs end-to-end inside
[Claude Code](https://docs.anthropic.com/en/docs/claude-code) — review
the diff, classify findings, and automatically apply fixes for the
ones worth adopting.

## What ships in the repo

The repo ships a Claude Code plugin under
[`plugins/open-code-review/`](https://github.com/alibaba/open-code-review/tree/main/plugins/open-code-review).
The command prompt itself lives at
[`plugins/open-code-review/commands/review.md`](https://github.com/alibaba/open-code-review/blob/main/plugins/open-code-review/commands/review.md)
and is the source of truth for the workflow described below.

## Install

### Option 1: Plugin marketplace (recommended)

Run these two commands **inside Claude Code**:

```bash
/plugin marketplace add alibaba/open-code-review
/plugin install open-code-review@open-code-review
```

This registers the `/open-code-review:review` slash command and keeps
it updateable through `/plugin`.

### Option 2: Copy the command file directly

If you'd rather skip the plugin marketplace, drop the command file
straight into `.claude/commands/`. This registers as `/open-code-review`
(without the `:review` suffix).

**Project-level** (commit alongside the repo so the team shares it):

```bash
mkdir -p .claude/commands
curl -o .claude/commands/open-code-review.md \
  https://raw.githubusercontent.com/alibaba/open-code-review/main/plugins/open-code-review/commands/review.md
```

**User-level** (available in every project on the machine):

```bash
mkdir -p ~/.claude/commands
curl -o ~/.claude/commands/open-code-review.md \
  https://raw.githubusercontent.com/alibaba/open-code-review/main/plugins/open-code-review/commands/review.md
```

### Other agents with command support

The command file is plain markdown with a single frontmatter field —
nothing about it is Claude-Code-specific. If your agent supports a
similar **command** convention (markdown prompts loaded as invokable
commands from a directory), the file-copy recipe above is the install
path: drop `open-code-review.md` into whichever directory your agent
reads commands from, and invoke it the way your agent invokes
commands. The prompt body is agent-agnostic — it just tells the model
which `ocr` flags to pick and how to triage the output.

> **Prerequisite:** the command will install the `ocr` CLI itself the
> first time it runs (via `npm install -g @alibaba-group/open-code-review`)
> if the binary isn't on `PATH`. You **do** need an LLM configured up
> front — the command will fail if `ocr llm test` can't reach one. See
> [Configuration](../../configuration/).

## Use

In Claude Code, invoke the command by name. Use `/open-code-review:review`
if you installed via the plugin marketplace, or `/open-code-review` if
you copied the file directly:

```
/open-code-review:review
/open-code-review:review review this PR against main
/open-code-review:review focus on race conditions in commit abc123
```

The prompt parses your request and picks the right `ocr review` flags:
no arguments → workspace mode (staged + unstaged + untracked), mention
of a commit → `--commit`, mention of a branch range → `--from` / `--to`.
You can also pass OCR flags through directly (e.g.
`/open-code-review:review --commit abc123` or `--from main --to feature`).

## What the command does

The command prompt is short — three steps:

1. **Run the review.** Invoke `ocr review --audience agent` with the
   flags inferred from your request (plus an optional `--background`
   when you've described requirement context). If the `ocr` binary
   isn't on `PATH`, the command auto-installs it via
   `npm i -g @alibaba-group/open-code-review` and continues. Output is
   captured with a 5-minute timeout.
2. **Filter and evaluate.** Classify each comment as **High** /
   **Medium** / **Low**. Low-confidence comments (likely false
   positives, nitpicks, lacking context) are dropped silently; the
   rest are displayed.
3. **Fix.** Automatically apply fixes for the High/Medium items worth
   adopting. Unlike the [Agent Skill](../agent-skill/), this command
   **auto-fixes by default** — it's the right surface for a "review
   and clean up" workflow, not a "show me a diff" workflow.

If you want the command to ask before touching code, or to tighten the
triage rubric, edit your local copy of the prompt. Claude Code
re-reads commands on every invocation, so no restart is needed.

## See Also

- [Agent Skill](../agent-skill/) — the SDK-level equivalent; same
  underlying CLI, different defaults (asks before fixing).
- [Direct Subprocess](../subprocess/) — bypass the slash command and
  call the CLI yourself.
