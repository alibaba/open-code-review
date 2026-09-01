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

#### Adding a local MCP server

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

#### Adding a remote MCP server

For servers that support **Streamable HTTP**, set `type` to `remote` and
provide a `url` instead of a local command. Setting only `url` is not
enough: the default type is `stdio`.

Use a new server name so these commands do not overwrite an existing
connection:

```bash
ocr config set mcp_servers.search.type remote
ocr config set mcp_servers.search.url https://mcp.example.com/mcp
ocr config set mcp_servers.search.tools '["search", "fetch"]'
```

These commands save the connection in your user config. On the next
review, OCR connects and makes `search` and `fetch` available to the
agent alongside the built-in tools. The tool allowlist keeps any
additional tools the server might offer out of the review. Other
configured servers and your review settings are unchanged.

Once configured, the agent can call these tools during reviews without
asking before each call. Tool arguments — search queries, requested URLs,
and any context the agent includes — leave your machine and reach whoever
operates the endpoint. Because this is user configuration, it applies
across repositories: enable it only where external requests are allowed,
and do not include secrets, private code, or internal URLs in requests.
Review the operator's privacy policy and terms before connecting a
third-party service.

#### Removing an MCP server

Remove a server with `unset`:

```bash
ocr config unset mcp_servers.docs
```

MCP servers live under the `mcp_servers` key in your user config file (`~/.opencodereview/config.json`).

| Field | Type | Required | Description |
|---|---|---|---|
| `type` | string | | `stdio` (default) for a local subprocess, or `remote` for Streamable HTTP. |
| `command` | string | For `stdio` | Executable that starts the MCP server (e.g. `npx`, `uvx`, an absolute path). |
| `args` | string array | | Arguments passed to `command` (`stdio` only). |
| `url` | string | For `remote` | HTTP or HTTPS MCP endpoint. |
| `headers` | object | | HTTP header names and string values (`remote` only). Values expand `$VAR` or `${VAR}` from OCR's environment at connection time; a value that expands to empty **fails the connection** rather than being sent empty or dropped. Omit for anonymous access. |
| `tools` | string array | | Allowlist of tool names to register. Empty = register every tool the server offers. |
| `setup` | string | | Shell command run once before the server starts (`stdio` only, e.g. install deps). Runs in the repo root with a 5-minute timeout. |
| `env` | string array | | Extra subprocess environment variables in `KEY=VALUE` form (`stdio` only). |

For remote servers that require authentication, follow that server's
instructions for `headers`. Use single quotes around JSON passed to
`ocr config set` when it contains environment variable references, so
your shell does not expand them before OCR saves the configuration.
Servers that allow anonymous access need no `headers` at all.

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
- `remote MCP server "x" has no URL configured, skipping` — `type` is
  `remote` but `url` is unset; the mirror image of setting `url` without
  `type`.
- `failed to connect to remote MCP server "x": …` — the endpoint didn't
  connect or list its tools within the 30-second init timeout. Check the
  URL, network access, and any required headers.
- `MCP server "x" header "h" expanded to empty value` — a `$VAR` in
  `headers` isn't set in OCR's environment. This fails the connection; it
  is not treated as an absent header.
- `remote MCP server "x" returned HTTP 401 Unauthorized` — check the token
  or header configuration.
- `remote MCP server "x" returned HTTP 403 Forbidden` — the credentials
  reached the server but lack the required permissions.
- `tool "y" conflicts with built-in tool, skipping` — rename the server's
  tool or drop it from `tools`.
- `allowed tool "y" not found in server's tool list` — the name in `tools`
  doesn't match anything the server offers; check spelling.

A server that fails to start or connect is skipped; the review proceeds
without its tools.

## See also

- [Tools](../tools/) — the six built-in tools MCP tools sit beside.
- [Configuration](../configuration/) — the full config file and every key.
- [CLI Reference](../cli-reference/) — `ocr config` and the review flags.
