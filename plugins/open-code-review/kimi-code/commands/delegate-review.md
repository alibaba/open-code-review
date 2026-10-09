---
description: Run OCR in delegation mode — OCR handles file selection and rules, the host agent performs the actual review.
---

Invoke OpenCodeReview (OCR) in delegation mode. OCR determines which files to review and provides review rules; you perform the actual code review using your own capabilities.

## Workflow

### Step 1: Preview

```bash
ocr delegate preview [user-args]
```

- Default (no user arguments): workspace mode (staged + unstaged + untracked).
- If the user provides `--commit` or `-c`: pass through as-is.
- If the user provides `--from` and `--to`: pass through as-is.
- (Optional) Provide `--background "context"` or `-b "context"` for business context.
- If `ocr` is not found, install it: `npm i -g @alibaba-group/open-code-review`.

This outputs mode/ref metadata and the reviewable file list.

### Step 2: Get Rules

Pass all reviewable file paths to get their review checklists:

```bash
ocr delegate rule <path1> <path2> ...
```

### Step 3: Get Diffs and Review

For each reviewable file, get its diff using git (based on mode/ref from Step 1):
- Range: `git --no-pager diff --no-ext-diff --no-textconv --no-color <merge_base>..<to> -- "<path>"`
- Commit: `git --no-pager show --no-ext-diff --no-textconv --no-color <commit> -- "<path>"`
- Workspace: `git --no-pager diff --no-ext-diff --no-textconv --no-color HEAD -- "<path>"` (or read directly for untracked files)

Keep all four flags to prevent interactive pagers and preserve plain unified patches. For large diffs, choose a unique absolute temporary file path outside the repository, substitute it literally below, and use that same path in subsequent file-reading tool calls. Git's `--output` works in Bash and PowerShell without shell variables or redirection:

```bash
git --no-pager diff --no-ext-diff --no-textconv --no-color --output="<absolute-diff-file>" <merge_base>..<to> -- "<path>"
```

Add `--output="<absolute-diff-file>"` to the commit or workspace command when needed. Require exit code 0 before reading the result. On a nonzero exit or timeout, retry or mark the file `skipped` with the error; never mark it `reviewed` or infer "no changes" from an empty or partial file. If successful output is unexpectedly empty, reconcile it with a fresh preview. Read every chunk through the end before marking the file `reviewed`, then remove the temporary file.

Account for every previewed `(path, status)` entry as `reviewed` or explicitly `skipped` with a reason. Include `total_files`, `reviewed_files`, `skipped_files`, and `coverage_rate` in the report.

Then review focusing on: correctness, security, performance, error handling, concurrency, maintainability. Only comment on changed code (+ lines).

### Step 4: Report and Fix

Classify each issue by severity:

- **High**: Obvious bugs, security issues, data loss, or clear mistakes with precise fix proposals
- **Medium**: Reasonable concerns, performance suggestions, or fixes requiring manual work
- **Low**: Discard silently (likely false positives, nitpicks, or insufficient context)

Automatically fix High and Medium issues that are safe and well-defined.
