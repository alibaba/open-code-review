# Assistant-turn replay: production interoperability audit

Audit date: 2026-08-09  
Target: `fix/assistant-turn-replay` at `c8b9847` (`fix(llm): harden provider replay boundaries`), including all final production-hardening revisions  
Scope: OpenAI-compatible Chat Completions replay and streaming, with Anthropic Messages and OpenAI Responses examined only to define the correct future adapter boundaries

## Executive verdict

The central design is sound: provider continuation data belongs in a private, adapter-owned envelope, while the loop consumes only normalized visible content and tool calls. The current uncommitted refactor is also materially safer than replaying the entire response object: it starts from the official SDK's request-safe assistant parameter and overlays only exact content presence plus provider extensions. That protects against sending response-only `annotations` and full audio response payloads back as request fields.

The final synchronized WIP fixes every P1 wire issue discovered during this audit: it requests `stream_options.include_usage`, upgrades `openai-go` to v3.50.0 for legal SSE heartbeat handling, preserves omitted tool-only `content`, and now replays split legacy `function_call` name/arguments correctly. It also adds exact request-level coverage for Gemini's documented nested signature shape and empty-versus-absent reasoning.

The WIP now fixes an important Kimi failure in the committed implementation: when a streamed tool-call turn never contained a `content` field, replay keeps that field omitted instead of inventing `content: null`. A Kimi-specific policy should still be considered for effectively empty content that is explicitly present.

| Priority | Finding | Production effect | Recommended action |
|---|---|---|---|
| Resolved in WIP | `stream_options.include_usage` | The streaming path now requests usage and the test rejects a request that omits it | Keep the request-body assertion; retain approximate fallback for interrupted streams |
| Resolved in WIP | Legacy `function_call` replay became `{}` | Older OpenAI-compatible/Gemini-compatible function calling broke on the next retry | Canonical name/arguments now fall back to adapter-accumulated values; split-delta coverage passes |
| Resolved in WIP | SSE heartbeat parsing | `openai-go` is now v3.50.0 and a comment-only event test was added | Keep the dependency and regression test |
| Resolved in WIP | Kimi streamed omission | A strict two-request test rejects any present tool-only `content`, matching Kimi's first-party E2E | Keep the provider-specific empty-content policy out of the generic adapter unless explicit empty Kimi responses are observed |
| Resolved in WIP | Gemini signature wire shape | A bare-field fixture did not prove nested extension preservation | Non-streaming and parallel streaming tests now assert `extra_content.google.thought_signature` exactly |
| P2 | Kimi explicit null/empty/whitespace policy | Kimi's production converter drops all effectively-empty tool-call content, while generic replay intentionally preserves exact content | Add a provider/model-specific request-lowering policy only if real Kimi traffic produces an explicitly empty field |
| P2 | Replay has protocol type but no origin/model affinity | Opaque signed state could be replayed through another `OpenAIClient` endpoint or model after routing/model changes | Bind replay to an immutable adapter affinity key and reject/strip it on mismatch |

## What the current code does on the wire

### Non-streaming Chat Completions

`mapOpenAIResponse` receives an SDK `ChatCompletionMessage`. The WIP's `openAIReplayMessageFromResponse`:

1. decodes `message.RawJSON()` so omitted, explicit `null`, empty string, and unknown provider fields remain distinguishable;
2. calls `message.ToAssistantMessageParam()` for standard request fields;
3. restores the exact `content` presence/value;
4. overlays provider extensions onto tool calls and the assistant message;
5. converts response audio to request-safe `{ "id": ... }` and drops response-only annotations.

This is the right hybrid. The SDK explicitly exposes the unmodified response JSON through `RawJSON`, and response field metadata through `JSON.ExtraFields`; its response type contains fields that are not valid in the assistant request type. Its conversion helper deliberately elides explicit null, maps audio to its ID, and converts known tool-call variants ([openai-go v3.41.0 response and conversion source](https://github.com/openai/openai-go/blob/v3.41.0/chatcompletion.go#L2262-L2355)). A blind raw replay would therefore preserve vendor fields but also bypass the standard request schema; a typed-only replay would be request-safe but lose unknown signed state and exact empty/null/omitted distinctions.

The use of `param.Override[ChatCompletionAssistantMessageParam]` in `buildOpenAIParams` is appropriate after that adapter-owned sanitization. `Override` exists specifically to supply raw JSON for a parameter and makes that raw JSON authoritative ([openai-go parameter source](https://github.com/openai/openai-go/blob/v3.41.0/packages/param/param.go)). It should not be moved into generic `ChatResponse` fallback code.

### Streaming Chat Completions

The implementation correctly uses two accumulators with different jobs:

- `openai.ChatCompletionAccumulator` canonicalizes standard visible content, refusal, usage, and indexed tool-call fragments.
- `openAIStreamChoiceState` reads every `choice.delta.RawJSON()` and retains fields the generated SDK does not know.

This duplication is necessary. The official accumulator documents that its `ChatCompletion.JSON` is not accumulated and its implementation ignores raw JSON while concatenating only known fields. It also clamps negative tool-call indices because AWS Bedrock can emit `-1`, and warns that `JustFinishedToolCall` cannot be relied on with parallel tool calls ([openai-go v3.41.0 accumulator source](https://github.com/openai/openai-go/blob/v3.41.0/streamaccumulator.go#L34-L38), [tool-call accumulation](https://github.com/openai/openai-go/blob/v3.41.0/streamaccumulator.go#L77-L140)). The WIP mirrors index clamping and does not use the unsafe completion callback for parallel calls.

Unknown top-level provider fields are intentionally treated as snapshots (last delta wins), not recursively concatenated. That is a good default because arbitrary JSON has no universal delta algebra. It also means compatibility is only guaranteed for explicitly understood delta fields or provider extensions that arrive as complete snapshots. Any provider that incrementally streams a new opaque field needs a provider-specific merge policy and test.

### Display normalization remains separate

The replay envelope preserves the assistant's exact visible `content`, including leading/trailing whitespace and `<think>` text. `ChatResponse.Content()` still strips think tags and trims the display value. That separation meets the contract: presentation normalization must not mutate the provider history sent back on a tool retry.

## Concrete remaining findings

### 1. Streaming usage (resolved in the synchronized WIP)

The current streaming path calls:

```go
stream := c.sdk.Chat.Completions.NewStreaming(ctx, params, opts...)
```

and now sets `params.StreamOptions.IncludeUsage = openai.Bool(true)` before doing so. `NewStreaming` adds `stream: true`; it does not opt into usage itself. The SDK's request type states that `include_usage: true` causes one additional usage-bearing chunk before `[DONE]`, while the other chunks carry `usage: null` ([openai-go stream-options source](https://github.com/openai/openai-go/blob/v3.41.0/chatcompletion.go#L3202-L3218)). Kimi CLI's production provider independently sets `stream_options={"include_usage": True}` on every streamed Chat Completions request ([Kimi provider source](https://github.com/MoonshotAI/kimi-cli/blob/cbc15c076d17f70fec9f89c90c0502e68657f505/packages/kosong/src/kosong/chat_provider/kimi.py#L169-L180)), and OpenCode's native OpenAI Chat protocol does the same ([OpenCode source](https://github.com/anomalyco/opencode/blob/38e10eb1408feb700021b8e8766fb0ab41bf84e2/packages/llm/src/protocols/openai-chat.ts#L345-L365)).

The synchronized local usage test now decodes the request and returns an error unless it contains:

```json
{
  "stream": true,
  "stream_options": { "include_usage": true }
}
```

If the stream is interrupted before the final chunk, usage can still be absent; the SDK documents that limitation. The existing approximate fallback remains necessary.

### 2. Streamed legacy `function_call` replay (resolved in the synchronized WIP)

The audit found that `openAIStreamChoiceState` correctly concatenated `delta.function_call.name` and `.arguments`, but finalization could replay only `{}` because the official `ChatCompletionAccumulator` does not accumulate this deprecated field. The final WIP now uses the adapter accumulator as the canonical fallback for those two standard fields, then overlays only genuinely unknown function-call extensions.

This is still a live compatibility surface, not merely dead code. Google's official OpenAI-compatibility table says deprecated `function_call` and `functions` remain supported for backward compatibility ([Google Cloud compatibility reference](https://docs.cloud.google.com/gemini-enterprise-agent-platform/models/migrate/openai/overview#supported-parameters)).

The request-boundary regression test uses split fragments such as:

```json
{"delta":{"function_call":{"name":"file_","arguments":"{\"path\":"}}}
{"delta":{"function_call":{"name":"read","arguments":"\"a.go\"}"}}}
```

and proves that the next request contains `{"name":"file_read","arguments":"{\"path\":\"main.go\"}"}` rather than an empty object.

### 3. SSE parser upgrade (resolved in the synchronized WIP)

The audit initially found `github.com/openai/openai-go/v3 v3.41.0`. The official v3.43.0 changelog records two directly relevant corrections: the example was fixed to request stream usage, and the SSE decoder was fixed to skip blocks with no `data` field ([openai-go changelog](https://github.com/openai/openai-go/blob/v3.50.0/CHANGELOG.md#3430-2026-07-14), [SSE fix commit](https://github.com/openai/openai-go/commit/114224dd71cf6695a01de8353352145832662e84)). Current v3.50.0 tests cover comment-only/retry directives and CRLF framing ([current SSE tests](https://github.com/openai/openai-go/blob/v3.50.0/packages/ssestream/ssestream_test.go)). The synchronized WIP now pins v3.50.0 and adds a comment-only heartbeat regression.

The project and local toolchain are already Go 1.25.5, satisfying v3.50.0's Go requirement. A v3.41-to-v3.50 comparison shows no change to `ChatCompletionAccumulator`'s raw-JSON limitation, so the custom replay accumulator remains required after the upgrade.

### 4. Kimi: omission, exact reasoning presence, and whitespace

Kimi CLI is useful primary production evidence because it uses the OpenAI Chat Completions API and ships an E2E mock for this exact retry sequence. Its first response streams an assistant tool call with no `content` member. Its second request is rejected with `400 "text content is empty"` if the prior assistant tool-call message contains any present effectively-empty content; successful behavior omits `content` entirely ([Kimi E2E source](https://github.com/MoonshotAI/kimi-cli/blob/cbc15c076d17f70fec9f89c90c0502e68657f505/tests/e2e/test_kimi_empty_tool_call_content_e2e.py#L1-L12), [wire fixture and rejection](https://github.com/MoonshotAI/kimi-cli/blob/cbc15c076d17f70fec9f89c90c0502e68657f505/tests/e2e/test_kimi_empty_tool_call_content_e2e.py#L53-L79)).

The committed branch's prior generic fallback invented `content: null` for this shape. The synchronized WIP fixes the exact fixture by tracking `contentSeen`; when the stream never contains `content`, final replay omits it. Its strict two-request endpoint rejects the retry with Kimi's `text content is empty` error if any `content` member is present, so the success path proves omission on the actual wire.

Kimi's production conversion is slightly stricter: for an assistant tool-call message it drops content that is empty or whitespace-only, and it separately preserves whether `reasoning_content` was present even when its value is the empty string ([Kimi conversion source](https://github.com/MoonshotAI/kimi-cli/blob/cbc15c076d17f70fec9f89c90c0502e68657f505/packages/kosong/src/kosong/chat_provider/kimi.py#L326-L362), [empty reasoning handling](https://github.com/MoonshotAI/kimi-cli/blob/cbc15c076d17f70fec9f89c90c0502e68657f505/packages/kosong/src/kosong/chat_provider/kimi.py#L469-L497)). The current WIP correctly preserves an explicitly empty `reasoning_content` as provider state. It deliberately preserves exact whitespace content, which is correct for the generic OpenAI-compatible adapter but may be rejected by Kimi-for-Coding when paired with tool calls. If this is observed, implement the omission as a Kimi endpoint/model request-lowering policy; do not trim the generic replay or normalized turn.

Moonshot's Kimi K2 tool-calling guide also demonstrates appending the complete returned `choice.message` before tool results, confirming that reasoning/tool-call state belongs to the assistant turn rather than being reconstructed from visible text ([Kimi K2 tool guide](https://github.com/MoonshotAI/Kimi-K2/blob/main/docs/tool_call_guidance.md)).

### 5. Gemini: test the documented nested extension

Google documents that Gemini 3 Chat Completions supports thought signatures ([Gemini OpenAI compatibility](https://ai.google.dev/gemini-api/docs/openai#thinking)). Its OpenAI-compatible schema places provider-specific content under:

```json
{
  "extra_content": {
    "google": {
      "thought_signature": "..."
    }
  }
}
```

`thought_signature` is bytes used to validate model-returned thought state, while `thought` is a separate boolean ([Google Cloud `extra_content` reference](https://docs.cloud.google.com/gemini-enterprise-agent-platform/models/migrate/openai/overview#gemini-specific-parameters)). Google's native thinking documentation says signatures are encrypted continuation state and, in stateless mode, thought blocks must be resent exactly; it also notes that native GenerateContent signatures can live on function-call parts ([Gemini thinking guide](https://ai.google.dev/gemini-api/docs/thinking#thought-signatures)).

The current raw extension overlay should preserve a complete nested `extra_content.google` snapshot on a tool call. The existing replay test instead puts `thought_signature` directly on the tool-call object, which does not prove the documented wire shape. Add exact nested non-streaming and streaming fixtures and assert byte-for-byte signature equality in the second request. Include two parallel calls and cover Google's convention that only one call/part may carry the signature.

If Gemini becomes a first-class provider rather than merely an OpenAI-compatible endpoint, Google recommends its native API/SDK for applications not already committed to the OpenAI libraries ([Gemini compatibility guidance](https://ai.google.dev/gemini-api/docs/openai)). A native Gemini adapter can then use `google.golang.org/genai` and keep thought-signature rules out of the generic Chat module.

### 6. Bind opaque replay to its origin

The private `openAIReplay` type prevents Anthropic or Responses code from inspecting it, but all `OpenAIClient` instances accept the same Go type. The envelope currently has only raw assistant JSON and an approximate token count. If a routing change reuses conversation messages with another base URL or model, signed provider state can be sent to the wrong backend.

Add an immutable affinity value when the adapter creates replay, for example:

```text
protocol=openai-chat + normalized endpoint/provider identity + model or compatible model family
```

`buildOpenAIParams` should replay only when the current client accepts that affinity. The policy should be adapter-owned: some providers allow aliases or compatible model upgrades, while Anthropic's current guidance explicitly says model switches should strip prior thinking blocks tied to another model ([Anthropic thinking guide](https://platform.claude.com/docs/en/about-claude/models/extended-thinking-models#preserving-thinking-blocks)).

### 7. Token estimates remain deliberately approximate

`openAIReplay.tokenCount()` counts the serialized raw JSON using the generic tokenizer. This is conservative in the sense that opaque reasoning is no longer omitted from compression decisions, but it is not model-accurate for GPT-4o/GPT-5/Kimi/Gemini families. Provider-reported usage is authoritative when present; fixing `include_usage` has higher value than expanding the shared tokenizer table.

If exact preflight sizing becomes necessary, make token estimation another adapter method keyed by model. Do not expose or normalize the hidden state merely to count it.

## Correct future adapter boundaries

### Anthropic signed thinking is not covered by this Chat slice

Anthropic returns ordered `thinking` blocks with an opaque `signature`, and can return opaque `redacted_thinking` blocks. During a tool-use turn, the complete blocks must be returned unmodified and in order; modified blocks can be rejected. In streaming, the signature arrives as `signature_delta` immediately before `content_block_stop` ([Anthropic thinking documentation](https://platform.claude.com/docs/en/about-claude/models/extended-thinking-models#thinking-encryption), [tool-use preservation](https://platform.claude.com/docs/en/docs/build-with-claude/extended-thinking#preserving-thinking-blocks)).

The current `mapAnthropicResponse` only joins visible `block.Thinking`, and `buildAnthropicParams` reconstructs text/tool-use blocks. It drops signatures, redacted blocks, and original ordering. Therefore Anthropic thinking-enabled tool replay is not production-safe today. The correct follow-up is an Anthropic-specific replay envelope containing typed content blocks and a `signature_delta` stream state machine—not encoding these blocks in an OpenAI assistant object.

### OpenAI Responses typed/encrypted reasoning is not covered by this Chat slice

For manually managed Responses history, OpenAI requires prior reasoning output items to be included in subsequent input. The reasoning item can carry `encrypted_content` when requested through `include: ["reasoning.encrypted_content"]`; for `store: false` or zero-data-retention flows, encrypted reasoning must be replayed ([OpenAI Responses reference](https://platform.openai.com/docs/api-reference/responses-streaming/response/refusal/delta), [OpenAI model guidance](https://developers.openai.com/api/docs/guides/latest-model)).

The v3.41.0 Go SDK already exposes typed `ResponseReasoningItem`, `EncryptedContent`, and conversion helpers in its Responses package ([openai-go Responses source](https://github.com/openai/openai-go/blob/v3.41.0/responses/response.go)). The repository's `OpenAIResponsesClient` forces `store: false`, does not request encrypted reasoning, and reconstructs only assistant text/function calls. Its reasoning continuation is therefore incomplete. The future adapter should retain and replay typed output items (or intentionally use `previous_response_id` with server-side storage); it should not reuse `openAIReplay`.

## What OpenCode's current source validates

OpenCode's upstream implementation has moved toward deep protocol modules rather than a universal reasoning schema:

- its OpenAI Chat protocol owns Chat request lowering and streaming state, and explicitly requests stream usage ([OpenAI Chat protocol](https://github.com/anomalyco/opencode/blob/38e10eb1408feb700021b8e8766fb0ab41bf84e2/packages/llm/src/protocols/openai-chat.ts));
- its OpenAI Responses protocol retains typed encrypted reasoning metadata ([Responses protocol](https://github.com/anomalyco/opencode/blob/38e10eb1408feb700021b8e8766fb0ab41bf84e2/packages/llm/src/protocols/openai-responses.ts));
- its Anthropic protocol has explicit `signature_delta` handling ([Anthropic protocol](https://github.com/anomalyco/opencode/blob/38e10eb1408feb700021b8e8766fb0ab41bf84e2/packages/llm/src/protocols/anthropic-messages.ts#L700-L740));
- its Gemini protocol stores `thoughtSignature` in Google provider metadata ([Gemini protocol](https://github.com/anomalyco/opencode/blob/38e10eb1408feb700021b8e8766fb0ab41bf84e2/packages/llm/src/protocols/gemini.ts)).

Its older/provider transform also preserves an explicitly empty `reasoning_content` for providers such as DeepSeek instead of treating empty as absent ([OpenCode transform](https://github.com/anomalyco/opencode/blob/38e10eb1408feb700021b8e8766fb0ab41bf84e2/packages/opencode/src/provider/transform.ts#L320-L345)). This supports the current branch's adapter-owned envelope direction. The reusable lesson is architectural, not a package import: each protocol owns its wire types, stream state machine, continuation metadata, and model/provider compatibility rules.

## Minimum production gate to add

Before presenting this slice as production-checked, add or verify these cases at the HTTP request boundary:

1. [x] Streaming request contains `stream_options.include_usage=true` and the final usage-only chunk is recorded. Keep the interrupted-stream fallback.
2. [x] Kimi first-party response shape: no `content` delta + tool call -> next request omits `content`; the fake endpoint rejects invalid present content.
3. [x] Explicit empty `reasoning_content` is distinct from absent in both non-streaming and streaming replay.
4. [x] Split legacy `function_call` stream fragments replay canonical name/arguments, not `{}`.
5. [x] Gemini `tool_calls[].extra_content.google.thought_signature` survives non-streaming, streaming, and parallel-call assembly.
6. [x] Negative tool-call index and subsequent normalized index refer to the same call.
7. [x] Response-only `annotations`, audio data/transcript/expiry, and explicit null standard fields do not leak into request JSON.
8. [x] SSE comment-only heartbeat blocks work after the SDK upgrade. The upstream SDK additionally covers `retry:` and CRLF framing.
9. [ ] Replay affinity mismatch is rejected or intentionally stripped. This remains a P2 defense-in-depth item because a review loop uses one immutable client endpoint/model; it should be addressed if cross-client conversation routing is introduced.

The built `dist/opencodereview` CLI was also run end to end against a strict local OpenAI-compatible service using real Git repositories with uncommitted diffs. Both streaming and non-streaming reviews completed their `file_read` and `task_done` tool rounds; the service rejected missing stream usage, synthesized tool-only content, altered visible/reasoning/provider state, broken indexed reasoning, lost nested Gemini signatures, or mismatched tool results. Streaming additionally exercised a comment-only SSE heartbeat and a usage-only final chunk.

This audit did not send paid live traffic to OpenAI, Moonshot, Gemini, or Anthropic because no provider credentials or spend were placed in scope. It used the repository's actual wire code and CLI, official SDK source at the pinned and current versions, first-party provider documentation, Kimi's production implementation/E2E fixture, and OpenCode's upstream protocol source. A credentialed smoke test remains useful, but it should complement rather than replace the deterministic request-shape and binary tests above.
