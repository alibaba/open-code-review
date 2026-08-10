## Problem Statement

Follow-up to #805 and the OpenAI-compatible first slice in #806.

`OpenAIResponsesClient` supports the Responses API and currently forces `store: false`, but it maps response output into normalized assistant text/function calls and reconstructs the next input from that projection. Typed reasoning output items, encrypted reasoning content, item IDs, and original output ordering are therefore not retained across a tool turn.

Official OpenAI documentation says that applications managing history manually should preserve and resend every response output item; for `store: false` or Zero Data Retention flows, encrypted reasoning items must be replayed. Reconstructing only a message and function calls can lose reasoning continuity or produce a provider-invalid continuation.

Official guidance: https://developers.openai.com/api/docs/guides/latest-model#using-gpt-56

## Proposed Solution

Add an adapter-owned `openAIResponsesReplay` envelope that:

- retains the typed response output items required as subsequent `input`, including reasoning and function-call items in original order;
- requests and preserves `reasoning.encrypted_content` whenever required by the active Responses contract and storage policy;
- keeps each output `id`, function `call_id`, status, and tool-output linkage intact;
- uses the official SDK's Responses input/output conversion helpers where they preserve the wire contract, with narrowly scoped raw preservation only for unsupported provider fields;
- remains opaque to the review loop, normalized `ChatResponse`, persistence, and logs;
- participates in cloning, token estimation, and active-round compression through the private `replayEnvelope` seam.

The current `store: false` behavior should use manual typed replay. A future `previous_response_id` path may be supported only as an explicit, mutually exclusive server-stored continuation policy.

## Acceptance Criteria

- A strict two-request fake Responses endpoint returns a reasoning item with encrypted content plus one or more function calls, then verifies that the next request includes the original typed output items and matched `function_call_output` items with exact IDs and ordering.
- `store: false` is covered, including the request/response handling needed to obtain and replay encrypted reasoning.
- Parallel function calls preserve every `call_id` and tool-result relationship.
- Reasoning summaries remain available for normalized display without becoming the replay source.
- Generic `ChatResponse` fallback cannot fabricate a Responses replay envelope, and OpenAI Chat replay types are not reused.
- Replay survives cloning, no-tool retry behavior, token accounting, and active-round compression without leaking encrypted or hidden reasoning to persisted logs.
- If streaming Responses support is added, event assembly finalizes complete typed items before the turn becomes replayable.
- Existing OpenAI Chat and Anthropic behavior is unchanged.

## Alternatives Considered

Flattening a reasoning item into assistant text loses its item type and encrypted continuation state. Always using `previous_response_id` is incompatible with the client's current `store: false` policy and deployments that require manual or zero-retention history.

## Affected Area

Review Agent / LLM interaction

## Additional Context

#806 establishes the provider-owned replay seam but intentionally leaves typed Responses replay to this dedicated adapter.
