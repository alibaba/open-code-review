# MCP Server Integration

OCR review agents can invoke external MCP server tools (e.g. database schema lookup, API doc search) to enrich review context.

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

## Built-in Tools

MCP server tools are registered alongside the following built-in agent tools:

- `file_read` — Reads file content with line ranges
- `code_search` — Searches codebase with regex
- `file_find` — Finds files by pattern
- `file_read_diff` — Reads diff of other changed files
- `code_comment` — Submits structured review comment
