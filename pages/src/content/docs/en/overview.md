---
title: Overview
sidebar:
  order: 2
---

## What is Open Code Review?

Open Code Review (**OCR** for short, distinct from Optical Character
Recognition) is an AI-powered code review CLI distributed as the
[`@alibaba-group/open-code-review`](https://www.npmjs.com/package/@alibaba-group/open-code-review)
NPM package and as standalone Go binaries. The CLI binary is named `ocr`.

In a single command (`ocr review`) it:

1. Resolves a Git diff — workspace, branch range, or single commit.
2. Filters the changed files using both system defaults and any user rules.
3. Spawns one **per-file sub-agent** for each changed file, in parallel.
4. Each sub-agent runs an LLM tool-use loop, optionally preceded by a
   **plan phase** for larger diffs.
5. The model calls `code_comment` to record findings, optionally `file_read`,
   `code_search`, `file_find`, `file_read_diff` to gather context, and
   `task_done` when finished.
6. OCR resolves each comment to exact line numbers, runs an optional
   re-positioning pass for any comments that didn't match cleanly, and
   prints (or JSON-emits) the final list.

## The problem with general-purpose agents

If you've used a general-purpose coding agent (Claude Code with a Skill,
Cursor, Cline, etc.) for code review, you've likely run into:

- **Incomplete coverage** — on larger changesets the agent quietly cuts
  corners, reviewing only some files.
- **Position drift** — comments don't line up with the code they refer to;
  line numbers and file paths drift off target.
- **Unstable quality** — natural-language Skills are hard to debug, and
  output quality fluctuates with minor prompt edits.

The root cause: a purely language-driven architecture lacks **hard
constraints** on the review process.

## Core design: deterministic engineering × agent

OCR's core philosophy is to combine **deterministic engineering** with an
**agent** — each handling what it does best.

### Deterministic engineering — hard constraints

For steps that *must not go wrong*, engineering logic — not the model —
guarantees correctness:

- **Precise file selection** — a [five-gate filter](../review-rules/#how-files-are-filtered)
  decides exactly which files are reviewed, with explicit `include`/`exclude`
  controls.
- **Smart file bundling** — related files (e.g., `message_en.properties` and
  `message_zh.properties`) can be grouped into a single review unit. Each
  bundle runs as a sub-agent with isolated context — divide and conquer that
  stays stable on very large changesets and naturally supports concurrent
  review.
- **Fine-grained rule matching** — review rules are matched per file path
  with first-match-wins, keeping the model's attention sharply focused and
  eliminating noise. Template-based matching is more stable than purely
  language-driven rule guidance.
- **External positioning and reflection modules** — independent comment
  positioning ([`internal/diff/relocation.go`](https://github.com/alibaba/open-code-review/blob/main/internal/diff/relocation.go))
  and re-location passes systematically improve both location and content
  accuracy.

### Agent — dynamic decision-making

The agent's strengths are concentrated where they matter most:

- **Scenario-tuned prompts** — prompt templates deeply optimized for code
  review, improving effectiveness while reducing token consumption (see
  [`internal/config/template/task_template.json`](https://github.com/alibaba/open-code-review/blob/main/internal/config/template/task_template.json)).
- **Scenario-tuned toolset** — distilled from analysis of tool-call traces in
  large-scale production data (call-frequency distributions, per-tool
  repetition rates, the impact of each tool on the overall call chain). The
  result is a purpose-built set of [six tools](../tools/) that is more stable
  and predictable than a generic agent toolkit.

## How the pipeline fits together

```mermaid
flowchart TD
    Start["<b>ocr review --from main --to feature</b>"]
    S1["<b>1. Resolve LLM endpoint</b><br/>config / env / shell rc"]
    S2["<b>2. Load diffs from git</b><br/>workspace / commit / range"]
    S3["<b>3. Filter files</b><br/>binary → user_exclude → user_include<br/>→ ext allowlist → default path"]
    S4["<b>4. Drop diffs > 80% of MAX_TOKENS</b>"]
    S5["<b>5. Dispatch per-file sub-agents</b> (concurrent)<br/><br/>For each file:<br/>&nbsp;&nbsp;a. Plan phase (if changed lines ≥ 50)<br/>&nbsp;&nbsp;b. Main loop: LLM → tool calls → … → task_done<br/>&nbsp;&nbsp;c. code_comment results collected (async via worker pool)<br/><br/>Memory compression triggers when context<br/>exceeds 60 % (async) or 80 % (sync) of MAX_TOKENS."]
    S6["<b>6. Resolve line numbers</b><br/>from <code>existing_code</code> against diffs.<br/>Re-locate via LLM if needed."]
    S7["<b>7. Emit text or JSON output</b><br/>(and persist session to disk)"]

    Start --> S1 --> S2 --> S3 --> S4 --> S5 --> S6 --> S7
```

## Project layout

```
open-code-review/
├── cmd/opencodereview/   # CLI entry point: dispatch, flags, commands
├── internal/
│   ├── agent/            # Per-file sub-agent loop + memory compression
│   ├── config/
│   │   ├── allowlist/    # Default file-extension allowlist & exclusions
│   │   ├── rules/        # Layered rule resolver, system rule docs
│   │   ├── template/     # Plan / main / memory_compression prompts
│   │   ├── testconnection/ # Built-in `ocr llm test` task
│   │   └── toolsconfig/  # Tool definitions sent to the model
│   ├── diff/             # Git diff parsing, hunk math, relocation
│   ├── gitcmd/           # Git subprocess runner
│   ├── llm/              # Anthropic + OpenAI protocols, retries, BPE tokens
│   ├── model/            # Diff / Comment data structures
│   ├── pathutil/         # Path utilities
│   ├── release/          # Release-notes generation
│   ├── session/          # JSONL persistence of every review session
│   ├── stdout/           # Quiet-able stdout writer for `--audience agent`
│   ├── suggestdiff/      # Build "Apply suggestion" diffs
│   ├── telemetry/        # OpenTelemetry spans, metrics, exporters
│   ├── tool/             # The six built-in tools + comment collector
│   └── viewer/           # `ocr viewer` — local web UI for past sessions
├── pages/                # React-based marketing landing page (separate)
├── plugins/              # Claude Code plugin manifest + commands
├── extensions/           # Editor extensions (VS Code)
├── examples/             # CI recipes (GitHub Actions, GitLab CI)
├── skills/               # Generic agent Skill manifest
├── scripts/              # NPM install/update helpers, publish scripts
├── npm/                  # Per-platform optional dependency packages
└── bin/                  # NPM wrapper that shells out to the binary
```

## See Also

- [QuickStart](../quickstart/) — install and run your first review.
- [Architecture](../architecture/) — the agent loop, plan phase, and memory compression.
- [CLI Reference](../cli-reference/) — every flag and sub-command.
- [Integrations](../integrations/) — call OCR from Claude Code or any agent.
