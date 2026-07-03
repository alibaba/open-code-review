---
title: MCP Servers
sidebar:
  order: 10
---

OCR can act as a **Model Context Protocol (MCP) client**. You point it at
one or more external MCP servers, and the tools those servers expose
become available to the review agent — right alongside the
[built-in tools](../tools/) like `file_read` and `code_search`.

## When to use it

Reach for an MCP server when the reviewer would benefit from context that
lives outside the diff:

- **Issue / ticket lookup** — let the agent fetch the linked Jira / GitHub
  issue to check whether the change matches the stated requirement.
- **Docs / knowledge base** — pull internal API docs or coding standards
  so comments cite the real house rules.
- **Custom analysis** — expose a linter, a schema validator, or a
  dependency checker as a tool the reviewer can invoke on demand.

If all you need is a plain read of the repo, the built-in tools already
cover it — MCP is for reaching beyond the checkout.

## Configuration

#### Adding an MCP server

The `ocr config set` command writes these fields non-interactively. Array
fields (`args`, `env`, `tools`) take a JSON array string:

```bash
# Minimal: just a command
ocr config set mcp_servers.docs.command npx

# Arguments
ocr config set mcp_servers.docs.args '["-y", "@acme/docs-mcp-server"]'

# Restrict which tools are exposed to the reviewer
ocr config set mcp_servers.docs.tools '["search_docs", "get_page"]'

# A setup command to run before the server starts
ocr config set mcp_servers.docs.setup "npm install -g @acme/docs-mcp-server"

# Environment variables (KEY=VALUE entries)
ocr config set mcp_servers.docs.env '["DOCS_TOKEN=secret", "DOCS_REGION=eu"]'
```

#### Removing an MCP server

Remove a server with `unset`:

```bash
ocr config unset mcp_servers.docs
```

MCP servers live under the `mcp_servers` key in your user config file (`~/.opencodereview/config.json`).

| Field | Type | Required | Description |
|---|---|---|---|
| `command` | string | ✓ | Executable that starts the MCP server (e.g. `npx`, `uvx`, an absolute path). |
| `args` | string array | | Arguments passed to `command`. |
| `tools` | string array | | Allowlist of tool names to register. Empty = register every tool the server offers. |
| `setup` | string | | Shell command run once before the server starts (e.g. install deps). Runs in the repo root with a 5-minute timeout. |
| `env` | string array | | Extra environment variables in `KEY=VALUE` form. |

## Filtering tools

By default every tool a server advertises is registered. Set `tools` to an
allowlist when a server exposes more than the reviewer needs — fewer,
sharper tools keep the agent focused and cut token cost. Names in the list
that the server doesn't actually offer are skipped with a warning, so a
typo surfaces on stderr rather than silently doing nothing.

## Name conflicts

MCP tool names share one namespace with the built-in tools. If a server
advertises a tool whose name collides with a **built-in/reserved** tool
(`file_read`, `code_search`, `task_done`, …) or with a tool already
registered by another MCP server, OCR **skips** it and logs a warning.
First registration wins; give servers distinct tool names to avoid losing
tools this way.

## The `setup` command

`setup` runs once, before the server subprocess starts, from the
repository root. Use it to install or build the server on demand:

```json
"setup": "npm install -g @acme/docs-mcp-server"
```

It has a **5-minute timeout**. If it exits non-zero, OCR logs the command,
working directory, and output, then skips that server and proceeds with
the review.

## Troubleshooting

All MCP diagnostics go to **stderr**, prefixed with `[ocr]`, so they never
pollute `--format json` output on stdout:

- `Running setup for MCP server "x": …` — the setup command is executing.
- `failed to start MCP server "x": …` — the subprocess didn't connect
  within the 30-second init timeout, or `command` isn't on `PATH`.
- `tool "y" conflicts with built-in tool, skipping` — rename the server's
  tool or drop it from `tools`.
- `allowed tool "y" not found in server's tool list` — the name in `tools`
  doesn't match anything the server offers; check spelling.

## See also

- [Tools](../tools/) — the six built-in tools MCP tools sit beside.
- [Configuration](../configuration/) — the full config file and every key.
- [CLI Reference](../cli-reference/) — `ocr config` and the review flags.
