Parent document: `/CLAUDE.md`
Related documents:
- `docs/ai/PROMPTS.md`
- `docs/architecture/CHANGE_BLAST_RADIUS.md`
- `pages/src/content/docs/en/tools.md` (tool I/O, user-facing)
- `pages/src/content/docs/en/viewer.md` (JSONL walkthrough, user-facing)

Read this when:
- You're changing a struct, JSON schema, or JSONL record shape and need to know every consumer.
- You're building a new integration that parses `ocr` output.

Purpose:
- Enumerate every schema that crosses a package or process boundary, its producer(s)/consumer(s), and compatibility notes.

Scope:
- Included: core Go DTOs, JSONL session event schema, tool call schemas, delegate/SARIF output, MCP tool mapping.
- Excluded: prompt template placeholder substitution (`docs/ai/PROMPTS.md`), config file keys (`pages/.../configuration.md`).

---

# Data Contracts

## Core Go DTOs (`internal/model/`)

**`Diff`** (`diff.go`) — produced by `internal/diff` (git provider), consumed by `internal/agent` (filtering, prompt assembly), `internal/llmloop`:
`{OldPath, NewPath, Diff string, NewFileContent string, IsBinary, IsDeleted, IsNew, IsRenamed bool, Insertions, Deletions int}`

**`LlmComment`** (`review.go`) — the AI output contract. Produced by the main tool-use loop (from `code_comment` tool calls), consumed by line-resolution, review-filter, session persistence, and every renderer (text/JSON/SARIF) plus every downstream CI posting script:
`{Path, Content, SuggestionCode, ExistingCode string, StartLine, EndLine int, Thinking string, Category enum, Severity enum}`
- `Category`: bug / security / performance / maintainability / test / style / documentation / other.
- `Severity`: critical / high / medium / low.
- `StartLine == 0` is the **implicit** "unanchored comment" signal — there is no dedicated boolean field; every consumer must check the line value, not a flag.
- `Thinking` is populated only when the model emits reasoning content; deliberately **not** in the schema advertised to the model (`tools.json` omits it) — a runtime-only, best-effort field.

**`CodeReviewResult`** — the pre-line-resolution raw form (`RelevantFile, SuggestionContent, ExistingCode, SuggestionCode`), converted into `LlmComment` once line resolution runs.

## Tool schemas (`internal/config/toolsconfig/tools.json`)

Full I/O documented in `pages/src/content/docs/en/tools.md` — the tools are the model-facing contract. Key correctness note for this doc: **`code_comment`'s `existing_code` matching is a prompt-engineered contract, not a validated schema field.** The tool description tells the model the snippet must exist verbatim (whitespace-insensitive) in the diff; there is no schema-level enforcement — correctness depends on the model following free-text instructions, backstopped by the sliding-window matcher + `RE_LOCATION_TASK` fallback + `StartLine==0` degrade path. See `docs/ai/MODEL_GUARDRAILS.md`.

## Session JSONL (`~/.opencodereview/sessions/<encoded-repo>/<session-id>.jsonl`)

Append-only, one JSON object per line, every record shares `{uuid, parentUuid, type, sessionId, timestamp}` (RFC3339 UTC). `parentUuid` forms an informational hash-chain-like lineage — **not cryptographically verified**.

| `type` | Key fields | Notes |
|---|---|---|
| `session_start` | `cwd`, `gitBranch`, `model`, `reviewMode`, `diffFrom`/`diffTo`/`diffCommit` (review), `scanPaths` (scan), `resumedFrom` | first record |
| `review_item_done` / `_reused` / `_failed` | `filePath`, `oldPath`, `newPath`, `fingerprint`, `model`; `done`/`reused` add `comments: []LlmComment`; `reused` adds `sourceSessionId`; `failed` adds `error` | **doubles as the resume checkpoint index** — flushed immediately, not buffered |
| `llm_request` | `filePath`, `taskType`, `request_no`, `messages` | **raw prompt content persisted to disk** — unlike telemetry, which never logs content |
| `llm_response` | `taskType`, `content`, `tool_calls`, `duration_ms`, `usage{prompt_tokens, completion_tokens, cache_read_tokens, cache_write_tokens}` | |
| `llm_error` | `taskType`, `request_no`, `error`, `duration_ms` | |
| `tool_call` | `tool_name`, `arguments`, `result`, `ok`, `duration_ms` | |
| `resume_lineage` | `schema_version: "ocr.resume-lineage/v1"`, `run_id`, `parent_run_id`, `source_provider`/`source_model`, `target_provider`/`target_model` | one per resumed run, flushed immediately |
| `session_end` | `files_reviewed: []string`, `duration_seconds`, `llm_failures`, optional `run_manifest` (review only) | **terminal record; closes the file** |

**Durability**: `review_item_*` and `resume_lineage` force-flush on write; `llm_request`/`llm_response`/`llm_error`/`tool_call` are buffered and only flushed on normal exit. A crash mid-run can lose buffered transcript detail but never a checkpoint — resume correctness is preserved even when human-debugging detail has gaps. See `docs/operations/FAILURE_MODES.md`.

**Schema overload to know about**: scan's `DEDUP_TASK`/`PROJECT_SUMMARY_TASK` LLM calls are logged under `taskType: memory_compression_task` (no dedicated enum value exists for them yet), using synthetic path keys `__scan_dedup_batch_N__` / `__scan_project_summary__`. A consumer switching on `taskType` will see `memory_compression_task` records that are not actually memory compression.

## Run manifest (`internal/session/manifest.go`, review only)

`schema_version: "ocr.run-manifest/v1"`. `Coverage` has five **disjoint** sets: `Selected` (denominator, frozen via `SealSelected`), `Completed`, `Reused`, `Failed`, `Waived` — each holding `CoverageItem{item_id, path, old_path, fingerprint, classification, reason}`. `item_id` is **content-independent** (stable across a resume chain); `fingerprint` is content-dependent (used for checkpoint cross-reference). `sanitizeReason` redacts URL userinfo, bearer/basic tokens, and `key: value`-style credential assignments from every stored reason string — a defense-in-depth floor, explicitly documented as not a substitute for caller-side redaction. The manifest is embedded verbatim into `session_end.run_manifest` **and** is the same object serialized into the CLI's JSON output — "so they can never compute coverage differently."

**`ocr scan` does not produce a run manifest** — `Agent.RunManifest()` returns nil by design; scan predates the manifest system and uses per-file fingerprint reuse instead (weaker, best-effort identity check vs. review's atomic whole-run identity gate).

## Delegate mode output (`internal/delegate`)

`ocr delegate preview --format json` / `ocr delegate rule --format json` both carry `schema_version: "1"` — the compatibility signal for host-agent integrations (QCA Forward, plugin skills). A schema-version bump is the contract-break marker.

## SARIF output (`cmd/opencodereview/sarif.go`)

Full SARIF v2.1.0 implementation exists (`--format sarif`), rejected in combination with `--preview`. **No shipped integration (GitHub Action, any CI example) actually wires this into `github/codeql-action/upload-sarif` or any code-scanning surface** — a real, documented capability with zero production usage today.

## MCP tool mapping (`internal/mcp`)

External MCP `Tool{name, description, input schema}` → `llm.ToolDef{Type:"function", Function:{Name, Description, Parameters}}`. Tool call results are flattened to text; non-text content types (images, embedded resources) degrade to a `[unsupported content type: %T]` stub — **silent data loss** for any MCP server returning rich content.

## Known gaps / uncertainties:
- `internal/viewer/store.go` (623 lines) parses the JSONL into view-models — its exact field-by-field mapping was not verified line-by-line against the schema above; treat the table as the source of truth over any viewer-internal type name.
- Whether `LoadResumeState` (strict, fails hard on any bad line) has any live caller, versus the tolerant `LoadReviewResumeState`, is unconfirmed.
