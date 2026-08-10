## Description

Fixes #805.

This change replaces lossy `Content()` / `ToolCalls()` reconstruction with an `AssistantTurn` that carries provider-owned continuation state opaquely. The OpenAI-compatible Chat Completions adapter is the first slice: it uses the official SDK's response-to-request conversion for standard fields, then restores exact visible-content presence and provider extensions without trimming, stripping think tags, or leaking response-only annotations and audio payloads into the next request. The generic `ChatResponse` fallback keeps its existing normalized display behavior and never invents an OpenAI envelope.

Streaming now has an explicit adapter-owned state machine for content, refusal, reasoning details, parallel tool calls, legacy `function_call`, and opaque provider snapshots. It preserves Kimi-style omitted tool-only content, Gemini `extra_content.google.thought_signature`, normalized negative tool indexes, and exact empty-versus-absent reasoning; it also requests usage and upgrades `openai-go` to v3.50.0 for comment-only SSE heartbeat handling.

Replay state stays out of normalized JSON and persisted logs, while cloning and token estimates retain its request semantics. Compression treats the active assistant/tool round atomically, including no-tool retries and empty-turn behavior.

This PR intentionally leaves Anthropic signed thinking blocks and OpenAI Responses typed/encrypted replay for future provider adapters. Those protocols require their own native continuation envelopes and must not be flattened into this OpenAI-compatible Chat Completions representation.

## Type of Change

- [x] Bug fix (non-breaking change that fixes an issue)
- [ ] New feature (non-breaking change that adds functionality)
- [ ] Breaking change (fix or feature that would cause existing functionality to change)
- [ ] Refactoring (no functional changes)
- [ ] Documentation update
- [ ] CI / Build / Tooling

## How Has This Been Tested?

Deterministic HTTP-boundary tests cover non-streaming and streaming replay, exact visible content, omitted/null/empty fields, opaque and indexed provider state, nested Gemini signatures, parallel and legacy tool calls, no-tool retry, empty turns, usage/token accounting, persistence redaction, SSE heartbeats, and compression atomicity.

- [x] `make test` passes locally
- [x] Manual testing (describe below)

The built `dist/opencodereview` CLI completed real streaming and non-streaming reviews against a strict local OpenAI-compatible service and disposable Git repositories. The service validated the second-request replay envelope, matched tool results, usage opt-in/accounting, tool-only content omission, SSE heartbeat handling, and completion through `file_read` and `task_done` rounds.

Validation commands:

```shell
go test ./internal/llmloop -timeout=120s -count=1
GOFLAGS=-p=1 make test
make check
make build
git diff --check
```

## Checklist

- [x] My code follows the project's coding style (`go fmt`, `go vet`)
- [x] I have performed a self-review of my code
- [x] I have added tests that prove my fix is effective or my feature works
- [x] New and existing unit tests pass locally with my changes
- [x] I have updated the documentation accordingly (not applicable; no user-facing configuration or behavior changes)
- [ ] I have signed the CLA

## Related Issues

Fixes #805

Follow-up provider adapters:

- #811 — preserve Anthropic signed/redacted thinking across tool turns
- #812 — replay typed/encrypted OpenAI Responses reasoning items
