---
name: open-code-review
description: Use Alibaba Open Code Review (`ocr`) from local Codex to review Git workspace changes, staged or unstaged changes, untracked files, commits, pull requests, or branch comparisons. Use when the user asks to review code, review a PR, compare branches, or review and optionally fix code review findings.
license: Apache-2.0
metadata:
  author: alibaba
  homepage: https://github.com/alibaba/open-code-review
  version: "1.0.0"
---

# Open Code Review for Codex

Use the local `ocr` CLI from the active Git workspace.

This skill integrates Open Code Review with local Codex by invoking the `ocr` command. It does not configure OpenAI Responses API, does not require a Codex API endpoint, and does not make Codex the internal LLM backend for OCR.

## Preconditions

Before running a review, check the environment:

```bash
command -v ocr
git rev-parse --is-inside-work-tree
```

If `ocr` is not installed, tell the user to install it:

```bash
npm install -g @alibaba-group/open-code-review
```

Do not install global packages unless the user explicitly asked to set up the tool.

Check OCR's LLM connectivity before the first real review:

```bash
ocr llm test
```

If this fails, report that OCR itself is not configured yet. Do not invent credentials, API keys, endpoint URLs, or model names.

## Review workflow

Run from the Git repository root unless the user provided a different repository path.

For current workspace changes:

```bash
ocr review --audience agent
```

For a single commit:

```bash
ocr review --audience agent --commit <sha>
```

For a branch comparison:

```bash
ocr review --audience agent --from <base-ref> --to <head-ref>
```

For preview mode without invoking the LLM:

```bash
ocr review --preview
```

When useful business or requirement context is available, pass it through `--background`:

```bash
ocr review --audience agent --background "<concise context>"
```

## Argument handling

- If the user provides `--commit` or `-c`, pass it through.
- If the user provides `--from` and `--to`, pass them through.
- If the user asks what would be reviewed, use `--preview`.
- Always use `--audience agent` for actual reviews so OCR emits agent-friendly output.
- Do not use `--audience human` because progress UI can pollute Codex output.

## Reporting

Classify OCR findings by priority:

- High: obvious bugs, security issues, clear correctness problems, or well-founded fixes.
- Medium: plausible issues that are context-dependent or require manual validation.
- Low: likely false positives, nitpicks, or weak suggestions.

Report High and Medium findings grouped by priority. Omit Low findings unless the user explicitly asks for all findings.

Use this format:

```markdown
## Code Review Results

**Files reviewed**: N
**Issues found**: X high priority / Y medium priority

### High Priority

- **`path/to/file.ext:42`** — Brief issue summary
  - Recommendation: Concrete fix

### Medium Priority

- **`path/to/file.ext:88`** — Brief issue summary
  - Recommendation: Concrete fix or manual validation step
```

If no actionable issues remain after filtering, say:

```text
Review complete — no actionable issues found.
```

## Fix policy

Do not modify files unless the user explicitly asks to fix issues.

If the user asks to review and fix:

1. Focus on High and Medium findings.
2. Apply only safe, localized fixes.
3. Re-run relevant tests or checks when available.
4. Summarize changed files and remaining risks.

Do not commit changes unless the user explicitly asks for a commit.
