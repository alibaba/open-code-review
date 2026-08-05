# Open Code Review Agent Integration

This context defines how the local Open Code Review CLI is exposed to coding-agent hosts.

## Language

**OCR runner**:
The local `ocr` CLI process that analyzes Git changes and emits structured review findings.
_Avoid_: using `ocr_review` to mean the CLI process.

**Codex review tools**:
The local `stdio` MCP server exposes `ocr_review` for one terminal review result.
_Avoid_: session polling, OpenCode tool.

**OpenCode integration**:
The separate plugin under `plugins/open-code-review/opencode/` that registers tools for OpenCode. It does not register tools in Codex.
_Avoid_: treating the OpenCode integration as the Codex integration.

**Review result**:
The structured JSON returned after the OCR runner finishes, fails, is cancelled, or reaches its timeout.
_Avoid_: treating an intermediate progress update as a review result.

**Synchronous review**:
A review request that stays open until OCR returns a terminal Review result, including success, failure, cancellation, or timeout.
_Avoid_: background review, progress event as result.

**Review deadline**:
The server-controlled point by which a Synchronous review must return a terminal Review result; the host timeout follows it with a short grace period.
_Avoid_: host timeout as the review result.

**Resumable failure**:
A terminal Review result showing that OCR stopped before completion while a Resume session may continue from completed checkpoints.
_Avoid_: partial success, retry without a Resume session.

**Review stage**:
The named phase and latest affected path reported with a Resumable failure to explain where the review stopped.
_Avoid_: treating diagnostic stage data as review findings.

**MCP call interruption**:
A host-side interruption that ends the request before a Review result exists; it does not prove that the review reached a terminal state.
_Avoid_: calling every interruption a Review cancellation.

**User cancellation**:
An explicit MCP call interruption requesting that a Synchronous review stop; the server propagates it through the review and completes cleanup.
_Avoid_: promising a Review result to an interrupted caller.

**Partial review result**:
Findings produced before a Resumable failure, with explicit coverage and no claim of complete review coverage.
_Avoid_: treating partial findings as a complete Review result.

**Resumability**:
Whether a Resume session has completed checkpoints that can continue a review; a session ID alone does not imply resumability.
_Avoid_: equating a session ID with a resumable review.

**Session finalization**:
Writing the terminal session record with coverage and failure metadata before a cancelled or deadline-exceeded review releases its resources.
_Avoid_: treating an interrupted session without a terminal record as complete.

**Resume session**:
A persisted OCR review state identified by a session ID that can reuse completed file-level checkpoints for a commit or ref-range review.
_Avoid_: resuming a mutable workspace review or assuming an unfinished file is reusable.

**Progress event**:
A machine-readable JSONL record written to stderr while OCR runs; it signals review activity without mixing with the final JSON result on stdout.
_Avoid_: treating progress events as the terminal review result.
