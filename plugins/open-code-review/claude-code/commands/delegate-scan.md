---
description: Scan whole files with OCR in delegation mode — OCR plans the scan (file selection, batching, rules), you perform the review in this session.
---

Run OpenCodeReview (OCR) full-file **scan** in delegation mode. OCR decides which files to scan, how to batch them, and which rules apply; **you** perform the actual review in this session using your own capabilities. No LLM endpoint is configured on the OCR side and no API key is used.

Use this instead of `/delegate-review` when there is no meaningful diff to review — auditing an unfamiliar codebase, a directory, or a set of files.

## Step 1: Get the scan plan

```bash
ocr delegate scan [user-args]
```

- Default (no user arguments): the whole repository.
- If the user names a directory or files: pass `--path <comma-separated>`.
- (Optional) `--exclude '<patterns>'` to drop generated files or fixtures.
- (Optional) `--batch by-directory | by-language | none` to override grouping.
- (Optional) `--background "context"` or `-b` for business context.
- Add `--format json` if you want to parse the plan programmatically.
- If `ocr` is not found, install it: `npm i -g @alibaba-group/open-code-review`.

This single command outputs everything needed to start: the batches of files to scan, and the review rule groups that apply to them.

**Check the scale before proceeding.** If the plan reports a large `scannable_count` (say, more than ~40 files), tell the user the size and confirm the scope — or suggest narrowing with `--path` — before burning the session on it.

## Step 2: Scan each batch

A batch is the unit OCR designed for one isolated context. Review **one batch per sub-agent**, dispatching batches in parallel where practical.

For each batch:

1. Read every file in the batch in full. Unlike diff review, there is no diff — the whole file is the subject.
2. Use the Rule Group matching those files as the review checklist.
3. Explore surrounding context (callers, definitions, tests) as needed to judge whether a finding is real.
4. Report findings with precise `path` and line numbers.

Every file in every batch must end as either reviewed or explicitly skipped with a concrete reason. Do not silently drop files.

## Step 3: Report

Classify each issue by severity:

- **Critical/High**: Bugs, security issues, data loss risks. Always report, with a precise fix proposal.
- **Medium**: Performance concerns, error handling gaps, maintainability issues. Report with context.
- **Low**: Style nits. Discard silently unless clearly valuable.

Scanning whole files surfaces far more candidate issues than diff review, and most long-standing code is intentional. Favor precision: report a finding only when you can point at the concrete consequence. Pre-existing style that the codebase applies consistently is not a finding.

Close with a coverage summary: total files planned, reviewed, skipped (with reasons).

## Step 4: Fix (only if asked)

If the user asked to "scan and fix", apply High/Critical fixes that are safe and well-defined, and describe the rest. Otherwise report only — a scan touches code no one asked to change.
