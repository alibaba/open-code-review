Parent document: `/CLAUDE.md`
Related documents:
- `docs/ai/AI_PIPELINE_MAP.md`
- `docs/ai/AGENT_WORKFLOWS.md`
- `docs/ai/MODEL_GUARDRAILS.md`

Read this when:
- You need the top-level map of where AI is (and deliberately isn't) in this system before diving into any single flow.

Purpose:
- Where AI is used, why, what depends on its output, and where the deterministic/AI boundary sits.

Scope:
- Included: all AI touchpoints across review, scan, and delegate; downstream dependents of AI output.
- Excluded: prompt content (`PROMPTS.md`), provider/client internals (`LLM_ARCHITECTURE.md`), safety mechanisms (`MODEL_GUARDRAILS.md`).

---

# AI System Map

## Two distinct AI flows, one shared engine

1. **`ocr review`** — diff-scoped. AI decides: what to comment on, where to anchor it (with a deterministic fallback), and re-anchoring when matching fails.
2. **`ocr scan`** — whole-file. Same per-file AI decisions as review, **plus** two scan-only AI stages: cross-file comment dedup (`DEDUP_TASK`) and an optional project-level summary (`PROJECT_SUMMARY_TASK`).

Both are driven by the same underlying tool-use loop, `internal/llmloop.Runner` — a prompt/provider change affects both flows identically. See `docs/architecture/RUNTIME_FLOWS.md` for the step sequences.

## Where AI is explicitly absent

**`ocr delegate`** makes zero LLM calls. Its package doc comment states this directly: it produces deterministic "review specifications" (file selection + resolved rule text) for an external agent to consume. This is the system's clearest deterministic/AI boundary — worth treating as a first-class architectural fact, not an edge case. The **QCA Forward** integration is the canonical consumer: an already-present host model performs the entire review; OCR contributes engineering (file filtering, rule resolution) only.

**MCP** is tool augmentation, not core AI — it extends what tools the *existing* review/scan model can call; it doesn't add a second model or a second decision-maker.

## Downstream dependents of AI output

| Consumer | Depends on |
|---|---|
| Line-number resolution | `existing_code` text matching model output against the diff (sliding-window, deterministic algorithm over AI-produced text) |
| `REVIEW_FILTER_TASK` | a second AI pass that reads the first pass's comments and removes provably-incorrect ones — **AI checking AI**, not independent validation |
| Session JSONL / `ocr viewer` | persists AI output verbatim, including raw prompts/responses |
| Renderers (text/JSON/SARIF) | render `LlmComment` structs built from AI tool calls |
| CI posting scripts (JS + Python ×4) | parse rendered JSON, route by AI-assigned `Category`/`Severity` |
| Scan's `PROJECT_SUMMARY_TASK` | reads **other AI output** (all collected comments) as its own input — a third-order AI dependency |

## Boundary between deterministic and AI-driven behavior

| Deterministic | AI-driven |
|---|---|
| File filtering (5-gate), rule resolution, batching, dedup-grouping in delegate mode | Which issues to flag, comment content, severity/category assignment |
| Sliding-window line matching (primary path) | `RE_LOCATION_TASK` re-anchoring (fallback path, when deterministic matching fails) |
| Memory-compression *triggering* (token-threshold math) | Memory-compression *content* (the summary itself is model-generated) |
| Manifest coverage bookkeeping, resume identity hashing | Cross-file dedup decisions (scan), project summary content (scan) |

## Known gaps / uncertainties:
- Whether `REVIEW_FILTER_TASK` runs against the *same* model/provider as the main loop, or could in principle use a different one, was not confirmed — current evidence (single resolved endpoint per run) implies same model.
