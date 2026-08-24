Parent document: `/CLAUDE.md`
Related documents:
- `pages/src/content/docs/en/telemetry.md` (user-facing recipes — this doc adds implementation depth and known gaps)
- `docs/operations/FAILURE_MODES.md`
- `docs/architecture/CALL_GRAPH.md`

Read this when:
- You're wiring a collector, debugging why telemetry shows nothing, or need the exact span/metric/event inventory.

Purpose:
- Implementation-level telemetry detail: exact span tree, metric names, config resolution order, and a confirmed doc/code drift.

Scope:
- Included: `internal/telemetry/` internals, full span/metric/event inventory (review + scan).
- Excluded: collector setup recipes (already well covered in `pages/.../telemetry.md`, not repeated here).

---

# Observability

Telemetry is off by default (`OCR_ENABLE_TELEMETRY` / `telemetry.enabled`). Full user-facing setup: `pages/src/content/docs/en/telemetry.md`. This doc is the implementation cross-check.

## Config resolution order

Defaults (`enabled=false`, `exporter=console`) < `~/.opencodereview/config.json` `telemetry.*` < environment variables (env always wins). `Init()` in `internal/telemetry/provider.go` is idempotent and called once from `main.go`; it builds an OTel resource (process/OS/host attrs) then wires OTLP (grpc/http-protobuf/http-json per `OTEL_EXPORTER_OTLP_PROTOCOL`) or console exporters for both traces and metrics.

## Span inventory (confirmed by source grep)

Review (`internal/agent/agent.go`): `review.run`, `diff.parse`, `no.files.changed`, `review.started`, `subtask.panic`, `subtask.execute.<file>`, `plan.skipped`, `plan.failed`, `plan.execute`, `token.threshold.exceeded`, `main.loop`, `review_filter.execute`, `review_filter.completed`.

Scan (`internal/scan/agent.go`): `scan.enumerate`, `scan.no.files`, `scan.started`, `scan.subtask.<file>`, `token.threshold.exceeded`.

Shared (`internal/llmloop/loop.go`, `internal/diff/relocation.go`): `tool.execute.<toolName>` (via `StartToolSpan`), `llm.request` (via `StartLLMSpan`, called from the main loop **and** the `RE_LOCATION_TASK` fallback).

**⚠️ Doc/code drift found**: `pages/src/content/docs/en/telemetry.md` states "LLM round trips and tool calls are not emitted as spans — they show up only in metrics." Source-level grep in this documentation pass found explicit `StartLLMSpan`/`StartToolSpan` call sites producing `llm.request` and `tool.execute.<toolName>` spans. **Treat the user-facing doc's claim as unverified/possibly stale** until directly reconciled — either the spans exist but are filtered/sampled out before export in some configuration, or the user doc needs correction. Do not resolve this discrepancy by assumption in either direction; confirm against a live trace before updating either document.

## Metric inventory (lazily registered, `internal/telemetry/metrics.go`)

`ocr.review.duration_seconds` (histogram) · `ocr.files_reviewed_total`, `ocr.comments_generated_total` (counters) · `ocr.llm.requests_total` (counter, attrs: model, status) · `ocr.llm.tokens_used` (counter, attrs: type=total, model) · `ocr.llm.request_duration_seconds` (histogram, attr: model) · `ocr.tool.calls_total` (counter, attrs: tool.name, status) · `ocr.tool.execution_duration_seconds` (histogram, attr: tool.name).

Metric registration errors are **silently swallowed by design** (`checkMetricErr` is an intentional no-op) — a broken metric registration degrades observability, not the review itself.

## Events

Discrete `event.<name>` spans (zero-duration) fire at decision points via `Event`/`Eventf`/`ErrorEvent` — full inventory in `pages/.../telemetry.md`.

## Non-telemetry progress output — do not conflate

`internal/telemetry/events.go` also drives plain-stdout progress printing (`[ocr] ▶ tool_name ...`) via `PrintTraceSummary`/`PrintToolCallStarted/Finished/Error`. **This is not gated by whether telemetry is enabled** — it's the ordinary human-readable progress output shown during any review/scan run, sharing a file with the OTel wiring but functionally independent of it.

## Content logging — confirmed dead code path

`ContentLogging()` re-resolves config via `ResolveConfig("")` (an **empty** config path) instead of `HomeConfigPath()`. Since config loading is skipped entirely when the path is empty, `telemetry.content_logging: true` in `~/.opencodereview/config.json` is **silently ignored** — only the `OCR_CONTENT_LOGGING=1` env var reaches the actual read site. This is worse than "reserved/plumbed but unused" as `pages/.../telemetry.md` frames it: it's specifically broken for the config-file input path. Regardless of this bug, telemetry never attaches prompt/response content to spans/events — that guarantee holds independent of the flag (see `docs/ai/MODEL_GUARDRAILS.md`).

## Known gaps / uncertainties:
- The `llm.request`/`tool.execute.<toolName>` span drift noted above is the single most important open item in this document — resolve before treating either source as authoritative.
- Whether `review.run`'s span-start call uses a literal string or an indirect constant/variable wasn't confirmed by a broad-enough grep.
