---
title: Agent Skill
sidebar:
  order: 1
---

Register OCR as a callable skill so an agent framework can invoke it
with the right flags, prerequisite checks, and triage rubric — without
you re-deriving any of that on the calling side.

## What ships in the repo

The repo ships a SKILL manifest at
[`skills/open-code-review/SKILL.md`](https://github.com/alibaba/open-code-review/blob/main/skills/open-code-review/SKILL.md).
It declares OCR as a callable skill, with prerequisite checks, an
invocation workflow, and a comment-triage rubric (High/Medium/Low).

## Install

### Option 1: `npx skills add` (recommended)

Run from inside the project where you want the skill available:

```bash
npx skills add alibaba/open-code-review --skill open-code-review
```

This pulls the manifest from the
[skills registry](https://github.com/alibaba/open-code-review/blob/main/skills/open-code-review/SKILL.md)
and drops it into the project so any coding agent that respects the
skills convention picks it up on the next invocation. Re-run the
command to update the skill to the latest version.

> **Prerequisite:** the skill will install the `ocr` CLI itself the
> first time it runs (via `npm install -g @alibaba-group/open-code-review`)
> if the binary isn't on `PATH` — see [What the skill does](#what-the-skill-does)
> below. You **do** need an LLM configured up front; the skill cannot
> do that for you and will stop and ask. See [Configuration](../../configuration/).

### Option 2: Manual copy (system-wide)

If you'd rather install the skill globally instead of per-project, copy
the folder into your skills directory:

```bash
mkdir -p ~/.claude/skills
cp -R /path/to/open-code-review/skills/open-code-review ~/.claude/skills/
```

This makes the skill available to every project on the machine.

## What the skill does

The SKILL.md is a prompt: when the calling agent loads it, the agent
itself executes the steps. End-to-end, a single `/open-code-review`
(or equivalent) request unfolds like this:

1. **Prerequisite check.** Run `which ocr` to confirm the CLI is on
   `PATH`, then `ocr llm test` to confirm an LLM is reachable.
2. **Auto-install the CLI if missing.** If `which ocr` reports
   "NOT INSTALLED", the agent runs
   `npm install -g @alibaba-group/open-code-review` and continues. No
   user prompt — this is treated as a routine setup step.
3. **Stop and ask if no LLM is configured.** If `ocr llm test` fails,
   the agent will *not* invent credentials. It shows the user the two
   supported options (environment variables or `ocr config set …`) and
   waits for the user to provide an API key.
4. **Extract business context.** Inspect the review target (commits,
   branch, working copy) and synthesise a short `--background` string.
5. **Run the review.** Invoke
   `ocr review --audience agent --background "…" [--commit | --from/--to]`,
   picking flags based on whether the user asked to review the working
   copy, a specific commit, or a branch range.
6. **Classify and report.** Group the JSON comments into **High** /
   **Medium** / **Low** using the rubric in SKILL.md (bugs and
   security issues are High; nitpicks and likely false positives are
   silently dropped), then render a Markdown summary.
7. **Fix on request.** If the user said "review **and** fix" (or
   similar), apply safe fixes to High/Medium items inline; otherwise
   ask before touching the code.

The full prompt — including the exact triage rubric, output template,
and gotchas — lives in
[`skills/open-code-review/SKILL.md`](https://github.com/alibaba/open-code-review/blob/main/skills/open-code-review/SKILL.md).
Edit your local copy if you want to tighten any of the above (e.g.,
flip the default to always-ask before fixing).

## Anthropic Agent SDK

Point your SDK init at the installed skill path:

```python
from anthropic_agent_sdk import Agent

agent = Agent(
    skill_paths=["/path/to/open-code-review/skills/open-code-review"],
)

agent.run("Review my staged changes — focus on race conditions.")
```

The SDK loads the SKILL.md prompt and the agent executes the workflow
described in [What the skill does](#what-the-skill-does) — including
the `npm install` fallback and the prompt-for-credentials step if no
LLM is configured.

## Other agent frameworks

Any framework with a "register external skill" surface can ingest the
SKILL.md — it's just markdown with frontmatter. If your framework
expects a different schema, the markdown body is still useful as a
prompt template.

## See Also

- [Command（Claude Code Plugin）](../claude-code/) — the
  slash-command flavor of the same skill.
- [Direct Subprocess](../subprocess/) — bypass the manifest and call
  the CLI yourself.
