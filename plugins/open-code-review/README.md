# Coding agent plugins

Open Code Review ships platform-specific integrations for Claude Code, Codex,
and Cursor. Choose your platform below instead of adapting installation
instructions written for a different agent.

The plugin root also includes an Agent Plugins Specification v1 manifest at
`plugin.json`. Its portable component is the bundled `skills/` directory. The
native `.codex-plugin`, `.cursor-plugin`, and Claude Code manifests remain
separate because they carry host-specific metadata and behavior.

The native Codex MCP configuration stays in `.mcp.json`. It starts
`ocr mcp serve` from the host task's Git worktree, while the portable Agent
Plugins format only provides plugin-root and plugin-data working directories.
The root portable package therefore does not declare that workspace-bound MCP
server.

All integrations require Git 2.41 or later. Install the `ocr` CLI first:

```bash
npm install -g @alibaba-group/open-code-review
```

Configure and test an OCR LLM before running a review, unless you plan to use
[Delegation Mode](https://open-codereview.ai/docs/delegate):

```bash
ocr config provider
ocr config model
ocr llm test
```

## Claude Code

Run these commands inside Claude Code:

```text
/plugin marketplace add alibaba/open-code-review
/plugin install open-code-review@open-code-review
```

This installs the `/open-code-review:review` and
`/open-code-review:delegate-review` slash commands. See the
[Claude Code guide](https://open-codereview.ai/docs/claude-code) for manual
installation, usage, and behavior.

## Codex

Add this repository as a Codex marketplace, then start Codex:

```bash
codex plugin marketplace add alibaba/open-code-review
codex
```

Open `/plugins`, install and enable **Open Code Review**, then start a new task.
The plugin exposes callable review skills backed by the local `ocr` CLI. For
example:

```text
@Open Code Review review my current changes
@Open Code Review review this branch against main
@Open Code Review review and fix high-confidence issues
```

## Cursor

This repository includes a Cursor plugin manifest at
[`.cursor-plugin/plugin.json`](.cursor-plugin/plugin.json). For a local manual
installation, copy the entire `plugins/open-code-review/` directory to:

```text
~/.cursor/plugins/local/open-code-review/
```

Verify that the manifest is located at
`~/.cursor/plugins/local/open-code-review/.cursor-plugin/plugin.json`, then
restart Cursor or run **Developer: Reload Window**. The plugin provides the
portable OCR review skills from the bundled `skills/` directory.

See the [Cursor plugin documentation](https://cursor.com/docs/plugins) for
plugin loading and management details.
