# Scan template overrides (`--scan-template`)

`ocr scan --scan-template <file>` replaces the embedded whole-file scan
contract with your own prompt×tool×budget×sentinel contract. The file uses
the whole `scan_template.json` schema: `MAIN_TASK` messages and tools,
optional `PLAN_TASK` / `DEDUP_TASK` / `PROJECT_SUMMARY_TASK` messages,
`NO_PLAN_GUIDANCE`, and the budget scalars. CLI flags (`--max-tools`,
`--batch`, `--max-tokens-budget`, `--max-tokens`) apply on top of the file,
exactly as they do on the embedded default.

`bounded-function-review.json` is a minimal bounded-scope example: two
tools, function-scope wording, `PLAN_TASK` omitted (plan-less run), custom
no-plan text, and the default budgets. A test loads and validates it, so it
cannot rot.

## Authoring contract

An invalid file aborts the scan at startup with zero LLM requests. Rules:

- **Required placeholder**: at least one `MAIN_TASK` user-role message must
  contain `{{file_content}}`. Without it the file under review never reaches
  the model.
- **Known tokens only**: every `{{token}}` must belong to its task's known
  set; anything else fails closed. Known sets:
  - `MAIN_TASK`: `plan_guidance`, `current_system_date_time`,
    `current_file_path`, `system_rule`, `change_files`, `file_content`,
    `requirement_background`
  - `PLAN_TASK`: `current_system_date_time`, `current_file_path`,
    `system_rule`, `file_content`
  - `DEDUP_TASK`: `batch_comments`
  - `PROJECT_SUMMARY_TASK`: `comment_count`, `file_count`, `all_comments`
- **Tools live on `MAIN_TASK` only** and every name must exist in the tool
  catalog; unknown names fail closed. `PLAN_TASK` / `DEDUP_TASK` /
  `PROJECT_SUMMARY_TASK` must not carry `tools` or `tool_references`.
- **Guidance must match scope**: when planning is disabled or fails,
  `{{plan_guidance}}` renders `NO_PLAN_GUIDANCE` (or the built-in
  whole-file sentinel when the field is empty). A bounded scope needs its
  own text here, or the prompt contradicts itself.
- **Prose must not instruct unexposed tools**: the harness enforces that
  the model sees exactly your tool list, but only you can keep the prose
  coherent with it. If the template exposes `task_done` + `code_comment`
  only, the instructions must not ask for searches, reads, or finds.
- **Budgets stay explicit**: `MAX_TOKENS` and `MAX_TOOL_REQUEST_TIMES`
  must be positive. Lowering `MAX_TOKENS` surfaces as per-file skips, never
  silent truncation.

File hygiene: the path must exist, must not be a directory, must be valid
JSON, and must not exceed 1 MiB.
