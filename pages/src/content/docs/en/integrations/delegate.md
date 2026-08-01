---
title: Delegation Mode
sidebar:
  order: 5
---

OCR handles deterministic engineering (file selection, rule resolution)
while the host agent performs the actual code review using its own LLM
capabilities. No LLM endpoint is required on the OCR side.

## When to use delegation mode

Delegation mode is designed for subscription-based AI coding agents —
such as Claude Code, Codex, Cursor, Open Code, Qoder, etc. — where you
already have an LLM subscription bundled with the host agent. Instead
of configuring a separate model endpoint for OCR, you reuse the host
agent's existing subscription quota to perform the review.

Use delegation mode when:

1. Your AI coding agent runs on a subscription plan and you want to
   reuse that quota for code review — no extra API key or model
   configuration needed.
2. You want OCR only for its engineering scaffolding — file filtering,
   rule resolution, exclusion logic — while the host agent handles all
   LLM reasoning.
3. You're building a custom agent pipeline that needs structured inputs
   (file list + rules) for its own review step.

## Prerequisites

The `ocr` CLI must be installed:

```bash
which ocr || npm install -g @alibaba-group/open-code-review
```

No LLM configuration (`ocr config set …` or environment variables) is
needed — delegation mode never calls an LLM on the OCR side.

## Install the skill / command

### Claude Code — Command

```bash
mkdir -p .claude/commands
curl -o .claude/commands/delegate-review.md \
  https://raw.githubusercontent.com/alibaba/open-code-review/main/plugins/open-code-review/claude-code/commands/delegate-review.md
```

### Any agent — Skill

```bash
npx skills add alibaba/open-code-review --skill open-code-review-delegate
```

Or copy the manifest manually:

```bash
cp -R /path/to/open-code-review/skills/open-code-review-delegate ~/.claude/skills/
```

## Workflow

### Step 1: Preview — determine what to review

```bash
ocr delegate preview [--from <ref> --to <ref>] [--commit <hash>] [--exclude <patterns>]
```

Outputs:

- **mode** — workspace / range / commit
- **ref metadata** — from, to, commit, merge\_base
- **Reviewable file list** — paths, status, insertions/deletions
- **Excluded files** — with exclusion reason

Common invocations:

| Scenario | Command |
|----------|---------|
| Workspace changes | `ocr delegate preview` |
| Branch comparison | `ocr delegate preview --from main --to feature` |
| Single commit | `ocr delegate preview -c abc123` |

### Step 2: Get rules for files

```bash
ocr delegate rule <path1> <path2> ...
```

Pass the reviewable paths from Step 1. Output is grouped by rule
content — files sharing the same rule appear under one group, avoiding
repetition.

### Step 3: Get diffs

Use git directly, based on the mode/ref info from Step 1:

**Range mode** (merge\_base provided):
```bash
git diff <merge_base>..<to> -- <path>
```

**Commit mode**:
```bash
git show <commit> -- <path>
```

**Workspace mode**:
```bash
git diff HEAD -- <path>        # tracked files
cat <path>                     # new untracked files
```

### Step 4: Review each file

For each reviewable file:

1. Get its diff (Step 3)
2. Consult the matching Rule Group (Step 2) as the review checklist
3. Conduct a thorough review, using context exploration as needed

### Step 5: Report

Classify each finding by severity:

- **Critical/High** — bugs, security issues, data loss risks. Always report.
- **Medium** — performance concerns, error handling gaps. Report with context.
- **Low** — style nits, minor suggestions. Discard silently unless clearly valuable.

## Sub-commands reference

| Command | Purpose |
|---------|---------|
| `ocr delegate preview` | List reviewable files + mode/ref metadata |
| `ocr delegate rule <path...>` | Resolve review rules grouped by content |

## Shared flags

| Flag | Description |
|------|-------------|
| `--from <ref>` | Source ref for range mode |
| `--to <ref>` | Target ref for range mode |
| `-c, --commit <hash>` | Single commit mode |
| `--repo <path>` | Repository root (default: cwd) |
| `--rule <path>` | Custom rule.json path |
| `--exclude <patterns>` | Comma-separated exclude patterns |
| `-b, --background <text>` | Business context |
| `-B, --background-file <path>` | Business context from Markdown file |

## Comparison with other integration modes

| Mode | Who calls the LLM? | Use case |
|------|-------------------|----------|
| [Agent Skill](../agent-skill/) | OCR | Agent invokes `ocr review`; OCR drives the full review |
| [Command (Claude Code)](../claude-code/) | OCR | Slash command in Claude Code; OCR drives the review |
| **Delegation Mode** | Host agent | OCR provides scaffolding; agent drives the review |

## See Also

- [Agent Skill](../agent-skill/) — OCR drives the full review on behalf of the agent.
- [Command (Claude Code)](../claude-code/) — slash-command flavor with auto-fix.
