---
status: accepted
---

# Keep review execution alive across MCP transport interruption

The MCP server owns the review lifecycle after `ocr_review` starts. A lost caller connection is an MCP call interruption, not a cancellation: while `ocr mcp serve` remains alive, the review becomes a Detached review and `ocr_review_wait` can return its terminal result. Add explicit `ocr_review_cancel` for deliberate cancellation; for commit/ref-range reviews whose session persistence succeeds, cancellation preserves completed checkpoints and returns a resumable session. Workspace reviews retain checkpoints and diagnostics but are not resumable.

Provider request failures use the bounded retry policy at the shared LLM request boundary defined by [ADR 0012](0012-unified-llm-retry-budget.md): three total attempts with short exponential backoff, retrying only transient transport errors, timeouts, truncated responses, HTTP `408`, `409`, `429`, `5xx`, and configured `retry_codes` responses. Authentication failures, other `4xx` responses, and caller context cancellation stop immediately. This retry stays inside one review and never starts a second review invocation.

If the MCP server process ends, recovery uses the existing local session JSONL: run `ocr session list`, then invoke the same commit or ref-range review with `resume`. No external job queue or database is introduced. Workspace review resume and SSH session persistence remain outside this decision.

## Consequences

- A disconnected caller may not receive the final result, while the server can continue consuming provider resources until completion, explicit cancellation, provider/per-file limits, or the idle watchdog stops it.
- `ocr_review_wait` only recovers an execution retained by the same live MCP server process.
- A process termination can lose the in-flight request, but completed file checkpoints remain the recovery boundary.
