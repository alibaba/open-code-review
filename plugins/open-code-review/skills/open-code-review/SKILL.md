---
name: open-code-review
description: >
  Review workspace changes, PRs, commits, or branch comparisons with OCR's
  native review pipeline and independently configured model provider.
  Produces line-level findings and explains the behavior and impact of
  committed changes by default. Fixes code only when requested. Use when
  the user wants native mode or has an OCR provider configured. To use the
  host agent's model without separate OCR credentials, choose
  open-code-review-delegate instead.
license: Apache-2.0
compatibility: >
  Requires the `ocr` CLI installed (via `npm install -g
  @alibaba-group/open-code-review` or GitHub release binary). Requires a
  configured supported LLM provider before first run (protocols: Anthropic,
  OpenAI Chat Completions, OpenAI Responses, AWS Bedrock).
metadata:
  author: alibaba
  homepage: https://github.com/alibaba/open-code-review
  version: "1.0.0"
---

# Open Code Review

This Codex plugin skill intentionally mirrors the canonical skill at
`skills/open-code-review/SKILL.md`. Keep both files synchronized when updating
OCR agent instructions; a symlink is avoided because plugin installs may only
materialize the plugin subtree.

## Workflow

### Step 1: Gather Business Context

Analyze the review target (commits, branch, or changes) to extract concise business context. Pass this context via `--background` to improve review quality.

### Step 2: Run Code Review

**Run `ocr review` directly.** Do not probe with `command -v ocr` or `ocr --version` first; that adds an unnecessary tool call on the normal path.

For a workspace review, omit revision flags and `--analysis`. **Pass business context via `--background`** when available:

```bash
ocr review --audience agent --background "business context here"
```

**Argument handling:**

- **Background context** (RECOMMENDED): use `--background "context"` or `-b "context"` to provide business context for better review quality
- **Default** (no user arguments): reviews staged, unstaged, and untracked changes (workspace mode)
- **Specific commit**: use `--commit` or `-c` to review a single commit against its parent
- **Branch comparison**: use `--from <ref>` and `--to <ref>` to review diff between two refs
- **Current branch against a base**: `--from <ref>` defaults the target to `HEAD`, equivalent to `--from <ref> --to HEAD`. This reviews committed changes from the merge-base to HEAD, excluding staged, unstaged, and untracked changes. Omit both refs to review workspace changes.
- **Timeout**: effective timeout per review group = `--timeout` × review rounds. Default `--timeout 15` with default effort `medium` (2 rounds) gives 30 minutes; `low`/`high` give 15/45 minutes.
- **Concurrency**: default concurrency is 8 file workers; reduce with `--concurrency <n>` if rate limits are hit
- **Preview mode**: use `--preview` or `-p` to preview which files will be reviewed without running the LLM
- **Output file**: use `--output <path>` to write the full result to a file instead of stdout.

**Common invocation patterns:**

| User says | Command to run |
|-----------|---------------|
| "review my changes" / "review the working copy" | `ocr review --audience agent -b "context"` |
| "review this PR" / "review feature branch" | `ocr review --audience agent -b "context" --from <base> --to <branch> --analysis --format json` |
| "review the current branch against main" | `ocr review --audience agent -b "context" --from main --analysis --format json` |
| "review commit abc123" | `ocr review --audience agent -b "context" --commit abc123 --analysis --format json` |
| "what would be reviewed?" (dry-run) | `ocr review --preview` |

**Behavior and impact analysis:**

For a fresh commit or range review (`--commit` or `--from`), run Review and Analysis together with `--analysis` by default. Use JSON unless the caller requests another format. Preserve the caller's repository, revisions, background, and output path.

Omit `--analysis` for workspace reviews, `--preview`, `--resume`, or `--format sarif`, and when the user explicitly asks to skip analysis. If the user explicitly requests analysis in an unsupported mode, explain the conflict rather than changing the target or format.

**Output mode:**

- Always use `--audience agent` to suppress progress UI and emit only the final summary
- **Prevent output truncation**: For large reviews or restricted tool environments, pass `--output /tmp/ocr_out.txt` and inspect the file in full via a file reading tool instead of piping stdout through `tail` or `head`, which drops earlier review comments.

**On failure:** If `ocr review` exits non-zero (e.g. an LLM connection error), do not retry blindly — consult the Troubleshooting section below for the matching fix before re-running.

### Step 3: Report

OCR output includes structured `severity` (critical / high / medium / low) and `category` (bug / security / performance / maintainability / test / style / documentation / other) on each comment. Present results grouped by severity, discarding `low` severity items that are likely false positives or nitpicks.

For combined runs, report Review and Analysis separately. Use these fields in `ocr.review-with-analysis/v2` JSON; if the caller requested text, use the Review summary and appended Analysis Report instead:

- **Review:** report findings from `comments` and coverage from `review_status` and `manifest.coverage` when available. A zero exit code or empty `comments` does not prove all selected files were reviewed.
- **Analysis:** summarize the before/after behavior, affected workflows, and any decisions supported by `analysis`. Distinguish `analysis_meta.execution_status` from `analysis_meta.declared_completion` (the model's assessment). For example, `completed` with declared `partial` means the investigation ran but left questions open. If analysis was skipped or failed, say why when known and retain the Review findings; do not infer that behavior is unchanged. If the combined schema or analysis metadata is missing, report analysis as unverified.

### Step 4: Fix

Before applying fixes, check whether the user requested automatic fixes:

- If the user explicitly requested "review and fix" or similar, proceed with automatic fixes
- If the user only requested "review" without fix intent, ask for permission before applying any changes

When fixing issues and suggestions:

- Focus on critical, high, and medium severity items
- Apply fixes directly to the code when safe and well-defined
- For complex fixes requiring manual intervention, clearly describe what needs to be done
- Always verify fixes with the user before committing

## Output Format

Each comment in OCR's output contains:

- `path`: File path
- `content`: Review comment text
- `start_line` / `end_line`: Line range (both 0 means positioning failed)
- `category`: Issue category (bug, security, performance, maintainability, test, style, documentation, other)
- `severity`: Issue severity (critical, high, medium, low)
- `suggestion_code`: Optional fix suggestion
- `existing_code`: Optional original code snippet
- `thinking`: Optional LLM reasoning process

Present results grouped by severity using this template:

```markdown
## Code Review Results

**Files reviewed**: N
**Issues found**: X critical, Y high, Z medium

### Critical

- **`path/to/file.java:42`** [bug] — Brief description
  > Recommendation: How to fix

### High

- **`path/to/file.java:26`** [bug] — Brief description
  > Recommendation: How to fix

### Medium

- **`path/to/file.ts:88`** [performance] — Brief description
  > Recommendation: How to fix (if applicable)
```

Only use "Review complete — no critical, high, or medium issues found in N files" when Review coverage supports it. Otherwise state the known coverage and gaps. For combined runs, also report the Analysis outcome separately.

**Handling mispositioned comments:**

When `start_line` and `end_line` are both `0`, the comment failed to locate the exact position in the file. In such cases:

1. Read the comment content to understand the issue
2. Examine the target file mentioned in the comment
3. Identify the relevant code section based on the comment's context
4. Verify the location and report the finding; if the exact position stays uncertain, report it at file level
5. Apply a fix or suggestion only when the user has authorized edits under Step 4

## Custom Review Rules

If the user wants project-specific rules, OCR resolves them in this priority order:

1. `--rule <path>` flag (highest)
2. `<repo>/.opencodereview/rule.json`
3. `~/.opencodereview/rule.json`
4. Built-in system defaults (lowest)

By default, the first matching user rule replaces the built-in system rule. Set `merge_system_rule: true` on a rule entry when the matched system rule and user rule should both be included. Set `"scope": "rules_only"` on a rule entry to report only breaches of that rule and nothing else for the files it matches.

Rule file format:

```json
{
  "rules": [
    {
      "path": "**/*.java",
      "rule": "All new methods must validate required parameters for null",
      "merge_system_rule": true
    },
    {
      "path": "**/*mapper*.xml",
      "rule": "Check SQL for injection risks and missing closing tags"
    }
  ]
}
```

To preview which rule applies to a file before reviewing:

```bash
ocr rules check src/main/java/com/example/Foo.java
```

## Advanced Review Options

Beyond the common flags above, `ocr review` exposes a few groups of controls. Run `ocr review --help` for the complete list.

**Scoping**

- `--exclude '<patterns>'` — comma-separated gitignore-style patterns (for example `--exclude '**/generated/*,**/testdata/*'`), merged with `rule.json` excludes.
- `--background-file <path>` — read review context from a Markdown file. Takes precedence over `--background`.

**Output**

- `--format text|json|sarif` — `text` (default) for humans; `json` for machine-readable findings; `sarif` for code-scanning integrations such as GitHub Code Scanning.

**Model**

- `--provider <name>` / `--model <name>` — override the configured provider/model for this run only (for example, to recheck a diff with a different model; the user names the model, `ocr llm providers` lists the built-ins).

**Budget**

- `--max-tokens <n>` — per-group prompt ceiling; defaults to the configured value or the template default (`200000`).
- `--max-tokens-budget <n>` — cap total input + output tokens for Review. Checked before every LLM round: a group already over budget gets one final round to submit findings, no further groups are dispatched, partial results are still published, and skipped files are reported as `failed(budget)`.
- `--analysis-max-tokens-budget <n>` — separate soft token limit for Analysis only (default `0`, unlimited). Its usage is reported in `analysis_meta.usage`, not the Review summary.
- `--no-filter` — keep all review comments and skip the LLM post-filtering call.

## Gotchas

- **LLM must be configured first** — `ocr review` will fail loudly if no LLM is reachable. See the Troubleshooting section below if this happens.
- **Working directory matters** — `ocr review` operates on the Git repo at the current directory. Use `--repo /path/to/repo` to run from elsewhere.
- **Untracked files are reviewed in workspace mode** — running bare `ocr review` includes staged, unstaged, *and* untracked changes. Stage selectively if you want narrower scope.
- **Large diffs may hit token limits** — `MAX_TOKENS` sets the prompt budget (`200000` in the review template; `ocr scan` uses `58888`); conversation context is compressed to stay within this prompt budget. Model output is capped separately by `MAX_COMPLETION_TOKENS` (`16384`). A file whose diff alone exceeds ~80% of `MAX_TOKENS` is skipped before the LLM is called.
- **Plan phase triggers on either of two thresholds** — a group runs an extra risk-analysis phase before main review when its largest changed file reaches `PLAN_MODE_LINE_THRESHOLD` (default `50`) **or** it holds 2+ files whose combined changed lines reach `PLAN_MODE_GROUP_LINE_THRESHOLD` (default `100`). This adds latency but improves quality.
- **Don't pass `--audience human`** — it streams progress UI that pollutes output. Always use `--audience agent`.
- **Comment language follows config** — the `language` config controls review comment language, defaults to `English`, and accepts any language name (for example `English` or `中文`).
- **Avoid output truncation** — Large review runs produce verbose output. Never pipe command output to `tail` or `head` as it drops review comments from earlier sections. Use `--output <path>` and read it in full.
- **Resume an interrupted review** — a failed or interrupted range/commit review can be continued with `ocr review --resume <id>` using the same `--from`/`--to` or `--commit` target (the id is printed as `retry with: --resume <id>` on failure, or find it with `ocr session list`). Workspace resume is not supported.

## Validation

Check the exit code, output, and stderr for failures or coverage gaps. The exit code reflects Review, not Analysis: use the separate outcomes in Step 3 for combined runs.

## Troubleshooting

**CLI missing or flag not recognized**

- If `ocr` is not found, install the latest CLI with `npm i -g @alibaba-group/open-code-review@latest`.
- If OCR reports `unknown flag` for an option this skill uses, offer to upgrade with the same command and wait for approval. For a user-supplied option, check the spelling and subcommand first. Do not silently drop the flag.
- After installation or an approved upgrade succeeds, rerun the original command once. If it still fails, report the error rather than retrying or changing the request.

**`ocr review` fails with LLM connection error**

If OCR credentials are unavailable, offer `open-code-review-delegate` rather than requiring a new key. If that skill is missing, offer `npx skills add alibaba/open-code-review --skill open-code-review-delegate`. Do not switch modes without user agreement. To continue in native mode, diagnose the provider connection or configure it below; a host-agent login is not an OCR credential.

Interactive setup (recommended):

```bash
ocr config provider
```

Manual setup (alternative):

```bash
ocr config set llm.url https://api.anthropic.com/v1/messages
ocr config set llm.auth_token <api-key>
ocr config set llm.model claude-opus-4-6
ocr config set llm.use_anthropic true
```

Verify connectivity with `ocr llm test`. If credentials are needed for native mode, ask the user to configure them locally rather than paste secrets into the conversation. Never invent or hardcode API keys.

## References

- Full docs: https://github.com/alibaba/open-code-review
- NPM package: https://www.npmjs.com/package/@alibaba-group/open-code-review
- Issue tracker: https://github.com/alibaba/open-code-review/issues
