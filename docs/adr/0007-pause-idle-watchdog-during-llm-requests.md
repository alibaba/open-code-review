---
status: accepted
---

# Pause the MCP idle watchdog during in-flight LLM requests

## Context

The MCP idle watchdog resets on meaningful OCR progress, such as a completed LLM response or persisted file checkpoint. A non-streaming LLM request can wait longer than the base idle duration without emitting a progress event, even while its provider request is still within its own timeout. The previous `OCR_LLM_TIMEOUT + 5 minutes` idle grace coupled two different safety boundaries and could still report an idle timeout during a valid request.

A periodic heartbeat would only prove that the OCR process is alive. It could keep extending the idle timer while the provider is hung and would obscure the per-request timeout that should bound the provider call. The review dispatcher can also have concurrent file requests, so one boolean pause flag cannot represent the request lifecycle safely.

## Decision

During an MCP `ocr_review`, wrap the runtime `LLMClient` at the shared `CompletionsWithCtx` boundary. Increment a thread-safe in-flight counter before each call and decrement it on every return path, including provider success, provider errors, per-request timeout, caller cancellation, and panics handled by the surrounding runtime. Use a deferred decrement so the counter cannot remain elevated after an error path.

The idle watchdog pauses while the in-flight counter is positive and resumes when the counter reaches zero. The request-start transition is synchronized with the watchdog timer state, so a request that begins at the idle boundary is recognized before idle cancellation. The idle watchdog continues to apply during non-LLM work and whenever no request is in flight.

The wrapper is installed only for MCP review execution. It covers plan, main-loop, compression, and review-filter requests in that execution. Normal CLI review and scan execution keep their existing behavior because they do not own an MCP idle watchdog.

Remove `mcpReviewIdleGrace` and `SetLLMTimeout`. The idle watchdog uses its base idle duration. Provider per-request timeout, caller cancellation, and any independently configured whole-review maximum duration remain active while the idle watchdog is paused. OCR emits no synthetic heartbeat; existing progress and session telemetry retain their current meanings.

## Alternatives considered

- **Emit a periodic heartbeat during each request**: rejected because process liveness does not establish provider progress and can hide a provider stall until the per-request timeout.
- **Keep `max(base idle, per-request timeout + grace)`**: rejected because request timeout and idle watchdog have separate ownership after the pause behavior is available.
- **Instrument every provider implementation separately**: rejected because the shared client wrapper covers all protocols and future request call sites without duplicated lifecycle logic.
- **Pause only after a successful response**: rejected because the watchdog can expire before the response arrives, which is the failure this decision addresses.

## Consequences

- A valid LLM request may occupy the synchronous MCP call until its per-request timeout or another active cancellation boundary.
- Long non-LLM gaps remain protected by the idle watchdog.
- Concurrent requests keep the watchdog paused until the last request returns.
- Progress events remain evidence of meaningful OCR progress, and request lifecycle evidence remains in existing session telemetry.

## Validation

The implementation must cover these cases:

1. A single LLM request longer than the idle duration but shorter than its per-request timeout does not trigger the idle watchdog.
2. Two concurrent requests keep the watchdog paused after the first returns and until the second returns.
3. A per-request timeout still cancels the request and preserves the existing error and session records.
4. With no active LLM request and no progress, the idle watchdog still triggers.
