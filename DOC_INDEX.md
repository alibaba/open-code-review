# DOC_INDEX — open-code-review (`ocr`)

Compressed router. Load this first, then `/CLAUDE.md`, then only the sub-docs your task needs. Full engineering map: [`/CLAUDE.md`](CLAUDE.md). Agent/commit conventions: [`/AGENTS.md`](AGENTS.md).

## What this repo is

`ocr` — a Go CLI that runs an LLM tool-use agent over `git diff`s (`review`) or whole files (`scan`) to produce inline code-review comments. Also: a delegation mode (`delegate`) that hands file-selection/rule-resolution to an external coding agent with no LLM call of its own, a local session viewer (`viewer`), an MCP client, 26+ built-in LLM provider presets, and companion distribution (npm, VS Code extension, CI examples).

## Authoritative document per domain

| Domain | Authoritative doc | Also see (existing, reused) |
|---|---|---|
| System purpose, architecture style | `docs/architecture/SYSTEM_OVERVIEW.md` | `README.md` |
| Directory/module map | `docs/architecture/REPOSITORY_MAP.md` | — |
| Step-by-step runtime flows | `docs/architecture/RUNTIME_FLOWS.md` | `pages/src/content/docs/en/architecture.md` (deepest source) |
| Module dependency graph | `docs/architecture/DEPENDENCY_GRAPH.md` | — |
| Entry-point call chains | `docs/architecture/CALL_GRAPH.md` | — |
| Struct/schema contracts | `docs/architecture/DATA_CONTRACTS.md` | `pages/.../tools.md`, `pages/.../viewer.md` |
| Process/service topology | `docs/architecture/SERVICE_TOPOLOGY.md` | — |
| Startup & runtime dependencies | `docs/architecture/RUNTIME_DEPENDENCY_TREE.md` | — |
| Data origin → storage → consumption | `docs/architecture/DATA_LINEAGE.md` | — |
| Who owns what code | `docs/architecture/MODULE_OWNERSHIP.md` | — |
| Risk of changing a module | `docs/architecture/CHANGE_BLAST_RADIUS.md` | — |
| Where/why AI is used | `docs/ai/AI_SYSTEM_MAP.md` | — |
| Prompt inventory & contracts | `docs/ai/PROMPTS.md` | — |
| LLM client/provider/retry internals | `docs/ai/LLM_ARCHITECTURE.md` | `pages/.../configuration.md` |
| Agent loop workflows (review/scan/delegate/MCP) | `docs/ai/AGENT_WORKFLOWS.md` | — |
| Injection/guardrail/output-validation | `docs/ai/MODEL_GUARDRAILS.md` | `ASSURANCE_CASE.md` |
| End-to-end AI pipeline w/ failure points | `docs/ai/AI_PIPELINE_MAP.md` | — |
| Day-2 operating tasks | `docs/operations/RUNBOOK.md` | — |
| Telemetry (spans/metrics) | `docs/operations/OBSERVABILITY.md` | `pages/.../telemetry.md` (user-facing) |
| Failure scenarios & recovery | `docs/operations/FAILURE_MODES.md` | — |
| Distribution/release/CI deploy | `docs/operations/DEPLOYMENT.md` | — |
| Failure propagation graph | `docs/operations/OPERATIONAL_FAILURE_GRAPH.md` | — |
| Auth/secrets/trust model | `docs/security/SECURITY_MODEL.md` | `ASSURANCE_CASE.md`, `SECURITY.md` (primary sources) |
| Trust zones & ingress/egress | `docs/security/TRUST_BOUNDARIES.md` | `ASSURANCE_CASE.md` |
| Third-party integrations | `docs/integrations/EXTERNAL_INTEGRATIONS.md` | `pages/.../mcp.md` |
| Local dev setup | `docs/development/LOCAL_DEVELOPMENT.md` | `CONTRIBUTING.md` (primary source) |
| Test strategy & coverage gate | `docs/development/TESTING_STRATEGY.md` | `AGENTS.md` |
| Safe vs. risky change zones | `docs/development/SAFE_CHANGE_ZONES.md` | `docs/architecture/CHANGE_BLAST_RADIUS.md` |

`pages/src/content/docs/en/*.md` is the **user-facing** docs site (installation, CLI reference, quickstart, FAQ) — authoritative for *how to use* `ocr`. The tree under `docs/` here is **engineering-facing** — authoritative for *how it works internally* and *how to safely change it*. Where both exist, engineering docs link to the user-facing page rather than repeating it.

## Task-based routing

| Task | Load |
|---|---|
| General understanding | `DOC_INDEX.md` → `CLAUDE.md` → `docs/architecture/SYSTEM_OVERVIEW.md` |
| Trace how review/scan works | `CLAUDE.md` → `REPOSITORY_MAP.md` → `RUNTIME_FLOWS.md` → `DEPENDENCY_GRAPH.md` |
| Debug a runtime issue | `CLAUDE.md` → `RUNTIME_FLOWS.md` → `docs/operations/OBSERVABILITY.md` → `FAILURE_MODES.md` → `CALL_GRAPH.md` |
| Change a schema (JSONL, tool schema, manifest) | `CLAUDE.md` → `DATA_CONTRACTS.md` → `CHANGE_BLAST_RADIUS.md` |
| Add/change an integration (CI, MCP, plugin) | `CLAUDE.md` → `docs/integrations/EXTERNAL_INTEGRATIONS.md` → `docs/architecture/SERVICE_TOPOLOGY.md` |
| Security review | `CLAUDE.md` → `docs/security/SECURITY_MODEL.md` → `TRUST_BOUNDARIES.md` → `ASSURANCE_CASE.md` |
| Change a prompt or model behavior | `CLAUDE.md` → `docs/ai/AI_SYSTEM_MAP.md` → `PROMPTS.md` → `LLM_ARCHITECTURE.md` → `MODEL_GUARDRAILS.md` → `AI_PIPELINE_MAP.md` |
| Production/CI incident | `CLAUDE.md` → `docs/operations/RUNBOOK.md` → `FAILURE_MODES.md` → `OPERATIONAL_FAILURE_GRAPH.md` |
| Onboard as a contributor | `CLAUDE.md` → `docs/development/LOCAL_DEVELOPMENT.md` → `TESTING_STRATEGY.md` → `SAFE_CHANGE_ZONES.md` |

## High-risk areas (load `CHANGE_BLAST_RADIUS.md` before touching)

`internal/model/*` (shared DTOs, everything depends on them) · `internal/session/persist.go` + `manifest.go` (JSONL schema — resume, viewer, CI posting scripts all depend on it) · `internal/llmloop/loop.go` (the shared tool-use loop under both `review` and `scan`) · `internal/config/template/*.json` + prompt `.md` files (embedded at build time — changes model behavior repo-wide with no automated regression) · `internal/config/toolsconfig/tools.json` (malformed schema breaks tool-calling for every provider) · `internal/llm/resolver.go` (endpoint precedence — silent behavior change for every user).

## Do not load unless needed

Localized docs (`README.*.md`, `CONTRIBUTING.*.md`, `pages/.../{ja,ru,zh}/*`), `.agents/`, `npm/*/package.json` (6 near-identical platform manifests), `internal/config/rules/rule_docs/*.md` (40+ per-language rule bodies — only load the one language you're touching).

## Known unknowns (see individual docs' "Known gaps" for detail)

Two-implementation CI-comment-posting risk (JS vs. Python) · `content_logging` config-file path is dead (env var works, config key doesn't) · exact behavior of `runGraceRound` in `llmloop/loop.go` unconfirmed by direct read · division of labor between `internal/agent/compression.go` and `internal/llmloop/compression.go` unconfirmed · whether npm `optionalDependencies` platform packages or the postinstall GitHub-Releases download is the actual binary source (or both) unconfirmed.
