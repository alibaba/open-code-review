---
name: open-code-review
description: >
  Performs AI-powered code review on Git changes using the `ocr` CLI from
  alibaba/open-code-review. Use when the user asks to review code, review
  a pull request, review staged/unstaged changes, review a commit, or
  compare branches for code quality issues. Produces line-level review
  comments and can automatically apply fixes when requested. With appropriate
  review rules, can detect various types of issues including bugs, security
  vulnerabilities, performance problems, and code quality concerns.
license: Apache-2.0
compatibility: >
  Requires the `ocr` CLI installed (via `npm install -g
  @alibaba-group/open-code-review` or GitHub release binary). Requires a
  configured LLM (Anthropic or OpenAI-compatible) before first run.
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

A skill for invoking [open-code-review](https://github.com/alibaba/open-code-review) (`ocr`) — an open-source AI code review CLI that reads Git diffs and generates structured, line-level review comments.

## Workflow

### Step 1: Gather Business Context

Analyze the review target (commits, branch, or changes) to extract concise business context. Pass this context via `--background` to improve review quality.

### Step 2: Run Code Review

Run the OCR command with appropriate flags. **Always pass business context via `--background`** when available:

```bash
ocr review --audience agent --background "business context here" [user-args]
```

**Argument handling:**

- **Background context** (RECOMMENDED): use `--background "context"` or `-b "context"` to provide business context for better review quality
- **Default** (no user arguments): reviews staged, unstaged, and untracked changes (workspace mode)
- **Specific commit**: use `--commit` or `-c` to review a single commit against its parent
- **Branch comparison**: use `--from <ref>` and `--to <ref>` to review diff between two refs
- **Timeout**: effective timeout per review group = `--timeout` × review rounds. Default `--timeout 15` with default effort `medium` (2 rounds) gives 30 minutes; `low`/`high` give 15/45 minutes.
- **Concurrency**: default concurrency is 8 file workers; reduce with `--concurrency <n>` if rate limits are hit
- **Preview mode**: use `--preview` or `-p` to preview which files will be reviewed without running the LLM
- **Output file**: use `--output <path>` or `-o <path>` to write the full result to a file instead of stdout (`-` means stdout)
- **Installation**: if `ocr` command is not found, install it by running `npm i -g @alibaba-group/open-code-review`

**Common invocation patterns:**

| User says | Command to run |
|-----------|---------------|
| "review my changes" / "review the working copy" | `ocr review --audience agent -b "context"` |
| "review this PR" / "review feature branch" | `ocr review --audience agent -b "context" --from main --to <branch>` |
| "review commit abc123" | `ocr review --audience agent -b "context" --commit abc123` |
| "what would be reviewed?" (dry-run) | `ocr review --preview` |

**Output mode:**

- Always use `--audience agent` to suppress progress UI and emit only the final summary
- **Prevent output truncation**: For large reviews or restricted tool environments, pass `--output /tmp/ocr_out.txt` (`ocr` v1.10.0+) and inspect the file in full via a file reading tool instead of piping stdout through `tail` or `head`, which drops earlier review comments. If the command fails with `unknown flag: --output`, fall back to shell redirection (`> /tmp/ocr_out.txt 2>&1`).

**On failure:** If `ocr review` exits non-zero (e.g. an LLM connection error), do not retry blindly — consult the Troubleshooting section below for the matching fix before re-running.

### Step 3: Report

OCR output includes structured `severity` (critical / high / medium / low) and `category` (bug / security / performance / maintainability / test / style / documentation / other) on each comment. Present results grouped by severity, discarding `low` severity items that are likely false positives or nitpicks.

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

If no critical, high, or medium severity issues remain after filtering, state: "Review complete — no critical, high, or medium issues found in N files."

**Handling mispositioned comments:**

When `start_line` and `end_line` are both `0`, the comment failed to locate the exact position in the file. In such cases:

1. Read the comment content to understand the issue
2. Examine the target file mentioned in the comment
3. Identify the relevant code section based on the comment's context
4. Apply the fix or suggestion to the correct location

## Custom Review Rules

If the user wants project-specific rules, OCR resolves them in this priority order:

1. `--rule <path>` flag (highest)
2. `<repo>/.opencodereview/rule.json`
3. `~/.opencodereview/rule.json`
4. Built-in system defaults (lowest)

By default, the first matching user rule replaces the built-in system rule. Set `merge_system_rule: true` on a rule entry when the matched system rule and user rule should both be included.

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

## Gotchas

- **LLM must be configured first** — `ocr review` will fail loudly if no LLM is reachable. See the Troubleshooting section below if this happens.
- **Working directory matters** — `ocr review` operates on the Git repo at the current directory. Use `--repo /path/to/repo` to run from elsewhere.
- **Untracked files are reviewed in workspace mode** — running bare `ocr review` includes staged, unstaged, *and* untracked changes. Stage selectively if you want narrower scope.
- **Large diffs may hit token limits** — files with very large diffs may be truncated. The default `MAX_TOKENS` is 58888 per request.
- **Plan phase triggers at 50 lines** — diffs exceeding 50 changed lines run an extra risk-analysis phase before main review. This adds latency but improves quality.
- **Don't pass `--audience human`** — it streams progress UI that pollutes output. Always use `--audience agent`.
- **Comment language follows config** — set `language` config to `English` or `Chinese` (default: Chinese) to control review comment language.
- **Avoid output truncation** — Large review runs produce verbose output. Never pipe command output to `tail` or `head` as it drops review comments from earlier sections. Use `--output <path>` (shell redirection works on older CLI versions) and read it in full.

## Validation

After the review completes, verify success by checking:

1. The command exited with code 0
2. Comments were generated (or "No comments generated" message appears)
3. Warnings (if any) are displayed in stderr

If errors occurred, check the stderr warnings for details about which files failed and why.

## Troubleshooting

**`ocr: command not found`**

Install the CLI:

```bash
npm install -g @alibaba-group/open-code-review
```

**`ocr review` fails with LLM connection error**

Prompt the user to configure an LLM provider.

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

Verify connectivity with `ocr llm test`. Stop here and ask the user to provide credentials — never invent or hardcode API keys.

## References

- Full docs: https://github.com/alibaba/open-code-review
- NPM package: https://www.npmjs.com/package/@alibaba-group/open-code-review
- Issue tracker: https://github.com/alibaba/open-code-review/issues
