---
name: open-code-review-delegate
description: >
  Review workspace changes, PRs, commits, or branch comparisons with the
  host agent's model. OCR's CLI selects files and resolves rules; the host
  reviews them for defects and explains changes that affect users or callers.
  Use when the user has no separate API key for OCR or wants to reuse their
  coding-agent subscription or model setup. For OCR's full native pipeline
  with its own model provider, choose open-code-review instead.
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

## Workflow

### Step 1: Preview — Determine What to Review

Skip OCR provider setup and `ocr llm test`; the host agent's authentication, usage limits, and costs still apply.

```bash
ocr delegate preview --format json [--from <ref> [--to <ref>]] [--commit <hash>] [--exclude <patterns>]
```

With `--from` and no `--to`, both delegate subcommands default to `HEAD`. Range review compares the merge-base to the target commit and excludes staged, unstaged, and untracked changes. Omit range/commit flags for workspace mode; range and commit flags are mutually exclusive.

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
| Current branch against main | `ocr delegate preview --from main` |
| Single commit | `ocr delegate preview -c abc123` |

### Step 2: Get Rules for Files

```bash
ocr delegate rule --format json [--from <ref> [--to <ref>]] [--commit <hash>] <path1> <path2> ...
```

Pass the reviewable file paths from Step 1. Output is grouped by rule content — files sharing the same rule appear under one group, avoiding repetition.

Pass the same range or commit flags as in Step 1 so rules are resolved against the same target. For example, pair `ocr delegate preview --from main` with `ocr delegate rule --from main --format json <path...>`.

### Step 3: Get Diffs

Choose the diff command using the mode and refs from Step 1. Use `git --no-pager` for every Git call during the review.

The diff commands disable external diff programs, text conversion, and color to produce plain patches.

**Range mode** (merge_base provided in preview output):
```bash
git --no-pager diff --no-ext-diff --no-textconv --no-color <merge_base>..<to> -- "<path>"
```

**Commit mode**:
```bash
git --no-pager show --no-ext-diff --no-textconv --no-color <commit> -- "<path>"
```

**Workspace mode**:
```bash
# Tracked files
git --no-pager diff --no-ext-diff --no-textconv --no-color HEAD -- "<path>"
# New untracked files — read directly (entire file is new code)
cat "<path>"
```

To read context:

```bash
git --no-pager log --no-color --oneline -- "<path>"
git --no-pager blame --no-textconv -- "<path>"
git --no-pager show --no-ext-diff --no-textconv --no-color "<ref>:<path>"
```

For large diffs, add `--output="<absolute-diff-file>"` to the command for the selected mode. Choose a unique absolute path outside the repository and create its parent directory. Use the same path in file-reading tool calls:

```bash
git --no-pager diff --no-ext-diff --no-textconv --no-color --output="<absolute-diff-file>" <merge_base>..<to> -- "<path>"
```

After Git exits with code 0, read and review the entire file in chunks, then remove it. If Git fails or times out, retry or record `skipped` with the error. Check unexpectedly empty output against a fresh preview.

### Step 4: Review Each File

Create a checklist containing every `reviewable_files` entry. For each reviewable file:

Use `(path, status)` as the checklist identity. Workspace mode can report the same path twice when a staged deletion is followed by an untracked recreation.

1. Get its diff (Step 3)
2. Consult its Rule Group (from Step 2) for the review checklist
3. Conduct a thorough review, using appropriate context tools as needed
4. Mark the file `reviewed`, or `skipped` with a concrete reason

For large changes, review in bounded batches grouped by shared rules and diff size. Do not stop after finding the first high-severity issue.

While reading each diff, ask which existing caller or user workflow, if any, behaves differently. Reuse the same evidence; follow a caller, contract, or test only when it helps establish a concrete before/after scenario. Compare the sides selected in Step 3 (the new side in workspace mode is the live working tree). Assess behavior changes as you review the diff. Do not run `ocr review --analysis` or pass `--analysis` to `ocr delegate`.

A removed interface may matter even when there are no new lines to review. If preview excludes a file solely as `deleted`, use its old side as context when needed, without counting it as reviewed. Respect all other exclusions when following evidence.

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

Identify the review mode as delegation.

Before reporting, verify that every previewed file is accounted for. Include `total_files`, `reviewed_files`, `skipped_files`, and `coverage_rate` in the summary. A skipped file must include its reason.

Group findings by severity:

- **Critical/High**: Bugs, security issues, data loss risks — always report
- **Medium**: Performance concerns, error handling gaps, maintainability issues — report with context
- **Low**: Style nits, minor suggestions — report only if clearly valuable

Discard likely false positives silently.

Report a material behavior change even if it is intentional and there are no defects. Name the affected workflow, what happened before and what happens now, and cite the supporting location. For example, removing a public option can break scripts that still pass it. Keep this impact distinct from severity-rated findings and file coverage; if it explains a finding, describe it there only once. Qualify material uncertainties, and omit the impact section when there is no concrete change to report.

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
| `--to <ref>` | Target ref for range mode; defaults to `HEAD` when `--from` is supplied in either subcommand |
| `-c, --commit <hash>` | Single commit mode |
| `--repo <path>` | Repository root (default: cwd) |
| `--rule <path>` | Custom rule.json path |
| `--exclude <patterns>` | Comma-separated exclude patterns |
| `-b, --background <text>` | Business context |
| `-B, --background-file <path>` | Business context from Markdown file (takes precedence over `-b`) |
| `-f, --format <text\|json>` | Output format; use `json` for agent integrations |

## Gotchas

- **Host-driven analysis** — OCR's native review agent, positioning, and reflection modules do not run in delegation mode.
- **Rules are grouped** — Files sharing the same rule are grouped together in the output. You can pass any number of paths per call; for large changes, fetch rules per-batch as you review.
- **Working directory matters** — `ocr delegate` operates on the Git repo at the current directory. Use `--repo /path` to override.
- **Untracked files in workspace mode** — `preview` includes untracked files. For these, read the file directly instead of using `git diff`.
- **Background context** — pass `--background` to `preview` when you have requirement context; it appears in the output for your reference during review.
- **Coverage is mandatory** — every `reviewable_files` entry must end as reviewed or explicitly skipped; do not silently omit files.

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

### Troubleshooting CLI Version Compatibility

If an OCR command fails with `command not found`, install the CLI with `npm install -g @alibaba-group/open-code-review` if your environment permits, then retry the same command. If installation is not permitted, explain the prerequisite and stop; do not substitute `ocr review` or ask for an API key.

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
