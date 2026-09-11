# OpenCode integration

This integration exposes OpenCodeReview as native tools and slash commands in
[OpenCode](https://opencode.ai/).

It registers:

- `ocr_review` — review workspace changes, one commit, or a ref range and
  return structured JSON findings.
- `ocr_health` — show the installed OCR version and test its configured LLM
  connection.
- `/ocr-review` and `/ocr-health` — convenient prompts that invoke the tools.

Existing user commands with either name are preserved.

## Prerequisites

Install and configure OpenCodeReview first:

```bash
npm install -g @alibaba-group/open-code-review
ocr config provider
ocr config model
ocr llm test
```

## Install globally

```bash
mkdir -p ~/.config/opencode/plugins
curl -fsSL \
  https://raw.githubusercontent.com/alibaba/open-code-review/main/plugins/open-code-review/opencode/open-code-review.ts \
  -o ~/.config/opencode/plugins/open-code-review.ts
```

Restart OpenCode after installation.

## Install for one project

Run this from the project root:

```bash
mkdir -p .opencode/plugins
curl -fsSL \
  https://raw.githubusercontent.com/alibaba/open-code-review/main/plugins/open-code-review/opencode/open-code-review.ts \
  -o .opencode/plugins/open-code-review.ts
```

Commit the plugin file if the integration should be shared with the project.

## Usage

Use the registered commands:

```text
/ocr-review current workspace; focus on authentication regressions
/ocr-review compare main to feature/auth-refresh
/ocr-health
```

Or ask OpenCode naturally:

```text
Use ocr_review to review my current changes. The goal is to add rate limiting
without changing the public API.
```

Set `preview` to `true` to inspect which files would be reviewed without making
an LLM request.

## Behavior and safety

- Reviews use `--audience agent` and JSON output.
- The process is launched with an argument array and `shell: false`.
- Background context uses OCR's sanitised 8,000-character file input through a
  private temporary file, removed after success, failure, cancellation, or timeout.
- Reviews have a 15-minute overall timeout and a 10 MiB output limit.
- Cancelling the OpenCode tool terminates the OCR process.
- OCR credentials remain in the existing OCR configuration or environment.
- Workspace mode includes staged, unstaged, and untracked files.

## Development

```bash
cd plugins/open-code-review/opencode
npm install
npm run check
```
