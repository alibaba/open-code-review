---
description: Run OpenCodeReview (OCR) to review code changes and autonomously apply fixes.
---

Invoke the professional code review Agent CLI tool OpenCodeReview (OCR) to review current code changes, and let the Agent autonomously decide whether to apply fixes.

## Workflow

### Step 1: Run Code Review

Run the OCR command:

```bash
ocr review --audience agent [user-args]
```
- Default (no user arguments): reviews staged, unstaged, and untracked changes (workspace mode).
- If the user provides `--commit` or `--c`: pass through as-is.
- If the user provides `--from` and `--to`: pass through as-is.
- (Optional) Provide `--background "requirement context"` to review whether the requirements are correctly implemented.
- (Optional) Provide `--background-file ./requirements.md` to load the same context from a Markdown file (sanitised and limited to 8000 characters). Combined with `--background` the inline value is given first.
- Capture full stdout. Set a 5-minute timeout.
- If the `ocr` command is not found, install it by running `npm i -g @alibaba-group/open-code-review`.

### Step 2: Filter and Evaluate

For each comment, assess its validity and quality:

- **High**: Obvious bugs, security issues, clear mistakes, or well-founded suggestions with precise fix proposals
- **Medium**: Reasonable concerns but context-dependent, style/performance suggestions, or fixes that require manual implementation
- **Low**: Likely false positives, lacking sufficient context, nitpicks, or meaningless suggestions

Silently discard low-confidence comments. Display the remaining comments.

### Step 3: Fix

Automatically fix issues and suggestions that are worth adopting.
