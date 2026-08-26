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
export OCR_LLM_MODEL=claude-opus-5
export OCR_LLM_PROTOCOL=anthropic

# Optional extended variables
export OCR_LLM_AUTH_HEADER="Authorization: Bearer <api-key>"  # Custom auth header
export OCR_LLM_EXTRA_HEADERS="X-Custom=value,X-Two=v2"         # Extra request headers (comma-separated key=value pairs)
export OCR_USE_ANTHROPIC=1                                    # Force Anthropic protocol (OCR_LLM_PROTOCOL takes priority)
export OCR_LLM_TIMEOUT=120                                    # LLM request timeout (seconds)
export OCR_CONFIG_PATH=/path/to/config.json                   # Custom config path (read-only commands only, e.g. ocr llm test)
```

Supported protocols: `anthropic`, `openai`, `openai-responses`, `anthropic-bedrock` (`openai-responses` used for GPT-5.6 models with `OPENAI_RESPONSES_API_KEY`; `anthropic-bedrock` used for AWS Bedrock via Provider mode with ambient AWS credentials).

### Option: Per-Run CLI Overrides (No Global Config Modification Required)

Override provider, model, tokens, or review effort directly during execution:

```bash
ocr review --provider anthropic --model claude-opus-5 --max-tokens 200000 --effort high --audience agent --format json
```

Model names as listed by `ocr llm providers`.

### Option: Persistent Configuration

```bash
# Recommended: provider-level config (active provider takes priority over legacy llm.*; having both emits a warning)
ocr config set provider anthropic
ocr config set providers.anthropic.api_key <api-key>
ocr config set providers.anthropic.url https://api.anthropic.com/v1/messages
ocr config set providers.anthropic.model claude-opus-5
ocr config set providers.anthropic.protocol anthropic

# Custom provider (e.g. OpenAI-compatible gateway)
ocr config set custom_providers.my-gateway.url https://gateway.internal.com/v1
ocr config set custom_providers.my-gateway.protocol openai
ocr config set custom_providers.my-gateway.api_key <api-key>

# Legacy llm.* keys
ocr config set llm.url https://api.anthropic.com/v1/messages
ocr config set llm.auth_token <api-key>
ocr config set llm.model claude-opus-5
ocr config set llm.protocol anthropic

# Advanced options (provider fields: api_key, api_key_cmd, url, protocol, model, models, auth_header, extra_body, extra_headers, retry_codes, aws_region, aws_profile)
ocr config set providers.openai.api_key_cmd "vault read -field=key secret/llm"  # Dynamic API Key resolution via command
ocr config set providers.bedrock.aws_region us-east-1      # AWS Bedrock region
ocr config set providers.bedrock.aws_profile default       # AWS Bedrock profile
ocr config set llm.retry_codes 403,400         # Add custom 4xx retry codes (4xx only; 408/409/429 and all 5xx are already retried by the SDK)
ocr config set llm.auth_header "Bearer <key>"  # Custom auth header
ocr config set llm.extra_headers "X-Custom=v"  # Extra request headers (comma-separated key=value; reserved headers forbidden)
ocr config set llm.extra_body '{"prompt_cache_key": "{ocr_session_key}"}'  # Extra request body fields (supports {ocr_session_key} macro for gateway prompt cache affinity)
ocr config set max_tokens 200000               # Per-file token limit (overrides template defaults: review 200000, scan 58888)
ocr config set effort high                     # Review effort preset (low = 1 round, medium = 2 rounds [default], high = 3 rounds)
```

## Resolution Priority (High → Low)

1. **CLI Flags** (`--provider` / `--model` / `--max-tokens` / `--effort`)
2. **Configuration file** (`~/.opencodereview/config.json`) — checks active provider block, then legacy `llm.*` block
3. **`OCR_LLM_*` environment variables**
4. **Claude Code fallback** (`ANTHROPIC_BASE_URL`, `ANTHROPIC_AUTH_TOKEN`, `ANTHROPIC_MODEL`)
5. **Shell rc files** (parses `~/.zshrc`, `~/.bashrc`, `~/.bash_profile`, `~/.profile`)

## Common Commands

> **Do not run `ocr llm test` pre-emptively.** Run review/scan directly; use test commands only when troubleshooting errors.

```bash
ocr llm test          # Test LLM connectivity
ocr llm providers     # List all 28 built-in provider presets (supports AWS Bedrock, OpenAI Responses, Gemini, xAI, SiliconFlow, Kimi, DashScope, MiniMax, etc.)
ocr config provider   # Interactive provider configuration (TUI)
ocr config model      # Interactive model selection (TUI)
ocr config set effort low          # Change default review effort preset (low, medium, high)
ocr config unset effort            # Reset review effort to default medium
ocr config unset provider          # Clear active provider and model
ocr config unset max_tokens        # Reset per-file token limit to default (review 200000, scan 58888)
ocr config unset custom_providers.my-gateway  # Remove an entire custom provider
ocr config unset mcp_servers.docs  # Remove a specific MCP server
ocr config set language 中文       # Change review comment language (default: English)
```
