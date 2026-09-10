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
and follow the matching section below. The single-file plugin
(`open-code-review.ts`) only works on OpenCode 1.x. OpenCode 2.x removed
plugin-defined tools, so the same features ship as native
[custom tools](https://opencode.ai/docs/custom-tools/) plus
[commands](https://opencode.ai/docs/commands/).

## Install globally (OpenCode 1.x)

```bash
mkdir -p ~/.config/opencode/plugins
curl -fsSL \
  https://raw.githubusercontent.com/alibaba/open-code-review/main/plugins/open-code-review/opencode/open-code-review.ts \
  -o ~/.config/opencode/plugins/open-code-review.ts
```

The plugin imports `@opencode-ai/plugin`, so the config directory needs
that dependency (otherwise the server log shows
`Cannot find package '@opencode-ai/plugin'` and the plugin fails to load):

```bash
cd ~/.config/opencode
npm init -y # skip if package.json already exists
npm install @opencode-ai/plugin
```

Restart OpenCode after installation.

## Install globally (OpenCode 2.x)

Copy the `tools/` and `commands/` directories from here into your OpenCode
config:

```bash
mkdir -p ~/.config/opencode/tools ~/.config/opencode/commands
cp tools/ocr_review.ts tools/ocr_health.ts ~/.config/opencode/tools/
cp commands/ocr-review.md commands/ocr-health.md ~/.config/opencode/commands/
```

The tools import `@opencode-ai/plugin`, so the config directory needs that
dependency:

```bash
cd ~/.config/opencode
npm init -y # skip if package.json already exists
npm install @opencode-ai/plugin
```

Restart OpenCode after installation. Do **not** copy `open-code-review.ts`
on 2.x — the 2.x loader requires plugins to default-export
`{ id, effect | setup }` and does not support plugin-defined tools, so the
1.x plugin file can never load there (`Plugin must export a default
definition with an id and an effect or setup function`).

## Install for one project

Run this from the project root:

```bash
mkdir -p .opencode/plugins
curl -fsSL \
  https://raw.githubusercontent.com/alibaba/open-code-review/main/plugins/open-code-review/opencode/open-code-review.ts \
  -o .opencode/plugins/open-code-review.ts
```

Then add `@opencode-ai/plugin` to the project's `.opencode/package.json`
as above. Commit the plugin file if the integration should be shared with
the project.

On OpenCode 2.x, copy `tools/` and `commands/` into the project's
`.opencode/` directory instead (see above) and commit them if the
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
