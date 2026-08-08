---
status: accepted
---

> The request-period idle-watchdog behavior and idle-duration derivation in this ADR are superseded by [ADR 0007](0007-pause-idle-watchdog-during-llm-requests.md). The fixed whole-review deadline was later removed by [ADR 0004](0004-unlimited-default-aggregate-token-budget.md); the MCP transport and result-contract decisions remain accepted.

# Expose OCR to Codex through a local stdio MCP server

Codex needs one callable `ocr_review` capability that waits for the local OCR process and returns its terminal result without session polling. We will add a local `stdio` MCP server to the Codex plugin and reuse the existing `ocr` CLI, because review execution depends on the current Git worktree and local OCR configuration; the separate OpenCode integration is not a Codex tool.

The execution-lifecycle and provider-request retry decisions are refined by [ADR 0008](0008-detached-review-recovery.md).

The tool returns OCR JSON only when the in-process review runner completes successfully. A runner error, timeout, cancellation, persistence failure, or invalid JSON is a tool error with a stable error type and captured diagnostics, so Codex cannot mistake an execution failure for a completed review.

The input surface stays typed and small: the current worktree by default, or one commit or a `from`/`to` range, plus business context and exclusions. The server owns the worktree and rejects arbitrary CLI arguments.

An optional `resume` session ID is accepted only with a commit or ref range. When a resumable failure occurs, the tool exposes the session ID in its diagnostics so Codex can retry the same target; workspace reviews restart from the current worktree.

The watchdog treats completed LLM responses and persisted file checkpoints as activity. There is no fixed whole-review deadline by default; per-file rounds, per-file timeouts, provider request limits, the idle watchdog, and explicit cancellation remain active. Activity can reset the idle timer; process liveness alone cannot disable the idle safety limit.

OCR exposes activity through machine-readable JSONL progress events on stderr while stdout remains reserved for the final review JSON. The MCP adapter also records the latest stage, path, and progress timestamp for terminal failure diagnostics.

Progress events are intentionally liveness-only. They identify a completed LLM response or checkpoint and the affected path, without prompt text, response content, token counts, provider metadata, or credentials.

The idle limit uses the MCP base duration and resets on meaningful progress events. It applies during non-LLM gaps and is paused while an LLM request is in flight; it does not declare normal completion.

Any host transport timeout is a transport safety limit, not the review completion signal. A live MCP server can retain the review for `ocr_review_wait`; a terminated server leaves persisted commit/ref-range checkpoints for `resume`.

When a run fails with a reusable session, the MCP tool returns a structured resumable error containing the session ID. Codex performs one explicit follow-up call with the same target and `resume` value; the MCP server does not hide an automatic retry.

An explicit user cancellation follows the checkpoint-preserving path. For commit/ref-range reviews, a successfully persisted session can provide a resumable session ID; workspace reviews and persistence failures are not resumable. A host transport interruption is handled as a Detached review while the server process remains alive. If the process ends, commit/ref-range recovery uses the persisted session and the same target with `resume`.

The server lives in the existing Go `ocr` binary as `ocr mcp serve`. The Codex plugin starts that command over local `stdio`; the server invokes the repository's review and session implementation in-process without adding a second runtime.

The plugin resolves `ocr` from `PATH` and reports a clear setup error when it is missing; it does not bundle platform-specific binaries.

The plugin does not set a fixed server `cwd`. `ocr mcp serve` inherits the Codex task's working directory, resolves its Git root with `git rev-parse --show-toplevel`, and binds the server instance to that worktree. Startup fails clearly when the directory is not a Git worktree. Separate tasks and worktrees therefore use separate server instances without a `repo` or `worktree` tool argument.

The MCP surface exposes `ocr_review`, the recovery-only `ocr_review_wait`, and explicit `ocr_review_cancel`. The wait tool only attaches to the current or most recent in-process review and returns its terminal result; it does not start, cancel, or poll a review. User cancellation uses the explicit cancel operation; health checks and preview remain CLI workflows.

Concurrency is scoped to a server instance and its worktree: one active review per instance, while separate Codex tasks with separate worktrees and server processes may review in parallel. A second `ocr_review` call returns `busy`; `ocr_review_wait` can attach to that existing execution without queueing a second review.
