---
status: accepted
---

# Expose OCR to Codex through a local stdio MCP server

Codex needs one callable `ocr_review` capability that waits for the local OCR process and returns its terminal result without session polling. We will add a local `stdio` MCP server to the Codex plugin and reuse the existing `ocr` CLI, because review execution depends on the current Git worktree and local OCR configuration; the separate OpenCode integration is not a Codex tool.

The tool returns OCR JSON only on a successful process exit. A non-zero exit, timeout, cancellation, or invalid JSON is a tool error with the exit reason and captured diagnostics, so Codex cannot mistake an execution failure for a completed review.

The input surface stays typed and small: the current worktree by default, or one commit or a `from`/`to` range, plus business context and exclusions. The server owns the worktree and rejects arbitrary CLI arguments.

An optional `resume` session ID is accepted only with a commit or ref range. When a resumable failure occurs, the tool exposes the session ID in its diagnostics so Codex can retry the same target; workspace reviews restart from the current worktree.

The watchdog treats completed LLM responses and persisted file checkpoints as activity, but keeps a configurable long `maxDuration` as the final safety limit. Activity can reset an idle timer; process liveness alone cannot disable the safety limit.

OCR will expose activity through machine-readable JSONL progress events on stderr. The MCP server reads that stream while the child process runs; stdout remains reserved for the final review JSON.

Progress events are intentionally liveness-only. They identify a completed LLM response or checkpoint and the affected path, without prompt text, response content, token counts, provider metadata, or credentials.

The idle limit is derived as `max(15 minutes, OCR_LLM_TIMEOUT + 5 minutes)` and resets on progress events. A separate four-hour `maxDuration` remains as the outer safety fuse; neither limit is used to declare normal completion.

The bundled MCP server sets Codex's `tool_timeout_sec` slightly above four hours, leaving room for the OCR process to stop and return its terminal timeout error after the four-hour `maxDuration`. The host timeout is a transport safety limit, not the review completion signal.

When a run fails with a reusable session, the MCP tool returns a structured resumable error containing the session ID. Codex performs one explicit follow-up call with the same target and `resume` value; the MCP server does not hide an automatic retry.

User cancellation follows the same checkpoint-preserving path: the child is stopped, completed checkpoints remain available, and the terminal error includes the session ID when one exists so a later explicit `resume` call can continue.

The server lives in the existing Go `ocr` binary as `ocr mcp serve`. The Codex plugin starts that command over local `stdio`, so the integration reuses the repository's OCR and session implementation without adding a second runtime.

The plugin resolves `ocr` from `PATH` and reports a clear setup error when it is missing; it does not bundle platform-specific binaries.

The plugin does not set a fixed server `cwd`. `ocr mcp serve` inherits the Codex task's working directory, resolves its Git root with `git rev-parse --show-toplevel`, and binds the server instance to that worktree. Startup fails clearly when the directory is not a Git worktree. Separate tasks and worktrees therefore use separate server instances without a `repo` or `worktree` tool argument.

The first MCP surface exposes only `ocr_review`; health checks and preview remain CLI workflows until the blocking review path is stable.

Concurrency is scoped to a server instance and its worktree: one active review per instance, while separate Codex tasks with separate worktrees and server processes may review in parallel. A second call on the same instance returns `busy` instead of being queued.
