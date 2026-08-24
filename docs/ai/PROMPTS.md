Parent document: `/CLAUDE.md`
Related documents:
- `docs/ai/AI_SYSTEM_MAP.md`
- `docs/architecture/DATA_CONTRACTS.md`
- `docs/architecture/CHANGE_BLAST_RADIUS.md`

Read this when:
- You're changing prompt wording or adding a new task type.
- You need to know exactly what placeholder data a given prompt receives.

Purpose:
- Full prompt inventory: location, purpose, inputs, expected output, versioning.

Scope:
- Included: all task templates in `internal/config/template/`.
- Excluded: the tool schemas the model can call (`docs/architecture/DATA_CONTRACTS.md`), retry/provider mechanics (`LLM_ARCHITECTURE.md`).

---

# Prompt Inventory

All prompts are embedded into the binary via `//go:embed` at `internal/config/template/` — **changing a prompt requires a rebuild**, not a config change (`--tools` overrides the tool *registry*, not the template). See `docs/architecture/CHANGE_BLAST_RADIUS.md` for why this makes prompt changes high-risk and hard to regression-test.

## Review prompts (`task_template.json`, 5 tasks × system/user `.md` pairs = 10 files)

| Task | Purpose | Key inputs | Output |
|---|---|---|---|
| `PLAN_TASK` | Produce a checklist for large diffs before the main loop | `{{system_rule}}`, `{{diff}}`, `{{plan_tools}}` (read-only tool subset as text) | freeform checklist text → becomes `{{plan_guidance}}` |
| `MAIN_TASK` | Drive the tool-use review loop | `{{system_rule}}`, `{{change_files}}`, `{{diff}}`, `{{current_file_path}}`, `{{plan_guidance}}`, `{{requirement_background}}`, `{{current_system_date_time}}` | `code_comment` / `task_done` / context-tool calls |
| `MEMORY_COMPRESSION_TASK` | Summarize the "compress zone" of a long conversation | `{{context}}` (XML-rendered old messages) | plain-text summary, appended to the conversation as `<previous_review_summary>` |
| `REVIEW_FILTER_TASK` | Remove provably-incorrect comments after the main loop | `{{path}}`, `{{comments}}` (JSON) | filtered comment set |
| `RE_LOCATION_TASK` | Re-anchor a comment whose `existing_code` couldn't be matched | `{diff}`, `{existing_code}`, `{suggestion_content}` — **single-brace syntax, the one exception to `{{double-brace}}`** | re-anchored snippet or failure (original preserved) |

`MAIN_TASK`'s system prompt defines the reviewer persona and a **Strict Focus Rules** section: context tools (`file_read`, `file_read_diff`, `file_find`, `code_search`) are understanding-only — issues found while using them must never become comments. This is a prompt-level behavioral constraint, not a technically enforced one (see `docs/ai/MODEL_GUARDRAILS.md`). A **Reply limit** section mandates the model call exactly one of `task_done`/`code_comment`/a context tool per turn.

## Scan prompts (`scan_template.json`) — parallel, not identical to review

A second, distinct prompt set exists for `ocr scan`, confirmed to exist but **not fully enumerated in this documentation pass**. It reuses the same `PLAN_TASK`/`MAIN_TASK` shape for the per-file loop and additionally covers the two scan-only stages (`DEDUP_TASK`, `PROJECT_SUMMARY_TASK` — names inferred from the calling code in `internal/scan/agent.go`, not confirmed against the template file directly). Treat this section as a placeholder for a follow-up pass — do not assume scan's prompts are identical to review's beyond the shared loop mechanics.

## Connectivity test prompt

`internal/config/testconnection/task.json` — a minimal prompt used by `ocr llm test` to verify an endpoint end-to-end. Confirmed to exist; contents not read.

## Constants that gate prompt behavior (config-file overridable, not hardcoded)

`MAX_TOKENS` (default 58,888), `MAX_TOOL_REQUEST_TIMES` (default 30), `PLAN_MODE_LINE_THRESHOLD` (default 50), `MAX_TOKENS_BUDGET` (optional aggregate cap) — all JSON-tagged fields on the template struct, meaning `ocr config set max_tokens ...` genuinely changes loop behavior without a rebuild, unlike prompt wording itself.

## Prompt contracts and versioning

There is **no explicit prompt version number**. The only versioning signals in the system are the JSONL `taskType` enum values themselves (`plan_task`, `main_task`, `memory_compression_task`, `review_filter_task`, `re_location_task`) — a new task type requires a new enum value and updates to every consumer that switches on it (viewer's five task-type lanes, per `viewer.md`). Output schema correctness for `code_comment` is entirely prompt-engineered (see `docs/architecture/DATA_CONTRACTS.md`'s note on `existing_code`), not schema-validated beyond the SDK-level function-calling JSON shape.

## Known gaps / uncertainties:
- `scan_template.json`'s exact prompt file names, task-type enum values, and system/user prompt content were not read in this pass — flagged as the single largest gap in this document.
- `internal/config/testconnection/task.json` contents unread.
