Parent document: `/CLAUDE.md`
Related documents:
- `docs/ai/AI_SYSTEM_MAP.md`
- `docs/architecture/RUNTIME_FLOWS.md`
- `docs/integrations/EXTERNAL_INTEGRATIONS.md`

Read this when:
- You need to understand a specific agent workflow's trigger, tool access, and human-in-the-loop points.

Purpose:
- Every distinct agent workflow in the system: what triggers it, what it can do, and where (if anywhere) a human approves its output.

Scope:
- Included: review loop, scan loop, delegate/QCA Forward, MCP-augmented tool use.
- Excluded: prompt text (`PROMPTS.md`), safety mechanisms (`MODEL_GUARDRAILS.md`).

---

# Agent Workflows

## Review agent (`internal/agent` + `internal/llmloop`)

**Trigger**: `ocr review` (manual, or from a CI pipeline on a PR/MR event). **Tool access**: full six-tool catalogue (`file_read`, `file_read_diff`, `file_find`, `code_search`, `code_comment`, `task_done`) plus any registered MCP tools. **Output**: `LlmComment` list, rendered and optionally posted back to the CI platform. **Human approval**: none *inside* OCR — the human loop is entirely downstream (a developer or reviewer reads the posted PR comments; nothing in OCR blocks a merge or requires acknowledgment).

```mermaid
sequenceDiagram
    participant U as trigger (CLI/CI)
    participant A as agent.Agent
    participant L as llmloop.Runner
    participant M as LLM

    U->>A: Run(diffs)
    A->>A: 5-gate filter
    loop per kept file (concurrency-bounded)
        A->>L: RunPerFile()
        opt diff >= PLAN_MODE_LINE_THRESHOLD
            L->>M: PLAN_TASK (read-only tools as text)
            M-->>L: checklist
        end
        loop up to MAX_TOOL_REQUEST_TIMES
            L->>M: MAIN_TASK + tool defs
            M-->>L: tool_calls
            L->>L: execute each; task_done -> break
        end
    end
    A->>M: REVIEW_FILTER_TASK (post-loop)
    M-->>A: filtered comments
```

## Scan agent (`internal/scan`)

**Trigger**: `ocr scan` (manual, typically first-time audits or scheduled full-repo passes — no git history required). **Tool access**: same as review, but `file_read_diff`'s "diff" is actually the whole file. **Additional stages review doesn't have**: per-batch `DEDUP_TASK` (merges near-duplicate comments across files in a batch) and an optional whole-run `PROJECT_SUMMARY_TASK`. **Human approval**: none inside OCR, same as review.

## Delegate / QCA Forward — the human-loop-shifted workflow

**Trigger**: an external coding agent invokes `ocr delegate preview` then `ocr delegate rule <paths>`. **What OCR does**: pure deterministic computation — file filtering and rule-text grouping — zero LLM calls, zero network calls beyond local git. **What happens next**: the *calling agent* (not OCR) becomes the reviewer, using its own model and tools. **Human approval**: depends entirely on the calling agent's own workflow — OCR has no visibility into it.

QCA Forward is the most structurally distinct workflow in the repo: it's documented as explicitly forbidding the host agent from running `ocr review`/`ocr llm test`/touching `OCR_LLM_*` at all, disabling Write/Edit tools for the session, and expecting Bash to be read-only — **but that Bash restriction is prompt-enforced, not sandboxed by OCR** (the integration's own docs acknowledge this gap). See `docs/security/TRUST_BOUNDARIES.md`.

## MCP-augmented workflow

**Trigger**: implicit — any configured MCP server's tools are merged into the registry before the review/scan loop starts. **What changes**: the model gains additional tool-call options (e.g., "fetch the linked Jira ticket") within the *same* review/scan loop above — it is not a separate workflow, it's an extension of the review/scan workflow's tool surface. **Failure isolation**: a misbehaving/unreachable MCP server is skipped individually (per `mcp.md`'s documented diagnostics); it does not abort the run.

## Known gaps / uncertainties:
- Whether an MCP server connection failure ever aborts the whole run (vs. always degrading gracefully) was not confirmed by a direct read of the calling code in this pass.
