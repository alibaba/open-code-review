# Installation & LLM Configuration

## Installation

```bash
npm install -g @alibaba-group/open-code-review
# or via pnpm
pnpm add -g @alibaba-group/open-code-review
```

## LLM Configuration

If LLM connectivity fails or you need to switch providers:

### Recommended: Interactive Configuration

```bash
ocr config provider
```

TUI guides provider selection, API key entry, model selection, and connectivity test.

### Option: Environment Variables (Suitable for CI / Temporary Setup)

```bash
export OCR_LLM_URL=https://api.anthropic.com/v1/messages
export OCR_LLM_TOKEN=<api-key>
export OCR_LLM_MODEL=claude-opus-4-6
export OCR_LLM_PROTOCOL=anthropic
```

Supported protocols: `anthropic`, `openai`, `openai-responses`.

### Option: Per-Run CLI Overrides (No Global Config Modification Required)

Override provider, model, or per-file token limit directly during execution:

```bash
ocr review --provider openai --model gpt-5.6 --max-tokens 200000 --audience agent --format json
```

### Option: Persistent Configuration

```bash
ocr config set llm.url https://api.anthropic.com/v1/messages
ocr config set llm.auth_token <api-key>
ocr config set llm.model claude-opus-4-6
ocr config set llm.protocol anthropic

# Advanced options
ocr config set llm.retry_codes 429,502,503    # Custom HTTP status codes to auto-retry
ocr config set max_tokens 200000               # Per-file token limit
```

## Resolution Priority (High → Low)

1. **CLI Flags** (`--provider` / `--model` / `--max-tokens`)
2. **Configuration file** (`~/.opencodereview/config.json`) — checks active provider block, then legacy `llm.*` block
3. **`OCR_LLM_*` environment variables**
4. **Claude Code fallback** (`ANTHROPIC_BASE_URL`, `ANTHROPIC_AUTH_TOKEN`, `ANTHROPIC_MODEL`)
5. **Shell rc files** (parses `~/.zshrc`, `~/.bashrc`, `~/.bash_profile`, `~/.profile`)

## Common Commands

> **Do not run `ocr llm test` pre-emptively.** Run review/scan directly; use test commands only when troubleshooting errors.

```bash
ocr llm test          # Test LLM connectivity
ocr llm providers     # List all built-in provider presets
ocr config set language 中文        # Change review comment language (default: English)
```

**Never invent or hardcode API keys.** Stop and ask the user for credentials when missing.
