Parent document: `/CLAUDE.md`
Related documents:
- `docs/architecture/REPOSITORY_MAP.md`
- `docs/architecture/SERVICE_TOPOLOGY.md`
- `docs/ai/AI_SYSTEM_MAP.md`

Read this when:
- You are new to the repo and need the "what is this and why does it exist" picture before touching code.
- You need to explain the project's positioning (vs. general-purpose coding agents) to a stakeholder.

Purpose:
- Establish system identity, users, capabilities, and architecture style at a level above any single flow.

Scope:
- Included: product purpose, users/systems served, capability inventory, architecture style classification.
- Excluded: step-by-step flows (`RUNTIME_FLOWS.md`), module boundaries (`REPOSITORY_MAP.md`), deployment mechanics (`operations/DEPLOYMENT.md`).

---

# System Overview

## What it is

`ocr` (`github.com/alibaba/open-code-review`) is a Go CLI that wraps an LLM in a scenario-tuned tool-use agent to perform automated code review. It originated as Alibaba's internal review assistant — the README states it served "tens of thousands of developers" and was incubated into an open-source project after internal validation. Its stated positioning against general-purpose coding agents (e.g. Claude Code) is: **higher precision/F1 on review at the same underlying model, ~1/9 the tokens, lower recall by design** — a deliberate trade favoring signal over noise (`README.md`).

## Who/what it serves

- **Individual developers** — running `ocr review`/`ocr scan` locally against their own working tree, `ocr viewer` to inspect past runs.
- **CI pipelines** — GitHub Actions (canonical `action.yml`), GitLab CI, Gerrit (Jenkinsfile), GitFlic, Codeup, Bitbucket Pipelines — each invokes the CLI and posts findings back to the code-review surface (PR/MR comments).
- **External coding agents** — via `ocr delegate` (deterministic file-selection/rule-resolution with no LLM call in OCR itself) and via MCP tool exposure — used by Claude Code, Codex, Cursor, OpenCode plugins, and the QCA Forward integration.
- **Operators/SREs** — via OpenTelemetry export (traces + metrics) for run-level observability.
- **LLM providers** — 26+ built-in presets (Anthropic, OpenAI, Bedrock, Gemini, DeepSeek, and a long tail of Chinese-market and aggregator providers) plus any custom OpenAI/Anthropic-compatible endpoint.

## Major capabilities

1. **`ocr review`** — diff-scoped review (workspace / commit / branch-range modes).
2. **`ocr scan`** — whole-file review with no git history requirement, batching, cross-file dedup, and an optional project-level summary.
3. **`ocr delegate`** — LLM-free file-selection and rule-resolution "spec" generation for host agents that perform the review themselves.
4. **`ocr viewer`** — read-only local HTTP UI over session JSONL transcripts.
5. **MCP client** — augments the built-in six-tool catalogue with tools from external MCP servers (stdio subprocess or remote HTTP).
6. **Session resume** — review has a strict identity-gated resume (repo/diff-range/rule/model must match); scan has per-file content-fingerprint reuse.
7. **Multi-provider LLM support** — 4 wire protocols (`anthropic`, `openai`, `openai-responses`, `anthropic-bedrock`) behind one client interface, single endpoint resolved per run, no cross-provider fallback.
8. **Structured output** — text, JSON (`--audience agent`), and SARIF v2.1.0 (unused by any shipped integration — see `docs/integrations/EXTERNAL_INTEGRATIONS.md`).
9. **Layered rule system** — CLI override → project config → global config → embedded system defaults, glob-matched per file, resolved into the prompt.
10. **OpenTelemetry observability** — opt-in spans/metrics/events, prompt content deliberately excluded from telemetry.

## Architecture style

Single-process CLI, not a service. No database — append-only JSONL is the only persistence. No message queue. No long-running server except the opt-in, loopback-bound `ocr viewer`. The "agent" is a bounded tool-use conversation loop (`internal/llmloop.Runner`) shared by both review and scan, fanned out per-file with a concurrency-limited goroutine pool. External state is limited to: the local git repository, the local filesystem (config, sessions), one resolved LLM provider endpoint per run, and optionally configured MCP server subprocesses/endpoints. See `docs/architecture/SERVICE_TOPOLOGY.md` for the full topology and `docs/ai/AI_SYSTEM_MAP.md` for where AI-driven behavior stops and deterministic code takes over.

## Known gaps / uncertainties:
- No formal ADRs exist in the repo; architectural intent above is reconstructed from `README.md`, `ASSURANCE_CASE.md`, and source inspection, not a design document.
- Precision/F1/token-savings claims in `README.md` are asserted, not independently verified in this documentation pass.
