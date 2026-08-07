---
status: accepted
---
# Expose full codebase-memory context to OCR

## Context

OCR has a small built-in review toolset and already supports external MCP servers. Code review findings must remain anchored to the current diff, while some correctness, security, performance, and dependency claims require repository-level evidence about callers, callees, definitions, coverage, and architecture.

The codebase-memory MCP provides that structural context. A curated subset would hide capabilities that the review agent may need, while exposing the complete surface lets the agent choose the narrowest query for each claim. Tool availability and tool invocation are separate decisions: the complete surface is available, but the agent chooses calls based on the query.

## Decision

Configure OCR to expose the complete codebase-memory MCP tool surface to the review agent. The agent follows this query protocol when a structural or relationship question arises:

1. Check index readiness with `index_status`.
2. If the index is missing or stale, run `index_repository` once, then retry the status or query.
3. Select the relevant tool: `search_graph`, `trace_path`, `get_code_snippet`, `query_graph`, `get_architecture`, or `detect_changes`.
4. Use `check_index_coverage` for cited paths and for scopes behind negative or exhaustive claims.
5. Fall back to `search_code` or ordinary file inspection when structural tools are unavailable or coverage is insufficient, and state the limitation.

The agent may use the full MCP surface, but it does not call every tool for every file. Indexing belongs to the MCP query lifecycle and is not a mandatory per-file sequence. Destructive or maintenance operations require explicit user intent.

The review target remains the current diff. Structural context may justify a finding on changed code, but findings about untouched files are out of scope. OCR's built-in `code_search` and codebase-memory's `search_code` remain available together: the former handles direct text search, while the latter adds graph context.

## Alternatives considered

- **Expose only a curated read-only subset**: rejected because it prevents the agent from selecting tools required by less common structural questions.
- **Force a fixed tool sequence for every file**: rejected because it creates redundant indexing and query calls, increases latency and tokens, and treats all findings as equally structural.
- **Use only OCR's built-in file and text tools**: rejected because text search does not provide reliable caller, callee, dependency, or impact relationships.

## Consequences

- Agents can use the same structural-query workflow for callers, dependencies, impact analysis, and architecture questions.
- Index state and coverage become explicit evidence instead of hidden assumptions.
- The full MCP tool schema increases prompt and tool-selection cost; benchmark results must measure that cost against additional high-confidence cross-file findings.
- A missing or stale index can add initialization latency. The agent must initialize it once and then continue querying rather than repeating the operation per file.
- Full tool availability does not grant permission to perform destructive or maintenance actions without an explicit request.

## Validation

Evaluate the integration on a fixed set of diff reviews against the built-in-tool baseline. Record high-confidence non-local findings, false positives, wrong-scope findings, tool-call counts, token usage, wall-clock latency, and index initialization cost. Keep the integration only when the added structural evidence improves review quality without unacceptable cost or scope drift.
