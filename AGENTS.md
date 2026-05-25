# AGENTS.md

Guide for AI agents working in the OpenCodeReview codebase.

## Project Overview

OpenCodeReview is a Go CLI tool that performs AI-powered code review. It reads git diffs, sends changed files to an LLM via a tool-use agent, and generates structured review comments with line-level precision.

**Key capability**: The agent can read full file contents, search the codebase, and cross-reference other changed files — producing deep contextual reviews rather than isolated diff scanning.

## Essential Commands

```bash
# Build
make build              # Build for current platform → dist/opencodereview

# Test
make test               # Run all tests with race detection

# Cross-compile
make build-all          # Build linux/amd64, linux/arm64, darwin/amd64, darwin/arm64
make dist               # Full release: clean → build-all → checksums

# Run locally (after build)
./dist/opencodereview --staged    # Review staged changes

# Install from source
make build && sudo cp dist/opencodereview /usr/local/bin/ocr
```

## Architecture & Control Flow

### Entry Point
`cmd/opencodereview/main.go` dispatches subcommands:
- `review` / `r` → `runReview()` in `review_cmd.go`
- `config` → config management
- `llm` → LLM utilities (connectivity test)
- `viewer` → WebUI session viewer

### Review Pipeline (review_cmd.go → agent.go)

1. **Config Loading**: Template, system rules, tool definitions loaded from embedded JSON
2. **Diff Parsing**: Git diff parsed into `model.Diff` structs via `internal/diff/git.go`
3. **Agent Execution**: `agent.Run()` orchestrates the three-phase workflow per file:
   - **Phase 1 - Plan**: Risk analysis for changes >50 lines (skipped for small diffs)
   - **Phase 2 - Main Task Loop**: LLM tool-use conversation until `task_done`
   - **Phase 3 - Memory Compression**: Three-zone partitioning at 60%/80% thresholds
4. **Line Resolution**: `diff.ResolveLineNumbers()` matches `existing_code` to diff hunks
5. **Output**: Comments collected via `tool.CommentCollector`, output as text or JSON

### Key Components

| Package | Purpose |
|---------|---------|
| `internal/agent` | Orchestrates review: diff loading, subtask dispatch, compression |
| `internal/llm` | Dual-protocol client (Anthropic + OpenAI), token counting, retry logic |
| `internal/diff` | Git diff parsing, hunk extraction, line number resolution |
| `internal/tool` | Tool registry, providers for file_read, code_search, code_comment, etc. |
| `internal/config/template` | Prompt templates with placeholder substitution |
| `internal/config/rules` | Path-matched review checklists per file type |
| `internal/config/toolsconfig` | Tool definitions for LLM function calling |
| `internal/session` | Review session history persistence |
| `internal/telemetry` | OpenTelemetry spans/metrics (optional) |

## Dual LLM Protocol Support

The LLM client (`internal/llm/client.go`) supports two APIs:

- **Anthropic Messages API**: `use_anthropic=true`, URL normalized to `/v1/messages`
- **OpenAI Chat Completions**: `use_anthropic=false`, standard `/v1/chat/completions`

Protocol selection is automatic based on `llm.use_anthropic` config. Both protocols share the same `ChatRequest`/`ChatResponse` types with internal conversion.

**Retry behavior**: 10 retries with exponential backoff (1s → 64s max) + jitter. Retryable on 429, 5xx, and network errors.

## Tool Registry Pattern

Tools are registered in `tool.Registry` (map[string]Provider). Each tool implements:
- `Tool() Tool` — returns the tool enum
- `Execute(args map[string]any) (string, error)` — executes with parsed arguments

**Built-in tools**:
- `task_done` — terminates review loop
- `code_comment` — submits review comment to collector
- `file_read` — reads file content at line range
- `file_find` — finds files by name pattern
- `file_read_diff` — reads diff content for other changed files
- `code_search` — searches text/regex across files

Tool definitions loaded from `internal/config/toolsconfig/tools.json` (embedded). Definitions are split by phase: `plan_task` vs `main_task`.

## Embedded Configuration

Configs are embedded via `//go:embed`:
- `internal/config/template/task_template.json` — prompt templates
- `internal/config/rules/system_rules.json` — review checklists per file glob
- `internal/config/toolsconfig/tools.json` — tool definitions
- `internal/config/allowlist/supported_file_types.json` — allowed extensions
- `internal/config/allowlist/default_exclude_patterns.json` — excluded paths

User can override with `--rule`, `--tools` flags.

## Template Placeholders

Templates use `{{placeholder}}` syntax. Key placeholders:

| Placeholder | Injected Value |
|-------------|----------------|
| `{{current_file_path}}` | File being reviewed |
| `{{diff}}` | Unified diff content |
| `{{change_files}}` | List of other changed files |
| `{{system_rule}}` | Path-matched review checklist |
| `{{plan_guidance}}` | Plan phase output (or empty) |
| `{{requirement_background}}` | Optional user-provided context |
| `{{current_system_date_time}}` | Timestamp |
| `{{plan_tools}}` | Tool descriptions for plan phase |

Placeholder substitution in `agent.executeSubtask()` and `executePlanPhase()`.

## Memory Compression

Three-zone compression when context exceeds thresholds:
- **Frozen zone** (first 2 messages): Always preserved
- **Compress zone**: Summarized via `MEMORY_COMPRESSION_TASK`
- **Active zone**: Most recent rounds preserved intact

Thresholds:
- 60% of `MAX_TOKENS`: Trigger async background compression
- 80% of `MAX_TOKENS`: Immediate synchronous compression

Compression logic in `agent.runCompression()`, `partitionMessages()`, `groupIntoRounds()`.

## Line Number Resolution

`diff.ResolveLineNumbers()` populates `StartLine/EndLine` on comments:

1. **Primary**: Match `existing_code` against diff hunks (new-side first, then old-side)
2. **Fallback**: Scan full `NewFileContent` line-by-line
3. **Normalization**: Strip whitespace, remove diff markers (`+`, `-`)

This enables precise line-level comments despite LLM not knowing exact line numbers.

## Concurrency Model

- Per-file reviews run concurrently via semaphore-bounded goroutines
- Default: 8 workers (`--concurrency` flag)
- Timeout per file: `--timeout` minutes (default 10)
- Comment post-processing uses separate `CommentWorkerPool` (async off critical path)

## Testing Patterns

Standard Go testing:
- Table-driven tests for unit logic (see `resolver_test.go`, `hunk_test.go`)
- Integration tests use raw diff strings
- Tests focus on edge cases: whitespace tolerance, multi-hunk matching, fallback scenarios

Run: `make test` (includes race detection)

## Important Gotchas

### Config Location
User config: `~/.open-code-review/config.json`. Environment variables override config file.

### Protocol Selection
Must set `llm.use_anthropic=true` for Anthropic API. Default is OpenAI-compatible.

### Token Encoding
Uses tiktoken with encoding selection:
- `o200k_base` for o1/o3/o4 models
- `cl100k_base` for others (default)

### File Filtering
- Only review files with extensions in `supported_file_types.json`
- Exclude paths matching `default_exclude_patterns.json`
- Pre-filter diffs exceeding 80% of `MAX_TOKENS`

### Comment Collection
`code_comment` tool adds to `CommentCollector`. Comments are collected after all subtasks complete, not during execution.

### Session History
Each review creates a session in `internal/session/`. Records plan/main/compression task messages for debugging.

### Anthropic Response Parsing
Anthropic returns `content` as array of blocks (`text`, `tool_use`). Parsed in `AnthropicClient.parseResponse()`. Tool calls extracted from `tool_use` blocks.

### OpenAI Tool Results
OpenAI format uses `role="tool"` with `tool_call_id`. Anthropic uses `tool_result` content blocks with `tool_use_id`.

## File Organization

```
cmd/
  opencodereview/     # CLI entry point, subcommands
  testdiff/           # Debug utility for diff parsing

internal/
  agent/              # Review orchestration
  llm/                # LLM clients, token counting
  diff/               # Git diff parsing, line resolution
  tool/               # Tool registry, providers
  model/              # Data types (LlmComment, Diff)
  config/
    template/         # Prompt templates
    rules/            # Review checklists
    toolsconfig/      # Tool definitions
    allowlist/        # Extension/path filters
    testconnection/   # LLM connectivity test
  session/            # Review history persistence
  telemetry/          # OpenTelemetry integration
  stdout/             # Output writer abstraction
  viewer/             # WebUI session viewer server
  suggestdiff/        # Diff suggestion utilities

pages/                # Frontend (React WebUI)
scripts/              # NPM install/publish scripts
```

## Common Modifications

### Adding a New Tool
1. Define in `internal/tool/definitions.go` (add to enum, `allTools()`)
2. Implement `Provider` interface in `internal/tool/<name>.go`
3. Add definition to `internal/config/toolsconfig/tools.json`
4. Register in `buildToolRegistry()` in `review_cmd.go`

### Modifying Review Templates
Edit `internal/config/template/task_template.json`. Phases:
- `MAIN_TASK`: Core review loop
- `PLAN_TASK`: Risk analysis (optional)
- `MEMORY_COMPRESSION_TASK`: Context summarization

### Adding File Type Support
Edit `internal/config/allowlist/supported_file_types.json`.

### Adding Exclude Patterns
Edit `internal/config/allowlist/default_exclude_patterns.json`.

## Related Documentation

- `README.md` — User-facing installation and usage
- `README.zh-CN.md` — Chinese translation
- `NPM-README.md` — NPM package README (copied during publish)