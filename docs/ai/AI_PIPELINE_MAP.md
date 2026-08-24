Parent document: `/CLAUDE.md`
Related documents:
- `docs/ai/AI_SYSTEM_MAP.md`
- `docs/architecture/DATA_LINEAGE.md`
- `docs/operations/FAILURE_MODES.md`

Read this when:
- You need the single end-to-end diagram of the AI pipeline with every failure point marked.

Purpose:
- One consolidated pipeline view: context assembly → model call → validation → persistence → downstream usage, with failure points annotated.

Scope:
- Included: the full review-mode pipeline (scan's divergences noted inline).
- Excluded: per-component deep-dives (already covered by `PROMPTS.md`, `LLM_ARCHITECTURE.md`, `MODEL_GUARDRAILS.md` — this doc is the map that ties them together).

---

# AI Pipeline Map

```mermaid
flowchart TD
    A["git diff / file content"] --> B["rule resolution<br/>(4-layer, deterministic)"]
    B --> C["5-gate filter<br/>(deterministic)"]
    C -->|"❌ fails: binary/excluded/unsupported"| C1["file dropped, warning recorded"]
    C --> D["context assembly:<br/>template + placeholders"]
    D --> E{"diff/file size<br/>&gt; 80% MAX_TOKENS?"}
    E -->|yes| E1["❌ file skipped,<br/>token.threshold.exceeded"]
    E -->|no| F["PLAN_TASK (optional)"]
    F -->|"❌ LLM error"| F1["plan.failed — main loop runs without plan"]
    F --> G["MAIN_TASK loop<br/>up to MAX_TOOL_REQUEST_TIMES"]
    G -->|"❌ 3 empty rounds / cancelled /<br/>compression failure"| G1["loop exits early,<br/>MainLoopStop reason recorded"]
    G --> H["code_comment calls collected"]
    H --> I["line resolution<br/>(sliding window, deterministic)"]
    I -->|"❌ no match"| J["RE_LOCATION_TASK<br/>(AI re-anchor)"]
    J -->|"❌ still no match"| K["StartLine=0<br/>(unanchored, not dropped)"]
    I -->|match| L["LlmComment"]
    J -->|match| L
    K --> L
    L --> M["REVIEW_FILTER_TASK<br/>(post-loop AI validation)"]
    M -->|"❌ LLM error"| M1["errors logged, ignored —<br/>unfiltered comments kept"]
    M --> N["2nd line-resolution pass<br/>(cmd layer, cross-file)"]
    N --> O["session JSONL persist"]
    N --> P["render: text / JSON / SARIF"]
    O --> Q["ocr viewer"]
    P --> R["CI posting script<br/>(JS or Python)"]
```

## Scan-mode divergence (inserted between M and N)

```mermaid
flowchart LR
    A["per-batch comments"] --> B["DEDUP_TASK<br/>(AI, per batch)"]
    B -->|"❌ parse/LLM failure"| B1["best-effort: originals kept unchanged"]
    B --> C["checkpoint-safe persistence<br/>(cross-file groups split per-file)"]
    C --> D["PROJECT_SUMMARY_TASK<br/>(optional, whole-run)"]
    D -->|"❌ failure/empty input"| D1["silently skipped"]
```

## Failure-point summary (see `docs/operations/FAILURE_MODES.md` for recovery detail)

| Stage | Failure mode | Blast radius |
|---|---|---|
| Filter | file dropped | that file only; non-fatal |
| Token check | file skipped pre-call | that file only; reported as a warning |
| Plan | LLM error | that file's plan skipped, main loop still runs |
| Main loop | 3 empty rounds / cancelled / compression failure | that file's review incomplete, partial comments kept |
| Line resolution | no match | comment kept with `StartLine=0`, never dropped |
| Review filter | LLM error | filter pass skipped, **unfiltered** comments ship — a quality regression, not a crash |
| Scan dedup | LLM/parse failure | originals kept, best-effort only |
| Scan project summary | failure/empty | silently skipped, run still succeeds |
| Persistence | crash before flush | buffered `llm_request`/`llm_response`/`tool_call` detail lost; checkpoints (`review_item_*`) survive because they force-flush |

## Known gaps / uncertainties:
- Whether `REVIEW_FILTER_TASK` failures are surfaced anywhere visible to the end user (vs. purely logged) was not confirmed.
