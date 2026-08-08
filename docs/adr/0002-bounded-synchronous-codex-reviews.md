---
status: accepted
---

# Bounded synchronous Codex reviews

Keep Codex review as one synchronous `ocr_review` call. The MCP server owns the in-process review after a transport interruption; explicit cancellation propagates through the review context. Terminal failures preserve finalized session metadata, partial findings, coverage, diagnostics, and resumability, while successful calls keep the native OCR JSON. The fixed whole-review deadline remains superseded by [ADR 0004](0004-unlimited-default-aggregate-token-budget.md); the lifecycle and request-retry refinement is recorded in [ADR 0008](0008-detached-review-recovery.md).

## Considered Options

- **Asynchronous job plus status polling**: rejected because it changes the existing one-terminal-result contract. The recovery-only `ocr_review_wait` tool is accepted because it blocks on the existing in-process call and returns that same terminal result.
- **Four-hour blocking call**: rejected because the host can terminate an unproductive call before a useful terminal error is visible; the active review remains bounded by provider limits and the idle watchdog.
- **Separate `ocr_cancel`/`ocr_reset` tools**: the original synchronous contract rejected them; that decision is superseded for cancellation by ADR 0008 because detached execution requires an explicit stop operation.

## Consequences

Commit and ref-range reviews can resume only when finalized checkpoints exist; workspace reviews cannot resume. Review-level automatic retry remains disabled; bounded provider-request retry is defined by ADR 0008. Failure payloads use the fixed `error_type` values `deadline_exceeded`, `cancelled`, `runner_error`, `invalid_result`, `integration_error`, and `persistence_error`; they include the latest review stage, progress timestamp, path, session ID, coverage, and partial result when available. A session persistence failure makes the result non-resumable even when in-memory findings exist.
