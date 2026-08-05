---
status: accepted
---

# Expose OCR to Codex through a local stdio MCP server

Codex needs one callable `ocr_review` capability that waits for the local OCR process and returns its terminal result without session polling. We will add a local `stdio` MCP server to the Codex plugin and reuse the existing `ocr` CLI, because review execution depends on the current Git worktree and local OCR configuration; the separate OpenCode integration is not a Codex tool.

The tool returns OCR JSON only when the in-process review runner completes successfully. A runner error, timeout, cancellation, persistence failure, or invalid JSON is a tool error with a stable error type and captured diagnostics, so Codex cannot mistake an execution failure for a completed review.

The input surface stays typed and small: the current worktree by default, or one commit or a `from`/`to` range, plus business context and exclusions. The server owns the worktree and rejects arbitrary CLI arguments.

An optional `resume` session ID is accepted only with a commit or ref range. When a resumable failure occurs, the tool exposes the session ID in its diagnostics so Codex can retry the same target; workspace reviews restart from the current worktree.

The watchdog treats completed LLM responses and persisted file checkpoints as activity, while the MCP server owns a fixed 60-minute review deadline. Activity can reset an idle timer; process liveness alone cannot disable the safety limit.

OCR exposes activity through machine-readable JSONL progress events on stderr while stdout remains reserved for the final review JSON. The MCP adapter also records the latest stage, path, and progress timestamp for terminal failure diagnostics.

Progress events are intentionally liveness-only. They identify a completed LLM response or checkpoint and the affected path, without prompt text, response content, token counts, provider metadata, or credentials.

The idle limit is derived as `max(15 minutes, OCR_LLM_TIMEOUT + 5 minutes)` and resets on meaningful progress events. The 60-minute review deadline remains the outer safety fuse; neither limit is used to declare normal completion.

The bundled MCP server sets Codex's `tool_timeout_sec` to 61 minutes, leaving one minute for the OCR process to stop and return its terminal timeout error after the 60-minute review deadline. The host timeout is a transport safety limit, not the review completion signal.

When a run fails with a reusable session, the MCP tool returns a structured resumable error containing the session ID. Codex performs one explicit follow-up call with the same target and `resume` value; the MCP server does not hide an automatic retry.

User cancellation follows the same checkpoint-preserving path: the child is stopped, completed checkpoints remain available, and the terminal error includes the session ID when one exists so a later explicit `resume` call can continue.

The server lives in the existing Go `ocr` binary as `ocr mcp serve`. The Codex plugin starts that command over local `stdio`; the server invokes the repository's review and session implementation in-process without adding a second runtime.

The plugin resolves `ocr` from `PATH` and reports a clear setup error when it is missing; it does not bundle platform-specific binaries.

The plugin does not set a fixed server `cwd`. `ocr mcp serve` inherits the Codex task's working directory, resolves its Git root with `git rev-parse --show-toplevel`, and binds the server instance to that worktree. Startup fails clearly when the directory is not a Git worktree. Separate tasks and worktrees therefore use separate server instances without a `repo` or `worktree` tool argument.

The MCP surface exposes `ocr_review` and the recovery-only `ocr_review_wait`. The wait tool only attaches to the current or most recent in-process review and returns its terminal result; it does not start, cancel, or poll a review. User cancellation is a host-side interruption propagated through the review context; health checks and preview remain CLI workflows.

Concurrency is scoped to a server instance and its worktree: one active review per instance, while separate Codex tasks with separate worktrees and server processes may review in parallel. A second `ocr_review` call returns `busy`; `ocr_review_wait` can attach to that existing execution without queueing a second review.
