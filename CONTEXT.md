# Open Code Review Agent Integration

This context defines how the local Open Code Review CLI is exposed to coding-agent hosts.

## Language

**Viewer session**:
A code-review execution record shown under its repository. Its identity is the session ID.
_Avoid_: treating a session ID as proof of resumability.

**Session ID search**:
A lookup by session ID. The repository page searches within the selected repository, while the repository index searches across all repositories. Both support case-insensitive partial IDs and exclude session content.

**OCR runner**:
The local `ocr` CLI process that analyzes Git changes and emits structured review findings.
_Avoid_: using `ocr_review` to mean the CLI process.

**Codex review tools**:
The local `stdio` MCP server exposes `ocr_review` for one review, `ocr_review_wait` to recover the terminal result of an existing in-process review, and `ocr_review_cancel` for explicit cancellation.
_Avoid_: session polling, progress inspection, OpenCode tool.

**Detached review**:
An in-process review that continues after the MCP request transport is interrupted while the local MCP server remains alive. `ocr_review_wait` can return its terminal Review result; if the server process ends, recovery uses a Resume session.
_Avoid_: treating a lost caller connection as review cancellation.

**OpenCode integration**:
The separate plugin under `plugins/open-code-review/opencode/` that registers tools for OpenCode. It does not register tools in Codex.
_Avoid_: treating the OpenCode integration as the Codex integration.

**Review result**:
The structured JSON returned after the OCR runner finishes, fails, is cancelled, or reaches its timeout.
_Avoid_: treating an intermediate progress update as a review result.

**Synchronous review**:
A review request that stays open until OCR returns a terminal Review result, including success, failure, cancellation, or timeout.
_Avoid_: background review, progress event as result.

**Review wait**:
A blocking recovery request that waits for the current or most recent in-process review and returns its same terminal Review result; it can attach to a Detached review, but never starts a second review.
_Avoid_: treating it as a status or polling API.

**Review deadline**:
The server-controlled idle-watchdog point by which a Synchronous review with no meaningful OCR progress and no LLM request in flight must return a terminal Review result; the idle watchdog pauses while one or more LLM requests are in flight, while the host can still cancel the request.
_Avoid_: host timeout as the review result.

**Resumable failure**:
A terminal Review result showing that OCR stopped before completion while a Resume session may continue from completed checkpoints.
_Avoid_: partial success, retry without a Resume session.

**Review stage**:
The named phase and latest affected path reported with a Resumable failure to explain where the review stopped.
_Avoid_: treating diagnostic stage data as review findings.

**Tool-request round**:
One LLM completion cycle in a file review, including the model response and any tool calls it requests before the next completion.
_Avoid_: treating one tool call as one round.

**LLM request in flight**:
The interval from entering the shared `CompletionsWithCtx` request wrapper until that call returns, including provider success, provider error, per-request timeout, or cancellation. Concurrent requests contribute to one in-flight count.
_Avoid_: ending the interval only when a successful response or progress event is emitted.

**Tool-call failure**:
A single tool invocation that returns an error, such as an exact file path not being found.
_Avoid_: treating one failed tool call as a failed file review when the conversation later recovers.

**Conversation completion**:
A file-scoped LLM task that reaches its terminal completion state after the model has handled any recoverable tool errors.
_Avoid_: equating task completion with every intermediate tool call succeeding.

**Coverage status**:
The file-level outcome recorded for the review, based on whether the file-scoped conversation completed, was reused, failed, or waived.
_Avoid_: deriving it from the presence of any intermediate tool-call failure alone.

**Review completion**:
The review as a whole reaches completion only after every item in its declared scope reaches conversation completion. A partial review result can contain useful findings while remaining incomplete.
_Avoid_: treating returned findings or process exit success as proof of complete coverage.

**Quality-first review**:
When deeper model reasoning increases execution time or cost, preserving review quality takes precedence over latency and cost within the explicit safety boundaries.
_Avoid_: lowering reasoning effort solely to avoid a timeout.

**Effort-preserving recovery**:
Recovery from a failed review keeps the same reasoning quality target; it may spend more time or use another attempt, but completion is not purchased by reducing review effort.
_Avoid_: treating a lower-effort retry as equivalent to the original review.

**Path lookup recovery**:
The LLM handles an exact-path lookup failure by finding the canonical path and issuing a new read request; the read tool does not rewrite the path or perform a hidden retry.
_Avoid_: silently substituting a guessed filename or treating the first failed lookup as the final file outcome.

**Aggregate token budget**:
The input-plus-output token ceiling for a complete review run; reaching it stops dispatching additional files and reports incomplete coverage.
_Avoid_: confusing it with the per-request `MAX_TOKENS` limit or the per-file timeout.

**Per-request timeout**:
The provider HTTP timeout applied independently to each LLM request. It remains active while the MCP idle watchdog is paused and stays separate from the per-file timeout, aggregate token budget, and idle watchdog.
_Avoid_: replacing the request timeout with a heartbeat or using the idle watchdog as the request timeout.

**MCP call interruption**:
A host-side interruption that ends the MCP request before a Review result exists. It does not cancel a Detached review and does not prove that the review reached a terminal state.
_Avoid_: calling every interruption a Review cancellation.

**User cancellation**:
An explicit request to stop a Synchronous or Detached review; the server propagates it through the review, preserves completed checkpoints, and completes cleanup.
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
A machine-readable JSONL record written to stderr while OCR runs; it signals meaningful OCR activity such as a completed LLM response or persisted file checkpoint without mixing with the final JSON result on stdout.
_Avoid_: treating progress events as the terminal review result.

**Idle watchdog**:
The MCP server timer that observes gaps without meaningful Progress events while no LLM request is in flight. It pauses when the in-flight count is positive and resumes when the count reaches zero.
_Avoid_: using it to bound an active provider request.

**Synthetic heartbeat**:
A periodic liveness signal emitted while an LLM request is in flight. OCR does not use it to reset the idle watchdog because it cannot establish provider progress.
_Avoid_: treating process liveness as review progress.

**Structural context**:
Repository-level evidence about definitions, callers, dependencies, and architecture that supplements the current diff. Structural context can support a finding about changed code, but it does not expand the finding scope to untouched files.
_Avoid_: treating a referenced external file as the review target.

**MCP tool availability**:
The complete configured codebase-memory capability surface is visible to the review agent. Availability means the agent may select a tool; it does not mean every tool must be called for every file.
_Avoid_: confusing exposed tools with mandatory tool calls.

**MCP query phase**:
The part of a review where the agent gathers repository-level structural context. It starts by checking index readiness, initializes a missing or stale index once, then selects the structural query needed for the current claim.
_Avoid_: indexing as an unbounded per-file review loop.

**Index readiness**:
The state that determines whether repository structural context is current enough to support a query. A missing or stale index requires initialization before graph evidence is treated as reliable.
_Avoid_: treating a successful query response as proof that the index covers every relevant file.

**Structural query fallback**:
Using ordinary text or file inspection when the structural context service is unavailable or its coverage is insufficient, while stating the resulting limitation.
_Avoid_: silently presenting a text-search approximation as a complete relationship query.

**Review search distinction**:
The built-in `code_search` provides direct repository text search; codebase-memory `search_code` provides text search enriched with structural context. Both can be available because they answer different evidence questions.
_Avoid_: assuming either search surface replaces the other for every query.

**Account provider**:
An LLM provider authenticated through an interactive user account rather than a static API key. The account provider has its own model availability and runtime settings.
_Avoid_: treating an account provider as an API-key provider with a different display name.

**Account credentials**:
The access and refresh credentials associated with an Account provider, together with the account identity needed by the provider service.
_Avoid_: treating a model catalog or provider configuration as a credential.

**Model catalog**:
The provider-published set of models visible to the current account, including capability metadata such as context limits and supported reasoning effort.
_Avoid_: treating a stale cached catalog as proof that a model is currently available.

**Reasoning effort**:
The user-selected quality and computation target for a model response. It is part of the provider runtime configuration and can vary by model.
_Avoid_: treating reasoning effort as the review's aggregate token budget or per-request timeout.

**Service tier**:
The processing tier selected for an account request. The user-facing fast mode is a service-tier choice, not a second model or a separate provider.
_Avoid_: treating fast mode as permission to reduce reasoning effort automatically.
