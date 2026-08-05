---
status: accepted
---

# Bounded synchronous Codex reviews

Keep Codex review as one synchronous `ocr_review` call, but make the MCP server the deadline owner with a fixed 60-minute limit and give the host a one-minute grace period. Cancellation must propagate through the review context; terminal failures preserve finalized session metadata, partial findings, coverage, diagnostics, and resumability, while successful calls keep the native OCR JSON.

## Considered Options

- **Asynchronous job plus status polling**: rejected because it changes the existing one-terminal-result contract. The recovery-only `ocr_review_wait` tool is accepted because it blocks on the existing in-process call and returns that same terminal result.
- **Four-hour blocking call**: rejected because the host can terminate an unproductive call before a useful terminal error is visible.
- **Separate `ocr_cancel`/`ocr_reset` tools**: rejected because they add a control surface without helping the already-blocked caller or a separate server instance.

## Consequences

Commit and ref-range reviews can resume only when finalized checkpoints exist; workspace reviews cannot resume. Automatic retry remains disabled. Failure payloads use the fixed `error_type` values `deadline_exceeded`, `cancelled`, `runner_error`, `invalid_result`, `integration_error`, and `persistence_error`; they include the latest review stage, progress timestamp, path, session ID, coverage, and partial result when available. A session persistence failure makes the result non-resumable even when in-memory findings exist.
