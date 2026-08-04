---
name: code-review
description: Review changes since a fixed point with OpenCodeReview (OCR). Use when the user wants to review a branch, PR, commit, or work-in-progress changes.
---

Review the diff between `HEAD` and a fixed point supplied by the user with one OpenCodeReview run. Keep the fixed point and any available task, repository, or business context as inputs, then return OCR's native review output without imposing an additional report structure or claiming coverage beyond OCR's result.

The issue tracker should have been provided to you — run `/setup-matt-pocock-skills` if `docs/agents/issue-tracker.md` is missing.

## Process

### 1. Pin the fixed point

Whatever the user said is the fixed point — a commit SHA, branch name, tag, `main`, `HEAD~5`, etc. If they did not specify one, ask for it.

Confirm the ref and diff before running OCR:

```bash
git rev-parse <fixed-point>
if git diff --quiet <fixed-point>...HEAD; then
  echo "empty review"
  exit 1
fi
git log <fixed-point>..HEAD --oneline
```

The fixed point must resolve and the diff must be non-empty. The `git diff --quiet` check returns success only for an empty diff; a non-zero result is expected when there is work to review. Fail only for a bad ref or an empty review.

### 2. Identify the task source

Look for the originating task requirements, in this order:

1. Issue references in the commit messages (`#123`, `Closes #45`, GitLab `!67`, etc.) — fetch via the workflow in `docs/agents/issue-tracker.md`.
2. A path the user passed as an argument.
3. A PRD or task document under `docs/`, `specs/`, or `.scratch/` matching the branch name or feature.
4. Context supplied by the user or still available in the current conversation.
5. If nothing is found, ask the user where the task requirements are. If they say there is none, omit the background options and continue without inventing requirements.

If a task source is found, summarize its requirements and only the repository or business context relevant to the changed code, then pass that context to OCR in step 3. Use `--background-file` for a suitable Markdown document when that is shorter and more accurate.

### 3. Run one OCR review

Run exactly one review for the validated range. Always use `--audience agent`; use `--format json` when the result will be consumed by another agent or tool.

```bash
ocr review --audience agent --format json \
  --from <fixed-point> --to HEAD \
  --background "<concise task requirements, repository, and business context>"
```

When no task context is available, omit the final `--background` line. Use `--background-file <path>` instead when the task source is a suitable Markdown document.

Do not spawn review sub-agents, run a second review, or rerank OCR findings.

### 4. Return the OCR result

Return OCR's native result with its own `status`, `summary`, `comments`, `warnings`, and optional session or manifest fields. Preserve each finding's path, line range, category, severity, content, and suggestion when present.

When handing findings back to `/implement`, carry the original task source path or task-context summary alongside the OCR result. The OCR result alone is not a substitute for the original task requirements.

Do not add a second report structure or assert coverage beyond OCR's result. If the user requests fixes, apply only the requested or explicitly approved fixes, then verify them with a fresh review or the relevant tests.

## Validation

After OCR finishes, verify:

1. The command exited successfully.
2. The result is valid JSON when `--format json` was used.
3. The result contains comments, or explicitly reports that no comments were generated.
4. Any warnings or partial coverage are preserved in the handoff.
