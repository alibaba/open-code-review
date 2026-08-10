## Problem Statement

Follow-up to #805 and the OpenAI-compatible first slice in #806.

The native Anthropic Messages adapter parses visible `thinking` text into `ResponseMessage.ReasoningContent`, but assistant history is still reconstructed from normalized text and `tool_use` calls. That loses the provider-native ordered content blocks, including thinking signatures and `redacted_thinking` blocks, before the next tool-result request.

Anthropic requires thinking blocks around tool use to be preserved unmodified and in their original order. Dropping or rebuilding them can cause a strict endpoint to reject the continuation or silently lose reasoning continuity even when the HTTP request succeeds.

This is a protocol-specific continuation problem. The OpenAI Chat replay envelope added in #806 must not be reused for Anthropic.

## Proposed Solution

Add an adapter-owned `anthropicReplay` envelope that:

- retains the complete ordered assistant content blocks needed for the next Messages request;
- preserves `thinking`, its signature, `redacted_thinking`, text, and `tool_use` blocks without flattening or reordering them;
- serializes replay through Anthropic request types (or a narrowly scoped raw override) owned only by `AnthropicClient`;
- keeps hidden thinking/signatures out of normalized JSON, persisted logs, and generic `ChatResponse` fallback behavior;
- participates in message cloning, token estimation, and atomic active-round compression through the existing private `replayEnvelope` seam;
- defines an explicit model-transition policy, because signed thinking state may not be valid after switching models.

If Anthropic streaming is added or enabled, its adapter must assemble `thinking_delta`, `signature_delta`, and block stop events before publishing a replayable turn.

Anthropic reference: https://platform.claude.com/docs/en/about-claude/models/extended-thinking-models#thinking-encryption

## Acceptance Criteria

- A strict two-request fake Anthropic endpoint returns ordered thinking/signature/text/tool-use blocks, then verifies that the next request replays every required block exactly before matched `tool_result` blocks.
- Signed thinking and `redacted_thinking` each have regression coverage.
- Multiple tool calls retain original order and matching IDs.
- Normalized display content and tool execution remain unchanged; generic code cannot inspect or fabricate the Anthropic envelope.
- Replay state survives `Message.Clone`, no-tool retry handling, and an active-round compression decision without being split from its tool results.
- Persistence/logging tests prove that signatures and hidden/redacted thinking are not exposed.
- Token accounting includes the private replay payload without exposing its contents.
- Model changes either reject/strip incompatible replay state according to an explicit adapter-owned policy and are covered by tests.
- Existing OpenAI Chat and OpenAI Responses behavior is unchanged.

## Alternatives Considered

Flattening thinking into ordinary assistant text is not valid: it changes block type, order, and signature semantics. Reusing the OpenAI-compatible raw envelope would also couple unrelated protocols and recreate the generic-envelope problem fixed by #806.

## Affected Area

Review Agent / LLM interaction

## Additional Context

#806 intentionally limits its implementation to OpenAI-compatible Chat Completions so that each protocol can own its native continuation format.
