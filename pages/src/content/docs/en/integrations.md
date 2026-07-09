---
title: Integrations
sidebar:
  order: 12
---

OCR is a CLI; it composes with anything that can spawn a process. This
section covers the first-class ways to wire it into agentic workflows
and CI, with one page per integration method.

## Why these particular integrations?

OCR's `--audience agent` mode is purpose-built for being driven by
another agent: stdout carries only the JSON / final summary, no
progress UI. That makes three composition patterns natural:

1. **Agent skill** — register OCR as a skill the calling agent can
   invoke (e.g., the Anthropic Agent SDK).
2. **Command (Claude Code plugin)** — install the bundled command so
   `/open-code-review:review` runs `ocr review` end-to-end. Also works
   in any other agent that supports a Claude-Code-style command
   convention.
3. **Direct subprocess** — any framework that can call `subprocess.run`
   (LangChain tool, custom shell, CI step) just shells out.

You can mix and match. The skill and plugin both end up calling the
same binary.

## Pick a pattern

| Method | Best when | Page |
|---|---|---|
| Agent skill | You're building on the Anthropic Agent SDK or another framework that consumes a `SKILL.md`. | [Agent Skill](agent-skill/) |
| Command (Claude Code plugin) | You use Claude Code (or any agent with a Claude-Code-style command convention) and want `/open-code-review:review` to do the right thing. | [Command（Claude Code Plugin）](claude-code/) |
| Direct subprocess | You need to call OCR from a custom script, LangChain tool, or non-Anthropic agent. | [Direct Subprocess](subprocess/) |
| CI/CD | You want OCR to run on every PR or pre-commit. | [CI/CD](ci/) |

## What about MCP?

OCR doesn't expose a Model Context Protocol server today. The intended
integration surface is "agent calls CLI", which is simpler and avoids
the long-running-process issues an MCP server would introduce. If your
agent platform requires MCP specifically, wrap the CLI with a thin
shim — a 30-line Node script that exposes a single `review` tool is
enough.

The reverse direction *is* supported: OCR can act as an MCP **client** and
pull tools from external MCP servers into a review. See
[MCP Servers](../mcp/).

## Tips that apply to every pattern

- **Always pass `--audience agent`** when the caller is non-human.
  Otherwise progress lines pollute the parsed output.
- **Always pass `--background`** when you have PR / requirement
  context. Quality gain is large, cost is one tool argument.
- **Set `--concurrency`** lower in CI (`--concurrency 4`) to stay below
  vendor rate limits. Default is 8.
- **Prefer `--from origin/main --to HEAD`** in CI over `--commit HEAD`
  — the merge-base computation excludes unrelated changes that landed
  on `main` since the branch was cut.
- **Keep `OCR_LLM_TOKEN` out of stdout/logs.** OCR doesn't print it,
  but a misconfigured shell may. Use CI secret masking.

## See Also

- [CLI Reference](../cli-reference/) — every flag the review command
  takes.
- [Configuration](../configuration/) — env vars and config keys.
- [QuickStart](../quickstart/) — minimal setup for a first review.
