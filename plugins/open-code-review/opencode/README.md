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

Check your OpenCode version (`opencode --version` vs `opencode2 --version`)
and install the matching dependencies below. The single plugin file
(`open-code-review.ts`) serves both versions: V1 reads its `server`
entrypoint, V2 reads its `id` + `setup` entrypoint
([dual plugin form](https://opencode.ai/v2/docs/build/plugins#support-v1)).
The V2 API is imported as types only, so OpenCode 1.x needs just
`@opencode-ai/plugin` at runtime, while OpenCode 2.x needs both packages.

## Install globally

```bash
mkdir -p ~/.config/opencode/plugins
curl -fsSL \
  https://raw.githubusercontent.com/alibaba/open-code-review/main/plugins/open-code-review/opencode/open-code-review.ts \
  -o ~/.config/opencode/plugins/open-code-review.ts
```

Then install the runtime dependencies for your version (otherwise the
server log shows `Cannot find package ...` and the plugin fails to load):

```bash
cd ~/.config/opencode
npm init -y # skip if package.json already exists
npm install @opencode-ai/plugin # OpenCode 1.x
npm install @opencode-ai/plugin @opencode/plugin@beta # OpenCode 2.x
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

Then add the runtime dependencies for your version to the project's
`.opencode/package.json` as above. Commit the plugin file if the
integration should be shared with the project.

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
- Tool-driven reviews default to a 30-minute overall timeout and a 10 MiB output limit.
- Cancelling the OpenCode tool terminates the OCR process (1.x; on 2.x
  the tool API has no abort signal, so cancellation relies on the overall
  timeout).
- OCR credentials remain in the existing OCR configuration or environment.
- Workspace mode includes staged, unstaged, and untracked files.

## Development

```bash
cd plugins/open-code-review/opencode
npm install
npm run check
```
