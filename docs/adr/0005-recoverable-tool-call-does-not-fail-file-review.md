---
status: accepted
---

# Recoverable tool-call failures do not fail file reviews

## Context

An LLM can call `file_read` with an incomplete or incorrect path, such as a
path without the `.md` suffix. The tool call can fail even though the LLM can
recover by locating the exact path and completing the file review.

## Decision

`file_read` returns the fixed error:

`file not found: "<exact path>". Use file_find to locate the exact path, then retry file_read.`

The LLM loop receives this error and decides whether to call `file_find` and
retry. The runtime does not guess suffixes, silently rewrite paths, or perform
a hidden retry. Every recoverable failed tool call is retained in session
history and JSONL, while a later `task_done DONE` marks the file completed.
Only an explicit `task_done FAILED` or an incomplete loop marks the file
failed.

## Consequences

- Intermediate tool-call failures remain visible for diagnosis without making
  the coverage manifest partial when the conversation completes.
- The existing per-file tool-request budget limits recovery attempts.
- The agent dispatch integration test covers the failure, `file_find`, corrected
  `file_read`, `task_done DONE`, complete coverage, and persisted failed call.
