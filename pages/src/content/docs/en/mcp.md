---
title: MCP Servers
sidebar:
  order: 10
---

## Server-first management and import

The manager uses one full-screen terminal session. **CONFIGURED SERVERS** and **MANAGEMENT ACTIONS** are separate sections; **Tab** switches between them. Add, import, edit and permission pages replace the current page instead of accumulating below it. Cancelling returns to the selected item without saving. Use **PgUp/PgDn** to scroll long previews and **Ctrl-C** to exit from any page; exit restores the previous terminal screen. **Q** exits only from navigation pages, so it remains usable in text fields.

`ocr mcp` opens the server list, not an action menu. Select a server, then manage its tools, connection and permissions. **Esc** returns to the parent view; **Q** exits. Actions return to the selected server. Opening the manager never connects. Configuration status is not online status: a discovery result is labelled as the last check in this session, not a live connection.

Select **Tools** to inspect configured tools without connecting. **Test connection** asks before starting or contacting the server and only reads the catalog. Newly discovered tools remain unselected. Each tool shows its effective permission, including parent deny, disabled state and observed definition drift. Revoking a tool works offline. Accepting a new definition reconnects and resets that tool to `ask`. A saved fingerprint is checked again at review startup.

Use **Import (JSON / TOML)** to choose a file or paste a configuration privately. Pasted content is hidden; **Ctrl-S** opens the preview and **Esc** cancels. Cursor JSON uses `mcpServers`; Codex TOML uses `mcp_servers`.

```bash
ocr mcp import [file] [--yes]
ocr mcp import ./mcp.json
ocr mcp import ./one-server.toml --yes
```

Import copies one connection only: disabled, zero tools, no grants, no setup execution and no network calls. Existing names are never overwritten; interactive import lets you rename. Files are limited to 1 MiB and 64 server entries. Non-interactive import requires exactly one server and `--yes`; `-` reads stdin.

Literal env/header values are not copied: environment values become `${VARIABLE_NAME}`, and literal headers become `${OCR_MCP_HEADER_NAME}` references. The preview lists references to set before connecting. Header variables contain the entire header value, including a Bearer prefix when needed. Codex `bearer_token_env_var` is converted to a Bearer header reference. Command arguments and URL connection values are retained, with sensitive values hidden in previews; keep credentials out of them.

OAuth, custom working directories (`cwd`), helper commands and unsupported connection fields are rejected rather than silently ignored. To activate an imported connection, choose **Edit connection**, confirm discovery, select tools and save. The independent execution authorization remains unchanged.

OpenCodeReview can use tools from **Model Context Protocol (MCP)** servers during
`ocr review`. MCP is not loaded by `ocr scan`.

An MCP server is external code and its tool metadata is untrusted. Adding a
server therefore does not expose all of its tools automatically. OCR first
discovers the catalog, then you choose an explicit tool allowlist and an
execution permission.

## The two security gates

Every MCP call must pass two independent gates:

1. **Capability visibility.** The server and global MCP support must be enabled,
   the tool must appear in that server's `tools` allowlist, and its saved
   definition fingerprint must match discovery. Only then can the model see it.
2. **Execution authorization.** Immediately before `tools/call`, OCR checks the
   same server, tool, fingerprint, context, and effective permission again.
   `ask` opens a terminal prompt; `allow` skips only that prompt.

An empty or missing `tools` list means **no tools**, not all tools. Neither an
`allow` permission nor an MCP server's self-declared annotations can add a tool
to the allowlist. A rejection, cancellation, timeout, closed input stream, or
unavailable terminal results in no `tools/call` request.

## Guided setup

Run the manager in a terminal:

```bash
ocr mcp
```

Or start the add wizard directly:

```bash
ocr mcp add docs
```

The wizard:

1. chooses a `stdio` or `remote` connection;
2. collects connection settings without printing stored secret values;
3. shows the exact local executable and argument structure (credential-like
   argument values are redacted), or the redacted remote endpoint and header names;
4. asks before starting or contacting the server;
5. initializes MCP and reads every page of `tools/list` without calling a tool;
6. starts with no tools selected and asks which tools to enable;
7. shows a redacted summary and saves once, atomically, after final confirmation.

Use arrows to select, **Space** to toggle tools, **Enter** to continue, **Ctrl-B**
to return and edit, or **Esc / Ctrl-C** to cancel without saving. No JSON input is
needed: add one argument at a time; supply environment variable names, not tokens.
The connection preview also offers **Save disabled (no connection)**.
Connection failures stay in the wizard so you can edit settings or retry.
Enabled tools start with `ask`; use `ocr mcp permissions docs` to explicitly
choose automatic execution (`allow`). Permission selection uses the same terminal
controls as the manager; inherited `deny` still wins.

Reject the final confirmation or cancel to leave without saving.
Starting an untrusted local process or connecting to a remote server can itself
have server-side effects, even though discovery never invokes a business tool.
Review the connection preview before confirming.

### Local stdio server

```bash
ocr mcp add docs
# Select: stdio
# Executable: npx
# Arguments: enter -y, then your installed MCP package name, then blank
# Environment name: DOCS_TOKEN; source environment variable: DOCS_TOKEN
```

The child receives a small platform environment (`PATH`, home/temp and locale
variables) plus only the entries listed in `env`; it does not inherit the full
OCR process environment. New wizard entries must use `${ENV_NAME}` references.
Historical literal values remain readable for migration but are deprecated and
redacted in output.

OCR displays the executable and argument structure before launch, while redacting
credential-like flag values and URL query values. The server starts
only after confirmation during management and only when at least one tool could
be visible during review.

### Remote server

```bash
ocr mcp add knowledge
# Select: remote
# URL: https://mcp.example.com/v1
# Header name: Authorization; source environment variable: MCP_TOKEN
# Optional prefix: Bearer followed by a space
```

Remote URLs must use HTTPS unless they target localhost/loopback. Plain HTTP to
another host requires the explicit `allow_insecure_http` setting and a separate
warning confirmation. URLs containing user information or fragments are
rejected. Transport-controlled and MCP protocol-reserved headers such as
`Host`, `Content-Length`, MCP session/protocol headers, `Accept`, and
`Content-Type` cannot be overridden. Authentication headers such as
`Authorization` may be configured and are always redacted from output.
New header values entered through the manager must use `${ENV_NAME}` references
(for example, `Bearer ${MCP_TOKEN}`); literal credential values are rejected.

Configured headers are sent only to the original origin. Cross-origin redirects
and HTTPS-to-HTTP redirects are rejected. CLI and JSON status output show only
redacted endpoints and header names, never header values or URL query values.

## Discovery and tool selection

Inspect a catalog without changing configuration:

```bash
ocr mcp discover docs
ocr mcp discover docs --json
```

Discovery performs MCP initialization and paginated `tools/list` only. It does
not call `tools/call`, register tools with the model, or save the catalog. A TTY
asks for confirmation; a non-interactive process must pass `--yes`.

Select tools with the interactive editor or exact non-interactive names:

```bash
ocr mcp tools docs
ocr mcp tools docs --enable search_docs --enable get_page --yes
ocr mcp tools docs --disable get_page --yes
```

Newly enabled tools default to `ask`. Disabling a tool removes its permission
and fingerprint. OCR bounds discovery to 64 pages, 512 tools, 4 MiB per catalog,
8 KiB per description, and 256 KiB per input schema; malformed, duplicate, or
oversized catalogs fail closed.

Tool descriptions, schemas, titles, icons, and annotations are untrusted server
metadata. OCR sanitizes and bounds them. An annotation such as `readOnlyHint`
may be displayed as an unverified hint but never grants permission.

## Permissions

Permissions have these meanings:

| Value | Meaning |
|---|---|
| `deny` | Hard stop. A deny at the global or server level cannot be overridden below it. |
| `ask` | Expose an otherwise eligible tool only in an interactive review and ask before each uncached call. |
| `allow` | Expose and authorize an otherwise eligible tool without a prompt; it does not enable the tool. |
| `inherit` | Server/tool only: use the next broader effective value. |

With no ancestor `deny`, the most specific non-`inherit` value wins:

```text
tool > server > global (default: ask)
```

Persistent `allow` can be written only through the permissions command, and
only for enabled tools with a current discovery fingerprint:

```bash
# Global policy and prompt timeout
ocr mcp permissions
ocr mcp permissions --default ask --timeout 60 --yes

# Server and exact-tool overrides
ocr mcp permissions docs
ocr mcp permissions docs --default inherit \
  --tool search_docs=ask --tool get_page=allow --yes
```

`approval_timeout_seconds` is configurable globally, defaults to 60 seconds,
and accepts 1 through 600. You can also set it directly:

```bash
ocr config set mcp.approval_timeout_seconds 120
```

The change applies to the next `ocr review` run.

### Runtime approval

An `ask` prompt displays the server, original tool name, qualified model alias,
untrusted description, fingerprint prefix, and a bounded/redacted argument
preview. The choices are:

- **Allow once**
- **Allow this review**
- **Deny once** (default)
- **Deny this review**

Review-scoped decisions use the exact server/tool identity, are shared by the
review's concurrent groups, and disappear when the process exits. Runtime
approval never modifies configuration.

## Tool identity and definition changes

The model receives a qualified alias rather than the server's raw name:

```text
mcp__<server-slug>__<tool-slug>__<16-hex>
```

The hash binds the exact, case-sensitive server key and remote tool name. OCR
keeps an immutable alias-to-tool mapping and never reconstructs an identity by
parsing the alias. Any collision with a built-in name, another MCP alias, or an
existing registry entry aborts all MCP registration for that review with zero
partial exposure. Built-in tools remain available.

Configuration saves detect concurrent edits. If another window saved settings
while your form was open, OCR rejects the stale draft; reopen settings and retry.
The private `config.json.lock` sidecar coordinates OCR writers and contains no
credentials. Do not remove it while OCR is running. Older clients and manual
editors do not participate in this lock; avoid concurrent edits with them.

Each selected tool also stores a SHA-256 fingerprint of its connection identity,
name, sanitized description, and canonical input schema. A changed or missing
fingerprint marks the tool `needs-review` and hides it until `ocr mcp tools` or
`ocr mcp edit` accepts the new definition. Server annotations and resolved
secret values are not part of the fingerprint.

## CI and non-interactive use

OCR treats a run as interactive only when both stdin and stderr are terminals,
`TERM` is not `dumb`, and common CI variables are not true. Values such as `0`,
`false`, `no`, and `off` are false; GitHub Actions, GitLab CI, Azure Pipelines,
Buildkite, Jenkins, and generic `CI` environments otherwise fail closed.

- `ask` tools are hidden and cannot execute.
- A matching, explicitly enabled `allow` tool can execute, but still passes the
  independent authorizer immediately before the MCP call.
- `ocr mcp` without a subcommand only prints redacted status/help. It never
  connects or writes configuration.
- Commands that may connect or mutate state require complete flags and `--yes`.

Use persistent `allow` sparingly in CI, scope CI credentials to the minimum
server and tool, and pin or otherwise trust the server package you launch.

## Management command reference

```text
ocr mcp
ocr mcp add [name]
ocr mcp list [--json]
ocr mcp show <name> [--json]
ocr mcp edit <name>
ocr mcp discover <name> [--json] [--yes]
ocr mcp tools <name> [--enable TOOL ...] [--disable TOOL ...] [--yes]
ocr mcp permissions [name]
ocr mcp enable <name> [--yes]
ocr mcp disable <name> [--yes]
ocr mcp remove <name> [--yes]
```

`list` and `show` never connect. `remove` defaults to No in a terminal and
requires `--yes` outside one. `enable` changes only the server switch; it never
adds tools to the allowlist.

## Configuration and migration

The manager writes versioned policy to `~/.opencodereview/config.json`:

The global keys are `mcp.version`, `mcp.enabled`, `mcp.default_permission`, and
`mcp.approval_timeout_seconds`; each named entry lives under `mcp_servers`.

```json
{
  "mcp": {
    "version": 1,
    "enabled": true,
    "default_permission": "ask",
    "approval_timeout_seconds": 60
  },
  "mcp_servers": {
    "docs": {
      "type": "stdio",
      "command": "npx",
      "args": ["-y", "@acme/docs-mcp-server"],
      "env": ["DOCS_TOKEN=${DOCS_TOKEN}"],
      "enabled": true,
      "default_permission": "inherit",
      "tools": ["search_docs"],
      "tool_permissions": {"search_docs": "ask"},
      "tool_definition_sha256": {"search_docs": "<64-lowercase-hex>"}
    }
  }
}
```

Legacy configuration is read but never rewritten during an ordinary review.
A legacy non-empty `tools` list without fingerprints is limited to interactive
`ask`; it cannot auto-run in CI. A legacy missing/empty list exposes nothing.
The first successful manager save writes version 1 and accepted fingerprints.

The old `setup` field is never executed by `ocr review`. When a manager action
saves that server, it removes the legacy field and tells you to install or build
the server manually. Older OCR versions should not edit a migrated config,
because they do not understand these security fields and may drop them.

Configuration writes use a mode-`0600` temporary file in the same directory,
flush it, and atomically replace the old file. Wizard cancellation and failed
discovery leave the original file unchanged.

`0600` is enforced on Unix-like systems. Windows uses directory-inherited ACLs,
not Unix mode bits; keep the configuration directory private to your account.

## Troubleshooting

- **`needs-review`**: run `ocr mcp tools <name>` and accept the discovered
  definition, then choose its permission.
- **No MCP tools in CI**: `ask` is intentionally hidden. Use a current
  fingerprint and a narrowly scoped persistent `allow` if unattended execution
  is required.
- **Discovery fails**: verify the displayed executable/arguments or redacted
  origin, required environment variables, TLS, and server MCP compatibility.
- **Tool result too large**: OCR rejects aggregate results over 1 MiB; narrow the
  query or update the server to return a bounded result.
- **Server reports an error**: an MCP `isError` result is treated as a failed
  execution, not successful telemetry.
- **Name collision**: change the server configuration key or remote tool name;
  OCR never uses first-registration-wins behavior.

## See also

- [Configuration](../configuration/) — all configuration keys.
- [CLI Reference](../cli-reference/) — command and flag reference.
- [CI Integration](../integrations/ci/) — unattended review setup.
- [Tools](../tools/) — built-in review tools.

## Terminal onboarding

`ocr mcp add` offers field-by-field input and a `Space` tool checklist. Use `Enter` to continue, `Ctrl-B` to go back and `Esc` to cancel. Set permissions and the 1–600 second timeout (default 60) with `ocr mcp permissions`. Configuration commands and review use `~/.opencodereview/config.json`. For isolated testing, set `HOME` (and `USERPROFILE` on Windows) to a dedicated test directory for the OCR process. Pure `ocr mcp tools docs --disable write` revocation works offline without connecting or requiring `--yes`; enabling tools still requires discovery and connection consent.

## Transcript and raw logging

When MCP tools are visible in a review, OCR disables raw capture even if `OCR_RAW_LOGGING=1`. Sensitive tool turns retain redacted arguments in session history; their opaque native payload and accompanying text/reasoning are omitted from the transcript. The live model conversation retains its original replay data. MCP subprocesses use the SDK shutdown timeout, and review closes clients concurrently with a bounded wait.
