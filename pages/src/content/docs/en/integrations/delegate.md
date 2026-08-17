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

For full-file scanning, install the scan command as well:

```bash
curl -o .claude/commands/delegate-scan.md \
  https://raw.githubusercontent.com/alibaba/open-code-review/main/plugins/open-code-review/claude-code/commands/delegate-scan.md
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

## Full-file scan (no diff)

Steps 1–4 above review a diff. When there is no meaningful diff — auditing an
unfamiliar codebase, a directory, or a set of files — use `ocr delegate scan`
instead. It is the delegation counterpart of `ocr scan`, and it replaces the
preview + rule pair with a single call:

```bash
ocr delegate scan [--path <dirs-or-files>] [--exclude <patterns>] [--batch <strategy>]
```

Outputs:

- **batches** — files grouped exactly as `ocr scan` would dispatch them, each
  with a batch id, its grouping key, and per-file line counts
- **excluded files** — with exclusion reason
- **rule groups** — the resolved review rules (omit with `--no-rules`)

Unlike `delegate preview`, this works outside a Git repository: a full-file
scan needs no refs.

### Workflow

1. **Run the plan.** Check `scannable_count` before proceeding — narrow with
   `--path` if the scan is larger than intended.
2. **Review one batch per sub-agent.** The batch is the unit OCR sized for a
   single isolated context; dispatch batches in parallel where practical.
3. **Read each file in full.** There is no diff — the whole file is the
   subject.
4. **Apply the matching rule group** as the review checklist, exploring
   callers, definitions and tests to confirm each finding is real.
5. **Report** using the same severity classification as Step 5 above, and
   close with a coverage summary.

Scanning whole files surfaces far more candidates than diff review, and
long-standing code is usually intentional. Favor precision: report a finding
only when you can name its concrete consequence.

### Scan-only flags

| Flag | Description |
|------|-------------|
| `--path <paths>` | Comma-separated directories or files to scan (default: whole repo) |
| `--batch <strategy>` | Override grouping: `none`, `by-language`, `by-directory` |
| `--batch-size <n>` | Max files per batch (0 = template default) |
| `--no-rules` | Omit resolved rules from the plan |

## Sub-commands reference

| Command | Purpose |
|---------|---------|
| `ocr delegate preview` | List reviewable files + mode/ref metadata |
| `ocr delegate rule <path...>` | Resolve review rules grouped by content |
| `ocr delegate scan` | Full-file scan plan: batches + rules, no diff needed |

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
