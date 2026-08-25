---
name: open-code-review
description: >
  Performs AI-powered code review and repository scanning using the `ocr` CLI from
  alibaba/open-code-review. Use when the user asks to review code, review
  a PR, review staged/unstaged changes, review a commit, compare branches, scan
  repository files, or resume an interrupted review. Supports diff-based review,
  full-file scanning, and delegate mode. Produces line-level review comments with
  severity/category classifications and can automatically apply fixes.
license: Apache-2.0
compatibility: >
  Requires the `ocr` CLI installed (via `npm install -g
  @alibaba-group/open-code-review` or GitHub release binary). Review and scan modes
  require a configured LLM provider (Anthropic, OpenAI-compatible, or others).
  Delegate mode requires no OCR-side LLM configuration.
metadata:
  author: alibaba
  homepage: https://github.com/alibaba/open-code-review
  version: "1.10.0"
---

# Open Code Review

A skill for invoking [open-code-review](https://github.com/alibaba/open-code-review) (`ocr`) — an open-source AI code review CLI that reads Git diffs or scans source files and delegates to tool-calling LLM agents to generate structured, line-level review comments.

## Progressive Reference Navigation

Read specific references based on task needs; do not load all files at once:

| Scenario | Reference File |
|----------|----------------|
| Complete flag reference & defaults | [references/flags.md](./references/flags.md) |
| Installation / LLM config / Per-run overrides | [references/llm-config.md](./references/llm-config.md) |
| Custom review rules format & debugging | [references/rules.md](./references/rules.md) |
| MCP Server integration & config | [references/mcp.md](./references/mcp.md) |
| Troubleshooting / Performance tuning / Session management | [references/troubleshooting.md](./references/troubleshooting.md) |

## Mode Selection

```
User Request → Has git diff context?
  ├─ YES → Review Mode
  │         ├─ Single commit → --commit <hash>
  │         ├─ Branch comparison → --from <ref> --to <ref>
  │         └─ Workspace (staged+unstaged+untracked) → No extra flag
  └─ NO → Scan Mode
           ├─ Entire repository → No --path
           └─ Specific path/directory → --path <paths>

LLM not configured & user does not want to configure?
  → Switch to `open-code-review-delegate` skill (host agent conducts review; OCR handles file selection & rule resolution)
```

## Workflow

### Step 1: Gather Business Context

Analyze the review target and extract concise business context to improve review quality.

- Short context (< 2000 chars): `--background "context"` / `-b "context"`
- Long context (PRD/docs): write to a temporary `.md` file, `--background-file <path>` / `-B <path>` (max 1 MB, hard limit 8000 chars)

### Step 2: Execute Review or Scan

**Always use `--audience agent`** (suppresses progress UI). Prefer `--format json`.

> 💡 **Prevent console truncation**: When reviewing many files or running a full-repo `scan`, stdout may exceed the host agent's tool output buffer and be hard-truncated, corrupting the JSON.
> - **Anticipate large reviews**: redirect to a file directly (e.g. `ocr scan --audience agent --format json -b "ctx" > scratch/ocr_result.json`), then read it with a file-reading tool. Note that run-level failure JSON goes to **stderr** — don't miss it when redirecting stdout.
> - **Already truncated**: **no need to re-run `ocr`** — extract comments on demand via `ocr session comments <session-id> --severity high,critical --json`.
> - **Windows PowerShell**: PowerShell 5's `>` redirect writes UTF-16 by default, which corrupts JSON — use `Out-File -Encoding utf8` or run through bash instead.

#### Review Mode (Diff-based)

| User Intent | Command |
|-------------|---------|
| "Review my changes" | `ocr review --audience agent --format json -b "ctx"` |
| "Review feature PR" | `ocr review --audience agent --format json -b "ctx" --from main --to feature` |
| "Review commit abc123" | `ocr review --audience agent --format json -b "ctx" --commit abc123` |
| "Write results to file" | `ocr review --audience agent --format json --output result.json -b "ctx"` |
| "Which files will be reviewed?" | `ocr review --preview --format json` |
| "Resume interrupted review" | `ocr review --audience agent --format json --from main --to feature --resume <session-id>` |

#### Scan Mode (Full-file, No Diff Required)

| User Intent | Command |
|-------------|---------|
| "Scan the whole repo" | `ocr scan --audience agent --format json -b "ctx"` |
| "Scan src/auth/ for security" | `ocr scan --audience agent --format json --path src/auth -b "security audit"` |
| "Fast scan without summary" | `ocr scan --audience agent --format json --no-summary --no-dedup` |
| "Write results to file" | `ocr scan --audience agent --format json --output result.json -b "ctx"` |
| "Resume interrupted scan" | `ocr scan --audience agent --format json --resume <session-id>` |
| "Which files will be scanned?" | `ocr scan --preview --format json` |

### Step 3: Classify and Parse

JSON output core structure:

```json
{
  "status": "review: complete | partial | failed | skipped; scan: success | completed_with_warnings | completed_with_errors",
  "session_id": "...",
  "llm": { "provider": "anthropic", "model": "claude-opus-5" },
  "trace_id": "...",
  "summary": {
    "files_reviewed": 12,
    "comments": 5,
    "total_tokens": 45000,
    "input_tokens": 40000,
    "output_tokens": 5000,
    "cache_read_tokens": 12000,
    "cache_write_tokens": 3000,
    "elapsed": "32s",
    "budget_exceeded": false
  },
  "tool_calls": { "total": 58, "by_tool": { "file_read": 40, "code_search": 18 } },
  "comments": [{
    "path": "src/auth/login.ts",
    "content": "Review comment content",
    "start_line": 42,
    "end_line": 45,
    "category": "security",
    "severity": "high",
    "suggestion_code": "Optional fix suggestion",
    "existing_code": "Original code",
    "thinking": "Optional: LLM reasoning"
  }],
  "warnings": [{ "file": "...", "message": "...", "type": "timeout" }],
  "project_summary": "Optional: scan-mode repository summary",
  "manifest": { "terminal_state": "complete", "coverage": { "selected": 12, "completed": 10, "reused": 0, "failed": 2, "waived": 0 } }
}
```

> **Structure notes**: `manifest` is emitted in review mode only (scan has no manifest, so scan `status` uses the `success` variants); `tool_calls` is always emitted; `llm`, `trace_id`, `project_summary`, `resume`, `message` are optional; run-level failures emit a `status:"failed"` JSON object to **stderr**.

Classify by severity:

- **critical / high** → Must report. Bugs, security risks, data loss risks, clear defects.
- **medium** → Report with context. Performance issues, error-handling flaws, maintainability concerns.
- **low** → Silently discard unless user requests full verbosity.

Categories: `bug`, `security`, `performance`, `maintainability`, `test`, `style`, `documentation`, `other`.

### Step 4: Report

```markdown
## Code Review Results

**Files Reviewed**: N | **Issues Found**: X high / Y medium | **Tokens Used**: Z

### High / Critical

- **`path/to/file.ts:42-45`** [security] — Brief description
  > Fix recommendation

### Medium

- **`path/to/file.go:88`** [performance] — Brief description
  > Recommendation
```

If no issues found: "Review complete — 0 issues found across N files."

**Handling mispositioned comments** (`start_line` and `end_line` are 0): Read the comment content, inspect the target file, locate the target code section, and report/fix at the correct position.

### Step 5: Fix (Optional)

- User asks "review and fix" → Fix critical/high items directly.
- User asks "review" only → Ask for permission before modifying code.
- Apply safe and well-defined fixes directly.
- Verify fixes pass compilation/tests before marking complete.

## Gotchas & Notes

- **Always use `--audience agent`** — `human` mode outputs progress UI that pollutes agent output.
- **Per-run overrides** — Override provider/model/token-limit on single runs using `--provider <name>`, `--model <name>`, or `--max-tokens <n>`.
- **Working directory matters** — `ocr` operates on the git repo in cwd. Use `--repo /path` to override.
- **Workspace mode includes untracked files** — Bare `ocr review` reviews staged + unstaged + untracked changes.
- **Plan phase at 50+ lines** — Diffs exceeding 50 changed lines run a pre-review risk analysis plan phase.
- **Background sanitization** — Applies to `--background-file` (`-B`) only: control characters stripped, content wrapped in `<ocr_user_background>` tags; soft limit 2000 chars, hard limit 8000. Inline `--background` (`-b`) is passed through raw with no sanitization or length limit.
- **Ref injection defense** — `--from`/`--to`/`--commit` values cannot start with `-`.
- **Scan mode needs no git** — `ocr scan` runs on non-git directories.
- **Resume conditions** — `ocr scan` fully supports `--resume`; `ocr review` supports `--resume` only in `--from/--to` or `--commit` modes (not workspace mode).
- **`--preview` and `--resume` are mutually exclusive.**
- **Language configuration** — Default: English. Switch via `ocr config set language 中文`.
- **Avoid tool-buffer truncation** — stdout exceeding the host agent's tool output buffer gets hard-truncated, corrupting JSON. For large runs, redirect to a file (`> scratch/ocr_result.json`) and read it with a file-reading tool; if truncation already happened, no need to re-run `ocr` — extract via `ocr session comments <session-id>`. On Windows PowerShell, use `Out-File -Encoding utf8` instead of `>` (PowerShell 5's `>` writes UTF-16 and corrupts JSON).
- **Do not test connectivity pre-emptively** — Execute review/scan directly; troubleshoot only on actual LLM failure (see troubleshooting.md).

## Verification

After review completes:

1. Command exit code is 0 (non-zero only on run-level failure or when every selected item failed)
2. JSON `status` field: `"complete"` for review (or `"partial"` with acceptable warnings); `"success"` for scan (`completed_with_warnings` / `completed_with_errors` acceptable — inspect `warnings`)
3. `comments` array structured as expected
4. `summary.files_reviewed` matches target count

## References

- Homepage & Docs: https://github.com/alibaba/open-code-review
- NPM Package: https://www.npmjs.com/package/@alibaba-group/open-code-review
- Issue Tracker: https://github.com/alibaba/open-code-review/issues
- Official site: https://open-codereview.ai
