---
status: accepted
---

# Preserve Responses reasoning state across stateless turns

## Context

The Responses client resends the complete conversation on every request. A
reasoning model can return reasoning output items and an assistant message
phase alongside a function call. Dropping those fields before the next turn
loses provider state even though the visible tool-call history is present.

## Decision

Keep the assistant `phase` and raw reasoning output items in the shared
conversation message. When building the next Responses request, replay the
reasoning items before the function calls and resend the assistant phase.
Chat Completions and Anthropic clients ignore these Responses-only fields.

This change preserves the existing effort target. It does not lower effort,
automatically retry a whole file, or switch protocols when a request is slow.

## Consequences

- Stateless Responses turns retain the state required by reasoning models.
- Raw reasoning items remain in memory for the active file conversation but are
  excluded from generic chat JSON serialization.
- Longer model runs still depend on the configured per-file and provider
  request timeouts.

## Validation

- Unit tests verify phase round-tripping and reasoning replay order.
- `go test ./...` passes with an isolated `HOME`.
- A real `gpt-5.6-luna` MAX review completed 1/1 selected item with zero
  execution failures.
