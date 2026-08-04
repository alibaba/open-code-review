---
name: code-review
description: Review changes since a fixed point with the synchronous OpenCodeReview (OCR) MCP tool. Use when the user wants to review a branch, PR, commit, or work-in-progress changes.
---

Review the diff between `HEAD` and a fixed point supplied by the user with one blocking `ocr_review` MCP call from `ocr-mcp-server`. The call returns only after OCR reaches a terminal result. Keep the fixed point and any available task, repository, or business context as inputs, then return OCR's native review output without imposing an additional report structure or claiming coverage beyond OCR's result.

Do not run `ocr review`, spawn review sub-agents, inspect progress events, or poll for completion. If the MCP tool is unavailable, report the integration error instead of falling back to the CLI.

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

### 3. Call the synchronous MCP tool

Call the `ocr_review` tool exposed by `ocr-mcp-server` exactly once for the validated range. MCP input is typed JSON; use `from` and `to` for the range, and include `background` or `exclude` only when they are available:

```json
{
  "from": "<fixed-point>",
  "to": "HEAD",
  "background": "<concise task requirements, repository, and business context>",
  "exclude": ["<optional glob>"]
}
```

When reviewing a single commit rather than a range, use `commit` instead of `from`/`to`. The MCP server resolves the current Git worktree; do not pass a `repo` or `worktree` argument.

The MCP call is synchronous: wait for its returned result in the same call. Do not retry automatically or issue a status/session-polling call. If a failed commit/range review returns a resumable session ID and the user explicitly requests resume, make one explicit follow-up `ocr_review` call with the same target and `resume` value.

### 4. Return the OCR result

Return OCR's native result with its own `status`, `summary`, `comments`, `warnings`, and optional session or manifest fields. Preserve each finding's path, line range, category, severity, content, and suggestion when present.

When handing findings back to `/implement`, carry the original task source path or task-context summary alongside the OCR result. The OCR result alone is not a substitute for the original task requirements.

Do not add a second report structure or assert coverage beyond OCR's result. If the user requests fixes, apply only the requested or explicitly approved fixes, then verify them with a fresh review or the relevant tests.

## Validation

After OCR finishes, verify:

1. The MCP call returned a terminal result rather than an intermediate progress event.
2. The result contains comments, or explicitly reports that no comments were generated.
3. Any warnings, partial coverage, failure reason, or resumable session ID are preserved in the handoff.
