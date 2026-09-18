# OpenCodeReview for pi

A [pi coding agent](https://github.com/badlogic/pi-mono) extension for
[OpenCodeReview](https://github.com/alibaba/open-code-review) (`ocr`) — an
open-source AI code review CLI that reads Git diffs and generates structured,
line-level review comments.

The extension adds two slash commands and two LLM-callable tools, covering both
of OCR's execution modes:

| Mode | Command | Tool | Who runs the review LLM |
|------|---------|------|-------------------------|
| Full review | `/ocr-review` | `ocr_review` | OCR's own configured LLM |
| Delegation | `/ocr-review-delegate` | `ocr_delegate` | pi's model (no OCR LLM needed) |

## Requirements

- Git >= 2.41
- The `ocr` CLI installed: `npm install -g @alibaba-group/open-code-review`
- For full review mode: a configured OCR LLM endpoint
  (`ocr config provider` && `ocr config model`, then verify with `ocr llm test`).
  Delegation mode has no LLM requirement on the OCR side.

## Install

**From npm (recommended):**

```bash
pi install npm:@alibaba-group/open-code-review-pi
```

The package has zero runtime dependencies, so the install does not pull in a
`node_modules` tree; pi loads `src/index.ts` directly.

**From a checkout of this repository (for development):**

```bash
# Try it once in the current session:
pi -e plugins/open-code-review/pi/src/index.ts

# Or copy the directory into your global extensions:
cp -r plugins/open-code-review/pi ~/.pi/agent/extensions/open-code-review
```

Restart pi (or run `/reload`) after a manual copy.

## Commands

### `/ocr-review [args]`

Runs a full OCR review and hands the structured findings to pi, which presents
them grouped by severity and can apply fixes on request. Arguments map onto the
`ocr_review` tool parameters, for example:

```text
/ocr-review                          # workspace changes (staged, unstaged, untracked)
/ocr-review --from main --to feat    # branch comparison
/ocr-review -c abc123                # single commit
/ocr-review -b "payment flow must handle timeouts"
```

### `/ocr-review-delegate [args]`

Delegation mode: OCR's deterministic engineering decides **which files to
review** (with exclusion reasons) and resolves the **review rules** per file;
pi's own model then performs the review with its regular tools (git, read,
grep) and reports a coverage summary. Useful when you want pi's model to do the
reasoning, or when no OCR LLM endpoint is configured.

## Tools

The tools are also available autonomously — pi's model calls them when you ask
for a review without using a slash command:

- **`ocr_review`** — runs `ocr review --audience agent --format json` against
  workspace changes, a commit, or a ref range. Results larger than pi's inline
  output limit are spilled to a file whose path is returned. `preview: true`
  lists the files that would be reviewed without calling an LLM.
- **`ocr_delegate`** — `action: "preview"` returns the reviewable file list
  with mode/refs (`merge_base` included for range mode); `action: "rule"`
  returns the resolved review rules for given paths, grouped by content.

## Custom review rules

OCR resolves rules in this priority order: `--rule` flag →
`<repo>/.opencodereview/rule.json` → `~/.opencodereview/rule.json` → built-in
system defaults. See the
[review rules documentation](https://open-codereview.ai/docs/review-rules).

## Troubleshooting

| Symptom | Fix |
|---------|-----|
| `ocr: command not found` | `npm install -g @alibaba-group/open-code-review` |
| Review fails with an LLM connection error | `ocr config provider`, `ocr config model`, then `ocr llm test` |
| `unknown flag: --output` | The installed CLI is older than v1.10.0: `npm i -g @alibaba-group/open-code-review@latest` |
| Interrupted review | Re-run with the session id printed by the tool (`resume` tool parameter, or `ocr review --resume <id>`) |

## Development

```bash
npm run typecheck   # tsc --noEmit against the pi extension API types
npm test            # node --test on the pure helpers (arg building, parsing, prompts)
```

The extension intentionally keeps zero runtime dependencies: the only non-Node
import is a type-only import of `ExtensionAPI`, which pi erases when loading
the file. All subprocess work goes through `pi.exec`, and tool parameter
schemas are plain JSON Schema (TypeBox-compatible).
