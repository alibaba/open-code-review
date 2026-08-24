Parent document: `/CLAUDE.md`
Related documents:
- `docs/architecture/MODULE_OWNERSHIP.md`
- `docs/architecture/DEPENDENCY_GRAPH.md`

Read this when:
- You need to find where a concern lives before making a change.
- You are onboarding and need a directory-by-directory map with a reading order.

Purpose:
- Map every top-level directory to what it does and how it relates to the rest of the system — not a folder listing, a purpose map.

Scope:
- Included: every directory that contains runtime code, build/CI tooling, or a distinct product surface.
- Excluded: per-file responsibilities inside `internal/*` (see `MODULE_OWNERSHIP.md`), call sequencing (see `CALL_GRAPH.md`).

---

# Repository Map

## Runtime Go code

| Path | What it does | Depends on |
|---|---|---|
| `cmd/opencodereview/` | CLI entry point, cobra command tree, flag parsing, output rendering (text/JSON/SARIF), config/provider TUI | everything below |
| `internal/agent/` | `ocr review` orchestrator: bootstrap, 5-gate file filter (`preview.go`), per-file dispatch, memory-compression glue | `internal/llmloop`, `internal/diff`, `internal/config`, `internal/model` |
| `internal/scan/` | `ocr scan` orchestrator: file enumeration, batching, dedup, project-summary — reuses the same tool-use loop as review | `internal/llmloop`, `internal/gitcmd`, `internal/config` |
| `internal/llmloop/` | The shared tool-use conversation loop (`Runner.RunPerFile`) both `agent` and `scan` call into; token accounting; three-zone memory compression; comment worker pool | `internal/llm`, `internal/tool`, `internal/model` |
| `internal/llm/` | Endpoint resolution (config→env→Claude-Code-env→rc files), 4-protocol client abstraction, 26-provider preset registry, retry classification/telemetry | `internal/config` |
| `internal/model/` | Shared data types (`Diff`, `LlmComment`, `CodeReviewResult`) — the contract every other package reads/writes | (leaf — no internal deps) |
| `internal/diff/` | Git diff loading (Workspace/Commit/Range modes), line-resolution sliding-window matcher, `RE_LOCATION_TASK` fallback | `internal/gitcmd`, `internal/model` |
| `internal/gitcmd/` | Thin, injectable wrapper around `git` subprocess execution (hardcoded subcommands, no shell) | — |
| `internal/tool/` | Built-in tool implementations (`file_read`, `code_search`, `file_find`, `file_read_diff`, `code_comment`, `task_done`) | `internal/pathutil`, `internal/gitcmd` |
| `internal/pathutil/` | `WithinBase()` — path-traversal guard used by every file-touching tool | — |
| `internal/mcp/` | MCP **client** — connects to external MCP servers (stdio or remote HTTP), merges their tools into the same registry as built-ins | `internal/tool` |
| `internal/delegate/` | Deterministic, LLM-free rule-grouping + Markdown/JSON formatting for delegation mode | `internal/config/rules` |
| `internal/session/` | JSONL persistence, resume-identity checks, run-manifest coverage tracking, `ocr session list/show` | `internal/model` |
| `internal/viewer/` | Read-only embedded HTTP server rendering session JSONL as a browsable UI; host-guard + security-header middleware | `internal/session` |
| `internal/stdout/` | Tiny mutex-guarded writer-indirection singleton (not a formatter) | — |
| `internal/suggestdiff/` | Suggested-diff generation/validation support for `code_comment` suggestions | `internal/diff` |
| `internal/telemetry/` | OpenTelemetry SDK wiring (traces + metrics), config resolution, console progress printing | — |
| `internal/config/` | Config file load/precedence, embedded template/tools/rules assets (`//go:embed`), allowlists | (leaf) |
| `internal/release/` | **Not a runtime package** — contains only a CI-consistency test cross-checking release asset naming | — |

## Non-Go surfaces

| Path | What it does |
|---|---|
| `pages/` | Astro-based docs website (`open-codereview.ai/docs`) — the authoritative **user-facing** documentation; deployed via `deploy-pages.yml`. |
| `plugins/open-code-review/` | Four coding-agent plugin integrations (Claude Code, Codex, Cursor, OpenCode) plus `qca/` (QCA Forward delegation template) and shared `skills/`. |
| `extensions/vscode/` | VS Code extension — thin subprocess wrapper around the `ocr` binary (spawns it via `child_process.spawn`, parses stdout/JSON); not a logic duplicate. |
| `npm/<platform>/` | Six per-platform npm package manifests for binary distribution (`darwin/linux/win32` × `arm64/x64`). |
| `examples/` | CI integration examples: `github_actions`, `gitlab_ci`, `gerrit_ci`, `gitflic_ci`, `codeup_ci`, `bitbucket_pipelines` — each a runnable reference for wiring `ocr` into that CI system. |
| `scripts/` | Build/release/compliance tooling: license-header injection, English-only enforcement, action-pin verification, npm publish scripts, translation-sync check. |
| `skills/` | Top-level copies of the delegation/review skill definitions shared across plugin integrations. |
| `action.yml` | The canonical, reusable GitHub Action — the authoritative CI integration contract; `examples/github_actions/` is a thin caller of it. |

## Where to start reading

1. `cmd/opencodereview/main.go` → `root.go` (entry point, command tree).
2. `pages/src/content/docs/en/architecture.md` (the deepest existing narrative of the review loop).
3. `internal/agent/agent.go` and `internal/llmloop/loop.go` (the two files that *are* the review engine).
4. `internal/scan/agent.go` for the second AI flow.
5. `internal/session/persist.go` + `manifest.go` for what gets written to disk and how resume works.

## Known gaps / uncertainties:
- `internal/release/`'s relationship to the actual release pipeline (does anything else consume it beyond the test itself?) was not traced beyond confirming it's test-only.
- `skills/` vs. `plugins/open-code-review/skills/` — appear to be duplicated/synced content; the sync mechanism (if any) was not verified.
