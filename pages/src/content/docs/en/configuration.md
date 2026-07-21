---
title: Configuration
sidebar:
  order: 5
---

The config file lives at `~/.opencodereview/config.json`. You have three ways
to edit it:

- **Interactive TUI** — `ocr config provider` / `ocr config model`, with guided menus.
- **Command line** — `ocr config set <key> <value>`, ideal for scripts and CI.
- **Manual edit (not recommended)** — the JSON file directly (it gets reformatted on the next `ocr config set` write).

## Configuring a model

### Recommended: interactive setup

```bash
ocr config provider
```

It lets you pick a built-in or custom provider, enter an API key, choose a model, saves everything to the config file, and then runs `ocr llm test` once to verify the endpoint. To switch models later:

```bash
ocr config model
```

### Non-interactive setup (CI / no-TUI environments)

Write to the same config with `ocr config set`:

```bash
ocr config set provider                    anthropic
ocr config set model                       claude-opus-4-6
ocr config set providers.anthropic.api_key sk-ant-xxxxxxxxxx
```

### Built-in providers

The following providers ship with OCR, with the Base URL and protocol
preset — once selected, you only need to fill in the API key. If
`providers.<name>.api_key` is unset, OCR falls back to the corresponding
environment variable.

| Name | Protocol | Base URL | API key env var |
|---|---|---|---|
| `anthropic` | anthropic | `https://api.anthropic.com` | `ANTHROPIC_API_KEY` |
| `openai` | openai | `https://api.openai.com/v1` | `OPENAI_API_KEY` |
| `dashscope` | openai | `https://dashscope.aliyuncs.com/compatible-mode/v1` | `DASHSCOPE_API_KEY` |
| `dashscope-tokenplan` | openai | `https://token-plan.cn-beijing.maas.aliyuncs.com/compatible-mode/v1` | `DASHSCOPE_TOKENPLAN_KEY` |
| `volcengine` | openai | `https://ark.cn-beijing.volces.com/api/v3` | `ARK_API_KEY` |
| `deepseek` | openai | `https://api.deepseek.com` | `DEEPSEEK_API_KEY` |
| `tencent-tokenhub` | openai | `https://tokenhub.tencentmaas.com/v1` | `TENCENT_TOKENHUB_API_KEY` |
| `hy-tokenplan` | openai | `https://api.lkeap.cloud.tencent.com/plan/v3` | `TENCENT_HUNYUAN_TOKENPLAN_KEY` |
| `kimi` | openai | `https://api.moonshot.cn/v1` | `MOONSHOT_API_KEY` |
| `z-ai` | openai | `https://open.bigmodel.cn/api/paas/v4` | `Z_AI_API_KEY` |
| `mimo` | openai | `https://api.xiaomimimo.com/v1` | `MIMO_API_KEY` |
| `minimax` | openai | `https://api.minimaxi.com/v1` | `MINIMAX_API_KEY` |
| `baidu-qianfan` | openai | `https://qianfan.baidubce.com/v2` | `QIANFAN_API_KEY` |

### Custom providers

Any provider name not in the table above is treated as custom and must
supply at least `url` and `protocol` (`protocol` is either `anthropic` or
`openai`):

```bash
ocr config set provider                             my-gateway
ocr config set custom_providers.my-gateway.url      https://gateway.internal.com/v1
ocr config set custom_providers.my-gateway.protocol openai
ocr config set custom_providers.my-gateway.model    llama-3-70b
ocr config set custom_providers.my-gateway.api_key  "$MY_API_KEY"
```

A local model served by Ollama is just a custom provider pointing at the
local OpenAI-compatible endpoint:

```bash
ocr config set provider                          ollama
ocr config set custom_providers.ollama.url       http://127.0.0.1:11434/v1
ocr config set custom_providers.ollama.protocol  openai
ocr config set custom_providers.ollama.model     qwen3:32b
ocr config set custom_providers.ollama.api_key   ollama
```

Ollama ignores the API key, but custom providers require a non-empty
`api_key` (there is no environment-variable fallback for them), so set
any placeholder value. The model itself must support native tool
calling — see
["No tool calls parsed" (local models / Ollama)](../faq/#no-tool-calls-parsed-local-models-ollama)
in the FAQ before picking one.

### Timeouts

Each LLM request has an HTTP timeout, defaulting to **300 seconds**.
Slow local models (or large files) can need more. Three knobs, in
increasing scope:

- `providers.<name>.timeout_sec` / `custom_providers.<name>.timeout_sec`
  — per-provider, in seconds.
- `llm.timeout_sec` — for the legacy `llm` section, in seconds.
- `OCR_LLM_TIMEOUT` environment variable — integer seconds; overrides
  the config-file value for every resolution path.

The `timeout_sec` keys are not supported by `ocr config set` — edit
`~/.opencodereview/config.json` directly:

```json
{
  "custom_providers": {
    "ollama": { "url": "http://127.0.0.1:11434/v1", "protocol": "openai", "timeout_sec": 900 }
  }
}
```

### Verify connectivity

```bash
ocr llm test
```

### Reuse existing environment variables

If you already have Claude Code's `ANTHROPIC_*` or OCR's own `OCR_LLM_*`
environment variables configured, OCR picks them up automatically — no
config file needed.

### Using CC-Switch

If you use [CC-Switch](https://github.com/farion1231/cc-switch) with its
[routing service](https://www.ccswitch.io/en/docs?section=proxy&item=service)
enabled, point the provider `url` at the local proxy — no other setup is
required:

```bash
# Claude (Anthropic-compatible)
ocr config set providers.anthropic.url http://127.0.0.1:15721

# Codex / OpenAI-compatible — set that provider's url key instead
ocr config set providers.<name>.url http://127.0.0.1:15721/v1
```

`api_key` can be any value. `extra_body` (and other per-provider fields)
still apply as usual.

### Send vendor-specific fields

Some providers require non-standard request fields (such as Bedrock-style
`thinking`). Use `extra_body` (merged into every request) to send them
without patching the source:

```bash
ocr config set providers.anthropic.extra_body '{"thinking":{"type":"disabled"}}'
```

## Configuring the review language

`language` determines which language review comments are written in;
it defaults to English when unset:

```bash
ocr config set language 中文
ocr config set language English
```

## See Also

- [QuickStart](../quickstart/) — minimal setup and first review.
- [CLI Reference](../cli-reference/) — every flag the review command accepts.
