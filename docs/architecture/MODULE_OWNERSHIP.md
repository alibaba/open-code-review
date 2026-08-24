Parent document: `/CLAUDE.md`
Related documents:
- `docs/architecture/REPOSITORY_MAP.md`
- `docs/architecture/CHANGE_BLAST_RADIUS.md`

Read this when:
- You need to know exactly which package is the single source of truth for a concern before adding logic elsewhere.

Purpose:
- One canonical owner per cross-cutting concern, to prevent logic from being duplicated across packages.

Scope:
- Included: ownership mapping for the concerns explicitly called out in the doc system spec (API handling, business logic, persistence, AI orchestration, integrations, worker logic, validation, config, observability).
- Excluded: file-by-file responsibilities within a package (see inline code / `REPOSITORY_MAP.md`).

---

# Module Ownership

| Concern | Owning package | Notes |
|---|---|---|
| CLI/command handling | `cmd/opencodereview/` | cobra tree, flag parsing, all output rendering (text/JSON/SARIF) |
| Review orchestration | `internal/agent/` | bootstrap, 5-gate filter, per-file dispatch, review-only compression glue |
| Scan orchestration | `internal/scan/` | enumeration, batching, dedup, project summary |
| Shared AI loop ("worker logic") | `internal/llmloop/` | the actual tool-use conversation runner both orchestrators call |
| LLM transport/provider logic | `internal/llm/` | endpoint resolution, protocol clients, retry classification |
| Tool implementations | `internal/tool/` | `file_read`, `code_search`, `file_find`, `file_read_diff`, `code_comment`, `task_done` |
| MCP integration | `internal/mcp/` | client only — connects out, merges tools into the same registry |
| Delegation integration | `internal/delegate/` | deterministic file-selection + rule formatting, zero LLM calls |
| Diff parsing | `internal/diff/` | git-mode diff loading, line-resolution matcher, `RE_LOCATION_TASK` glue |
| Git subprocess execution | `internal/gitcmd/` | the *only* place `git` is invoked as a subprocess |
| Path/traversal validation | `internal/pathutil/` | `WithinBase()` — the sole path-safety primitive, used by every file tool |
| Rule resolution | `internal/config/rules/` | 4-layer chain, embedded defaults |
| Prompt/tool/config templates | `internal/config/` (`template/`, `toolsconfig/`, `allowlist/`) | embedded at build time via `//go:embed` |
| Persistence | `internal/session/` | JSONL write/read, resume identity, run manifest |
| Read-only UI | `internal/viewer/` | HTTP server, host-guard, security headers |
| Observability | `internal/telemetry/` | OTel spans/metrics, config resolution, console progress printing (a separate, non-gated channel) |
| Shared data contracts | `internal/model/` | `Diff`, `LlmComment`, `CodeReviewResult` — the only package every AI-producing/consuming package agrees on |
| Output writer indirection | `internal/stdout/` | not a formatter — a swappable `io.Writer` singleton |
| Distribution (npm) | `npm/`, `scripts/install.js`, `scripts/publish/` | binary acquisition/publishing, not part of the Go build |
| CI integration contract | `action.yml`, `scripts/github-actions/post-review-comments.js` | canonical GitHub Action + its comment-posting logic |
| Other-CI integration contracts | `examples/*_ci/*/post_review.py` | duplicated (not shared) posting logic per platform — see `CHANGE_BLAST_RADIUS.md` |
| Coding-agent plugin surface | `plugins/open-code-review/`, `skills/` | slash commands / skill definitions that shell out to the `ocr` binary |
| VS Code UI | `extensions/vscode/` | thin subprocess wrapper, not a logic duplicate |

## Not a runtime component

`internal/release/` contains only a CI-consistency test (asset-naming cross-check against `package.json`) — it has no caller in the running binary and should not be treated as owning any runtime concern.

## Known gaps / uncertainties:
- Whether `internal/suggestdiff/` is owned by `internal/tool` (as a helper) or is an independent concern with its own call sites was not fully resolved.
