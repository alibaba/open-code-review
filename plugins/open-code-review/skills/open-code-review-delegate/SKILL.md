---
name: open-code-review-delegate
description: >
  Delegation mode for open-code-review (OCR). Instead of OCR calling an LLM
  endpoint, this skill instructs the host agent to perform the code review
  itself, using OCR only for deterministic engineering: file selection and
  rule resolution. Use when the host agent should drive the review with its
  own LLM capabilities.
license: Apache-2.0
compatibility: >
  Requires the `ocr` CLI installed (via `npm install -g
  @alibaba-group/open-code-review` or GitHub release binary). Does NOT
  require a configured LLM endpoint — delegation mode is LLM-free on the
  OCR side.
metadata:
  author: alibaba
  homepage: https://github.com/alibaba/open-code-review
  version: "1.0.0"
---

# Open Code Review — Delegation Mode

This Codex plugin skill intentionally mirrors the canonical skill at
`skills/open-code-review-delegate/SKILL.md`. Keep both files synchronized when
updating OCR delegation instructions; a symlink is avoided because plugin
installs may only materialize the plugin subtree.

A skill for performing AI code review where OCR provides deterministic engineering (file filtering, rule resolution) and the host agent performs the actual review using its own intelligence and tools.

## Prerequisites

```bash
which ocr || echo "NOT INSTALLED"
```

If `ocr` is not installed:

```bash
npm install -g @alibaba-group/open-code-review
```

No LLM configuration is needed for delegation mode.

## Workflow

### Step 1: Preview — Determine What to Review

```bash
ocr delegate preview --format json [--from <ref> --to <ref>] [--commit <hash>] [--exclude <patterns>]
```

This outputs:
- **mode** (workspace / range / commit)
- **from / to / commit / merge_base** — ref metadata for constructing git commands
- **Reviewable file list** — paths, status, insertions/deletions
- **Excluded files** — with exclusion reason

**Common invocations:**

| Scenario | Command |
|----------|---------|
| Workspace changes | `ocr delegate preview` |
| Branch comparison | `ocr delegate preview --from main --to feature` |
| Single commit | `ocr delegate preview -c abc123` |

### Step 2: Get Rules for Files

```bash
ocr delegate rule --format json <path1> <path2> ...
```

Pass the reviewable file paths from Step 1. Output is grouped by rule content — files sharing the same rule appear under one group, avoiding repetition.

### Step 3: Get Diffs

Use git directly based on the mode/ref info from Step 1:

**Range mode** (merge_base provided in preview output):
```bash
git diff <merge_base>..<to> -- <path>
```

**Commit mode**:
```bash
git show <commit> -- <path>
```

**Workspace mode**:
```bash
# Tracked files
git diff HEAD -- <path>
# New untracked files — read directly (entire file is new code)
cat <path>
```

### Step 4: Review Each File

Create a checklist containing every `reviewable_files` entry. For each reviewable file:

Use `(path, status)` as the checklist identity. Workspace mode can report the same path twice when a staged deletion is followed by an untracked recreation.

1. Get its diff (Step 3)
2. Consult its Rule Group (from Step 2) for the review checklist
3. Conduct a thorough review, using appropriate context tools as needed
4. Mark the file `reviewed`, or `skipped` with a concrete reason

For large changes, review in bounded batches grouped by shared rules and diff size. Do not stop after finding the first high-severity issue.

### Step 5: Format Output

Each comment must follow this structure:

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| path | string | yes | Relative file path |
| content | string | yes | Review comment describing the issue |
| start_line | integer | no | Start line in the new file |
| end_line | integer | no | End line in the new file |
| category | enum | no | bug, security, performance, maintainability, test, style, documentation, other |
| severity | enum | no | critical, high, medium, low |

### Step 6: Classify and Report

Before reporting, verify that every previewed file is accounted for. Include `total_files`, `reviewed_files`, `skipped_files`, and `coverage_rate` in the summary. A skipped file must include its reason.

Group findings by severity:

- **Critical/High**: Bugs, security issues, data loss risks — always report
- **Medium**: Performance concerns, error handling gaps, maintainability issues — report with context
- **Low**: Style nits, minor suggestions — report only if clearly valuable

Discard likely false positives silently.

### Step 7: Fix (Optional)

If the user requested "review and fix":
- Apply High/Critical fixes directly
- Describe Medium fixes that require manual intervention
- Skip Low-priority items unless trivial

## Sub-commands Reference

| Command | Purpose |
|---------|---------|
| `ocr delegate preview` | Which files to review + mode/ref metadata |
| `ocr delegate rule <path...>` | Review rules grouped by content |

## Shared Flags

| Flag | Description |
|------|-------------|
| `--from <ref>` | Source ref for range mode |
| `--to <ref>` | Target ref for range mode |
| `-c, --commit <hash>` | Single commit mode |
| `--repo <path>` | Repository root (default: cwd) |
| `--rule <path>` | Custom rule.json path |
| `--exclude <patterns>` | Comma-separated exclude patterns |
| `-b, --background <text>` | Business context |
| `-B, --background-file <path>` | Business context from Markdown file (takes precedence over `-b`) |
| `-f, --format <text\|json>` | Output format; use `json` for agent integrations |

## Gotchas

- **No LLM needed on OCR side** — delegation mode never calls an LLM. All intelligence comes from the host agent.
- **Rules are grouped** — Files sharing the same rule are grouped together in the output. You can pass any number of paths per call; for large changes, fetch rules per-batch as you review.
- **Working directory matters** — `ocr delegate` operates on the Git repo at the current directory. Use `--repo /path` to override.
- **Untracked files in workspace mode** — `preview` includes untracked files. For these, read the file directly instead of using `git diff`.
- **Background context** — pass `--background` to `preview` when you have requirement context; it appears in the output for your reference during review.

### Recovering Oversized Background Context

`--background-file` has two independent limits. The raw file must not exceed
1 MiB, and the sanitized content must not exceed 8000 characters. Either
condition aborts the command. When the command reports either limit:

1. Do not silently truncate the source file.
2. Summarize the original material while preserving its requirements,
   constraints, acceptance criteria, and other review-critical details.
3. Retry the affected command by passing the summary as one shell-safe
   argument (for example, use a quoted/escaped argument produced by the host
   shell, or write it to a new size-bounded file and pass that file). Do not
   place untrusted summary text directly in a double-quoted shell template;
   `$()`, backticks, quotes, and variable references can still be evaluated.
   Omit the original `--background-file` so the CLI does not reload the same
   oversized file and fail again.
4. If a faithful summary is not possible, omit the OCR background entirely and
   read the original material directly during the review.

- **Coverage is mandatory** — every `reviewable_files` entry must end as reviewed or explicitly skipped; do not silently omit files.

### Troubleshooting CLI Version Compatibility

The `--format` flag is available in `ocr` v1.9.0 and later. The Skill and the
installed CLI can be updated independently. If a requested `preview` or `rule`
command with `--format json` fails specifically with `unknown flag: --format`,
rerun it without the flag and use text output for the rest of the delegation
run. Preserve the explicit mode, ref, file, and rule information from that
output; do not parse text output as JSON or invent missing schema fields. Do
not retry without the flag for any other error; report it and stop the affected
workflow.

The host-agent Skill may consume the equivalent text output to complete its
review checklist. Programmatic integrations that require `schema_version` or
other JSON fields must require a JSON-capable CLI instead: verify with
`ocr --version` and upgrade when necessary:

```bash
npm install -g @alibaba-group/open-code-review
```