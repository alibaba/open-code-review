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

### Verify connectivity

```bash
ocr llm test
```

### Reuse existing environment variables

If you already have Claude Code's `ANTHROPIC_*` or OCR's own `OCR_LLM_*`
environment variables configured, OCR picks them up automatically — no
config file needed.

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
