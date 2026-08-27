# MCP Server Integration

OCR review agents can invoke external MCP server tools (e.g. database schema lookup, API doc search) to enrich review context. **Note: MCP server tools are currently loaded in `ocr review` mode only; full-file `ocr scan` mode does not load MCP servers.**

## Configuration Examples

```bash
# stdio transport (sub-process)
ocr config set mcp_servers.docs.type stdio
ocr config set mcp_servers.docs.command npx
ocr config set mcp_servers.docs.args '["@org/docs-mcp-server"]'

# remote transport (Streamable HTTP)
ocr config set mcp_servers.api.type remote
ocr config set mcp_servers.api.url https://mcp.example.com/sse
ocr config set mcp_servers.api.headers '{"Authorization": "Bearer $MCP_TOKEN"}'

# Filter tools with allowlist
ocr config set mcp_servers.docs.tools '["search_docs", "get_schema"]'

# Remove server
ocr config unset mcp_servers.docs
```

## Supported Fields

| Field | Description |
|-------|-------------|
| `type` | `stdio` (default) or `remote` |
| `command` | Sub-process command (stdio) |
| `args` | JSON array of arguments (stdio) |
| `env` | JSON array of `KEY=VALUE` (stdio) |
| `url` | Service endpoint URL (remote; http/https only) |
| `headers` | JSON object; values support `$ENV_VAR` expansion (remote) |
| `tools` | JSON array allowlist filter |
| `setup` | Shell command executed before startup (stdio) |

## Behavioral Details

- `headers` values must be non-empty (validated on `set`); an empty value after `$ENV_VAR` expansion fails at runtime.
- MCP tools whose names collide with built-in tools are skipped with a warning; tools in the `tools` allowlist that do not exist on the server are also warned about.
- `setup` runs in the repository root directory with a 5-minute timeout; on failure it prints an error and skips that server — the review proceeds without it.
- Invalid JSON in `args` / `env` / `tools` / `headers` reports an error.

## Built-in Tools

MCP server tools are registered alongside the following built-in agent tools:

- `file_read` — Reads file content with line ranges
- `code_search` — Searches codebase with regex
- `file_find` — Finds files by pattern
- `file_read_diff` — Reads diff of other changed files
- `code_comment` — Submits structured review comment
