---
title: GitHub Copilot CLI
sidebar:
  order: 3
---

Run OCR end-to-end inside
[GitHub Copilot CLI](https://docs.github.com/en/copilot/concepts/agents/about-copilot-cli)
through the same portable [Agent Skill](../agent-skill/) used by other
coding agents: review the diff, classify findings, and fix the ones
worth adopting. The integration is a copy-and-go skill — it installs no
hooks, adds no required instructions, and changes no repository
behavior.

## What ships in the repo

The workflow lives in the agent-agnostic skill at
[`skills/open-code-review/SKILL.md`](https://github.com/alibaba/open-code-review/blob/main/skills/open-code-review/SKILL.md).
A Copilot CLI plugin manifest at
[`plugins/open-code-review/plugin.json`](https://github.com/alibaba/open-code-review/blob/main/plugins/open-code-review/plugin.json)
(Agent Plugins 1.0) packages the bundled skills for `copilot plugin install`.
Copilot CLI still reads skills from standard locations, so copying the skill
directory also works.

## Install

Install the plugin (recommended):

```bash
copilot plugin install alibaba/open-code-review:plugins/open-code-review
```

This installs the `open-code-review` and `open-code-review-delegate` skills.
Remove it with `copilot plugin uninstall open-code-review`.

Copilot CLI's installer currently warns that direct plugin installs
(repo/path/URL) are deprecated in favor of marketplace installs; a marketplace
entry may make `plugin@open-code-review` the recommended form later.

### Manual alternative: copy the skill

Copy the skill into a directory Copilot CLI discovers. **Project-level**
(commit it so the team shares it):

```bash
mkdir -p .github/skills/open-code-review
curl -o .github/skills/open-code-review/SKILL.md \
  https://raw.githubusercontent.com/alibaba/open-code-review/main/skills/open-code-review/SKILL.md
```

**User-level** (available in every project on the machine):

```bash
mkdir -p ~/.copilot/skills/open-code-review
curl -o ~/.copilot/skills/open-code-review/SKILL.md \
  https://raw.githubusercontent.com/alibaba/open-code-review/main/skills/open-code-review/SKILL.md
```

Copilot CLI also discovers `.agents/skills/` and `.claude/skills/` in
the project; project skills win over personal ones with the same name.
Check what is loaded with `/skills list` inside a session (or
`copilot skill list` outside one). Manually copied skills take effect
after `/skills reload` or a new session; installed plugins are updated by
reinstalling (`copilot plugin install` again) or `copilot plugin update`.

## Use

Mention the skill explicitly with `/open-code-review`, or just describe
the task — Copilot picks the skill from its description:

```
/open-code-review
/open-code-review review this PR against main
/open-code-review focus on race conditions in commit abc123
```

| You say | What the skill runs |
| --- | --- |
| "review my changes" | `ocr review --audience agent` (staged, unstaged, untracked) |
| "review this PR" | `ocr review --from main --to <branch>` |
| "review commit abc123" | `ocr review --commit abc123` |

The skill drives `ocr` with its own configured model and reports
findings grouped by severity. If you would rather not configure a
separate LLM for `ocr`, try [Delegation Mode](../delegate/) — the host
agent (Copilot) supplies the model and `ocr` only selects files and
resolves rules.

### Optional: make review a habit

To make the review step automatic instead of on-demand, add one rule to
`.github/copilot-instructions.md` or `AGENTS.md` — pick a single file,
because Copilot combines instruction files without a precedence order:

```markdown
Before reporting a coding task complete, run `ocr review --audience agent`.
Fix all `critical` and `high` findings first.
```

Whether to install such a rule is up to each project; this repository
does not impose one.

## Headless runs

```bash
copilot -p "Review the current branch with /open-code-review and report the findings." \
  --allow-tool='shell(ocr:*)'
```

- `--allow-tool='shell(ocr:*)'` pre-approves `ocr` shell commands only — it
  does not authorize file edits. In non-interactive (`-p`) sessions, actions
  that would need approval are denied rather than paused; interactive
  sessions pause for approval instead. To have Copilot apply fixes, run
  interactively (or deliberately extend the allowlist to editing tools).
- Project-level skills (`.github/skills/`) may be skipped in non-interactive
  sessions in untrusted folders; Copilot CLI v1.0.107+ offers
  `--trust-folder` to trust the folder for a single session.
- With the Copilot sandbox enabled, `ocr` needs network access to your
  LLM endpoint and both read and write access to `~/.opencodereview/`
  (it stores session state).
- `--output-format json` emits Copilot's JSONL session events; the review
  result itself is `ocr`'s JSON, obtained by explicitly running
  `ocr review --format json --audience agent`.

For deterministic CI gates, skip the agent and invoke `ocr` directly —
see [CI/CD](../cicd/).

## Notes

- **No hooks, on purpose.** Copilot CLI can gate `git commit` or
  `git push` with `preToolUse` hooks, but OCR does not ship one: the
  review runs an LLM (slow, and it needs credentials a contributor may
  not have), the workspace it reviews is not the commit being made, and
  a hook that fails open advertises enforcement it cannot guarantee.
  Add your own hook only if you accept those trade-offs.
- **Prompt files are IDE-only.** Copilot CLI does not read
  `.github/prompts/*.prompt.md`; the skill is the supported surface.
- **Findings do not change exit codes.** `ocr` exits 0 on a completed
  review even when it found problems — gate on severities from
  `--format json`, never on exit status.

## Troubleshooting

- **Skill not listed** — confirm the path (`~/.copilot/skills/` or
  `<repo>/.github/skills/`), then run `/skills reload` or start a new
  session.
- **`ocr: command not found`** — install with
  `npm i -g @alibaba-group/open-code-review`.
- **Review fails with an LLM error** — configure a provider and test it
  with `ocr llm test`; see [Configuration](../configuration/).
- **Copilot keeps asking to approve `ocr`** — allow
  `shell(ocr:*)` via `--allow-tool` or approve it permanently at the
  prompt.
