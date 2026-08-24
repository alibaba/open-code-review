# CLAUDE.md — Engineering Map: open-code-review (`ocr`)

Router: [`/DOC_INDEX.md`](DOC_INDEX.md). Agent/commit conventions: [`/AGENTS.md`](AGENTS.md). This file is a map, not an analysis — every claim below links to the doc that proves it. Do not add deep technical detail here; add it to the linked sub-document.

## 1. System Overview

`ocr` (module `github.com/alibaba/open-code-review`) is a Go CLI that runs an LLM tool-use agent over git diffs or whole files to produce line-anchored code-review comments. It originated as Alibaba's internal review assistant and was open-sourced. It is provider-agnostic (26+ built-in LLM presets + custom endpoints) and ships as a single static binary distributed via npm, GitHub Releases, and platform install scripts. Full detail: [`docs/architecture/SYSTEM_OVERVIEW.md`](docs/architecture/SYSTEM_OVERVIEW.md).

## 2. Repository Ownership and Boundaries

Five ownership zones, each with one authoritative package cluster — see [`docs/architecture/MODULE_OWNERSHIP.md`](docs/architecture/MODULE_OWNERSHIP.md) for the full table:

- **CLI surface** — `cmd/opencodereview/` (cobra command tree, flag parsing, output rendering).
- **AI orchestration** — `internal/agent/` (review), `internal/scan/` (full-file scan), `internal/llmloop/` (the shared tool-use loop both call into), `internal/llm/` (provider clients).
- **Deterministic support** — `internal/diff/`, `internal/config/rules/`, `internal/tool/`, `internal/pathutil/`, `internal/model/` (shared DTOs).
- **Persistence & UI** — `internal/session/` (JSONL + resume + manifest), `internal/viewer/` (read-only local HTTP UI).
- **Distribution & integration** — `plugins/`, `extensions/vscode/`, `npm/`, `examples/*_ci/`, `action.yml`.

## 3. High-Level Architecture

Not a service — a single CLI process per invocation, with three optional network-facing extensions: an outbound call to one resolved LLM provider, outbound connections to configured MCP servers (client only, never a server), and an opt-in loopback-only HTTP viewer. See [`docs/architecture/SERVICE_TOPOLOGY.md`](docs/architecture/SERVICE_TOPOLOGY.md) for the topology diagram and [`docs/architecture/DEPENDENCY_GRAPH.md`](docs/architecture/DEPENDENCY_GRAPH.md) for the module graph.

## 4. Entry Points

| Entry | Command | Detail |
|---|---|---|
| Process start | `cmd/opencodereview/main.go` | [`RUNTIME_DEPENDENCY_TREE.md`](docs/architecture/RUNTIME_DEPENDENCY_TREE.md) |
| Diff-based review | `ocr review` | [`RUNTIME_FLOWS.md#review`](docs/architecture/RUNTIME_FLOWS.md) |
| Full-file scan | `ocr scan` | [`RUNTIME_FLOWS.md#scan`](docs/architecture/RUNTIME_FLOWS.md) |
| No-LLM delegation | `ocr delegate preview\|rule` | [`docs/ai/AGENT_WORKFLOWS.md`](docs/ai/AGENT_WORKFLOWS.md) |
| Session inspection | `ocr session list\|show\|comments` | [`docs/architecture/DATA_CONTRACTS.md`](docs/architecture/DATA_CONTRACTS.md) |
| Local UI | `ocr viewer` | [`docs/architecture/SERVICE_TOPOLOGY.md`](docs/architecture/SERVICE_TOPOLOGY.md) |
| Config / provider setup | `ocr config *`, `ocr llm *` | `pages/src/content/docs/en/configuration.md` |
| Rule inspection | `ocr rules check` | `pages/src/content/docs/en/review-rules.md` |

Full command tree: [`docs/architecture/CALL_GRAPH.md`](docs/architecture/CALL_GRAPH.md).

## 5. Core Runtime Flow Summaries

- **Review**: resolve LLM endpoint (config → env → Claude Code env → rc files, single provider, no cross-provider fallback) → load diffs (Workspace/Commit/Range) → 5-gate file filter → per-file plan (optional) + main tool-use loop → comment line-resolution → review-filter pass → JSONL persistence → render (text/JSON/SARIF).
- **Scan**: enumerate files (git-tracked or filesystem walk) → batch (none/by-language/by-directory) → per-file plan + main loop (same `internal/llmloop.Runner` as review) → per-batch dedup → optional project-summary → persistence.
- **Delegate**: no LLM call at all — deterministic file-selection + rule-grouping only; an external host agent (Claude Code, QCA Forward, etc.) performs the actual review.
- **Resume**: review checks a strict identity hash (repo/diff-range/rule-config/provider-model) before reusing any checkpoint; scan reuses per-file by content fingerprint, no whole-run identity gate.

Full step-by-step with mermaid diagrams: [`docs/architecture/RUNTIME_FLOWS.md`](docs/architecture/RUNTIME_FLOWS.md).

## 6. High-Risk Areas

See [`docs/architecture/CHANGE_BLAST_RADIUS.md`](docs/architecture/CHANGE_BLAST_RADIUS.md) for the full analysis. Top five: `internal/model/*` (shared DTOs), `internal/session/persist.go` + `manifest.go` (JSONL schema consumed by resume, viewer, and every CI posting script), `internal/llmloop/loop.go` (shared loop under review *and* scan), embedded prompt templates (`internal/config/template/`), and `internal/llm/resolver.go` (endpoint precedence).

## 7. Documentation Map

Compressed router with the full domain table: [`/DOC_INDEX.md`](DOC_INDEX.md). Domains: `docs/architecture/` (11 docs), `docs/ai/` (6 docs), `docs/operations/` (5 docs), `docs/security/` (2 docs), `docs/integrations/` (1 doc), `docs/development/` (3 docs). User-facing usage docs live separately at `pages/src/content/docs/en/*.md` — engineering docs here link to them rather than duplicating.

## 8. Documentation Loading Guide

| Task | Load in order |
|---|---|
| General understanding | `DOC_INDEX.md` → this file → `SYSTEM_OVERVIEW.md` |
| Architecture tracing | this file → `REPOSITORY_MAP.md` → `RUNTIME_FLOWS.md` → `DEPENDENCY_GRAPH.md` |
| Runtime debugging | this file → `RUNTIME_FLOWS.md` → `OBSERVABILITY.md` → `FAILURE_MODES.md` → `CALL_GRAPH.md` |
| Schema/contract change | this file → `DATA_CONTRACTS.md` → `CHANGE_BLAST_RADIUS.md` |
| Integration change | this file → `EXTERNAL_INTEGRATIONS.md` → `SERVICE_TOPOLOGY.md` |
| Security review | this file → `SECURITY_MODEL.md` → `TRUST_BOUNDARIES.md` |
| Prompt/model change | this file → `AI_SYSTEM_MAP.md` → `PROMPTS.md` → `LLM_ARCHITECTURE.md` → `MODEL_GUARDRAILS.md` → `AI_PIPELINE_MAP.md` |
| Incident response | this file → `RUNBOOK.md` → `FAILURE_MODES.md` → `OPERATIONAL_FAILURE_GRAPH.md` |

## 9. Recommended Reading Order (first-time contributor)

1. `DOC_INDEX.md` + this file
2. `docs/architecture/SYSTEM_OVERVIEW.md` + `REPOSITORY_MAP.md`
3. `pages/src/content/docs/en/architecture.md` (deepest existing source for the review loop)
4. `docs/architecture/RUNTIME_FLOWS.md` (fills in scan/delegate/resume, not covered above)
5. `docs/development/LOCAL_DEVELOPMENT.md` → `TESTING_STRATEGY.md`
6. `docs/development/SAFE_CHANGE_ZONES.md` before your first PR

## 10. Rules for Safe Changes

- Read [`AGENTS.md`](AGENTS.md) first — it governs commit format, license headers, English-only source, and the 90% coverage gate (`make check`, `make test`).
- Any change to `internal/session/persist.go`, `manifest.go`, or the JSONL record shapes must bump the relevant `schema_version` (`ocr.run-manifest/v1`, `ocr.resume-lineage/v1`) — consumers (viewer, resume, CI posting scripts) key off it.
- Any change to `internal/config/toolsconfig/tools.json` or `internal/config/template/*.json`/prompt `.md` files changes model behavior for every user on the next build — there is no automated regression for prompt wording; verify manually with `ocr review --preview` and a real session before merging.
- Do not add a cross-provider fallback silently — the single-endpoint-per-run design ([`LLM_ARCHITECTURE.md`](docs/ai/LLM_ARCHITECTURE.md)) is deliberate ("no fallback" per `architecture.md`); changing it is a behavioral/security decision, not a bug fix.
- Full checklist: [`docs/development/SAFE_CHANGE_ZONES.md`](docs/development/SAFE_CHANGE_ZONES.md).

## 11. Known Unknowns

- Exact trigger/behavior of `runGraceRound` in `internal/llmloop/loop.go` — inferred from naming, not confirmed by direct read.
- Division of labor between `internal/agent/compression.go` and `internal/llmloop/compression.go` (two files with overlapping names) — unconfirmed which is authoritative.
- Whether npm's `optionalDependencies` per-platform packages or the `postinstall` direct GitHub-Releases download is the actual binary-acquisition path (evidence suggests both exist; unclear if one is vestigial).
- `content_logging` is dead via the config-file path (only the `OCR_CONTENT_LOGGING` env var reaches the read site) — confirmed by code, contradicts the "reserved" framing in `pages/.../telemetry.md`.
- Two independent implementations of CI-comment-posting logic (JS for GitHub Actions, Python ×4 for GitLab/Gerrit/GitFlic/Codeup) with the same retry/incremental/routing feature set — drift risk, not yet unified.

Each sub-document carries its own "Known gaps" section — treat this list as the cross-cutting subset.
