---
title: Agent Skill
sidebar:
  order: 1
---

Register OCR as a callable skill so an agent framework can invoke it
with the right flags and finding-verification workflow — without
you re-deriving any of that on the calling side.

## What ships in the repo

The repo ships a SKILL manifest at
[`skills/open-code-review/SKILL.md`](https://github.com/alibaba/open-code-review/blob/main/skills/open-code-review/SKILL.md).
It defines how to invoke OCR, verify its findings, report uncertainty,
and apply fixes when requested. The skill directory also includes the
GitHub PR review helper; install the whole directory, not just SKILL.md.

## Install

### Option 1: `npx skills add` (recommended)

Run from inside the project where you want the skill available:

```bash
npx skills add alibaba/open-code-review --skill open-code-review
```

This installs the skill directory from the
[skills registry](https://github.com/alibaba/open-code-review/blob/main/skills/open-code-review/SKILL.md)
and drops it into the project so any coding agent that respects the
skills convention picks it up on the next invocation. Re-run the
command to update the skill to the latest version.

> **Prerequisites:** native mode needs an OCR model provider configured
> locally; do not paste credentials into the conversation. If `ocr` is
> missing, the skill's troubleshooting instructions cover installation via
> `npm install -g @alibaba-group/open-code-review`. Only GitHub PR mode also
> needs Python 3 and authenticated `gh`. See [Configuration](../../configuration/).

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

1. **Extract business context.** Inspect the review target (commits,
   branch, working copy) and synthesise a short `--background` string.
2. **Run the requested review.** For local reviews, invoke
   `ocr review --audience agent --background "…"` directly, adding `--commit` or
   `--from`/`--to` for committed changes. The normal path does not probe
   installation or connectivity first. Fresh commit and range reviews add
   `--analysis` by default and use JSON unless another format is requested.
   Omit analysis for workspace reviews, preview, resume, SARIF, or an explicit
   request to skip it. Troubleshoot a failure before retrying.
   For a GitHub PR number or URL, use the installed
   `python3 "$SKILL_DIR/scripts/review_pr.py" <id-or-url>` helper. A number
   selects the checkout's default `gh` repository, which may be upstream
   of a fork. Report the PR URL, revisions, freshness, and artifact path.
3. **Verify and report.** Check every finding, including low-severity ones,
   against the reviewed code. Report **Reproduced** and **Uncertain** groups,
   preserving OCR's severity and order within each group. Omit a finding
   only when specific evidence contradicts it; uncertainty alone is not
   grounds to discard it. Keep the raw result unchanged and report Review
   coverage and gaps. For combined runs, render the Analysis Markdown
   separately and distinguish its execution status from the model's
   assessment of completeness.
4. **Fix on request.** Apply safe, verified fixes only when the user asks
   for edits, focusing on critical, high, and medium findings. Review-only
   requests do not authorize code changes. In PR mode, retain the snapshot
   until the user explicitly agrees to cleanup; reports remain available.

The full prompt — including verification requirements, reporting, PR-mode restrictions,
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
described in [What the skill does](#what-the-skill-does), including
troubleshooting failed invocations and asking the user to configure
credentials locally when needed.

## Other agent frameworks

Any framework with a "register external skill" surface can ingest the
SKILL.md — it's just markdown with frontmatter. If your framework
expects a different schema, the markdown body is still useful as a
prompt template.

## See Also

- [Command（Claude Code Plugin）](../claude-code/) — the
  slash-command flavor of the same skill.
