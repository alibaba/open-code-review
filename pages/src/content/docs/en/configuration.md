---
title: Configuration
sidebar:
  order: 5
---

## Endpoint resolution

When `ocr review` or `ocr llm test` runs, it tries four sources **in order**
and uses the first one that yields a complete `(URL, token, model)` triple:

| Priority | Source | What it reads |
|---|---|---|
| 1 | `~/.opencodereview/config.json` | If `provider` is set, resolves via the `providers`/`custom_providers` maps (provider-first; see [Built-in providers](#built-in-providers)). Only falls back to the legacy `llm` section when no provider is set. |
| 2 | OCR environment variables | `OCR_LLM_URL`, `OCR_LLM_TOKEN`, `OCR_LLM_MODEL`, `OCR_USE_ANTHROPIC`, `OCR_LLM_AUTH_HEADER`. |
| 3 | Claude Code environment variables | `ANTHROPIC_BASE_URL`, `ANTHROPIC_AUTH_TOKEN`, `ANTHROPIC_MODEL`. |
| 4 | Shell rc files | `export ANTHROPIC_*=…` lines parsed out of `~/.zshrc`, `~/.bashrc`, `~/.bash_profile`, `~/.profile`. |

For Claude Code-style sources, if `ANTHROPIC_BASE_URL` lacks a versioned
path (`/v1/...`), OCR appends `/v1/messages` automatically.

If no strategy yields a complete triple, OCR exits with:

```
no valid LLM endpoint configured; one of OCR_LLM_URL/OCR_LLM_TOKEN/OCR_LLM_MODEL,
~/.opencodereview/config.json, or ANTHROPIC_BASE_URL/ANTHROPIC_AUTH_TOKEN/
ANTHROPIC_MODEL must be set
```

> Resolution stops at the first source that **errors**, not just the first
> that's empty. In particular, if `provider` is set in `config.json` but the
> entry is misconfigured (unknown provider name, missing `api_key` with no
> env-var fallback, missing `model`, a custom provider lacking `url`/`protocol`),
> OCR exits with that error and does **not** fall through to the OCR env,
> Claude Code, or rc-file sources. To switch to env-based config, unset the
> `provider` key first.

> Source priority means the **environment overrides nothing** when the
> config file is fully populated. To force the environment to win, either
> delete the relevant `llm.*` keys from `~/.opencodereview/config.json` or
> use `ocr config set` to switch to the new values.

## `ocr config set` — managing `~/.opencodereview/config.json`

```bash
ocr config set <key> <value>
```

`config set` mutates the file via key/value pairs with schema-aware
parsing. The interactive TUI commands `ocr config provider` and
`ocr config model` also write to the same file (see
[Interactive setup](#interactive-setup--ocr-config-provider--ocr-config-model)).
Recognised keys:

| Key | Type | Notes |
|---|---|---|
| `provider` | string | Set the active provider (built-in name or custom). Switching provider clears the model. |
| `model` | string | Set the model for the active provider (stored under the provider entry, or top-level `model` if no provider is set). |
| `providers.<name>.<field>` | varies | Per-provider fields for built-in providers: `api_key`, `url`, `protocol`, `model`, `models`, `auth_header`, `extra_body`. |
| `custom_providers.<name>.<field>` | varies | Same fields as above, for custom (non-built-in) providers. Custom providers must set at least `url` and `protocol`. |
| `llm.url` | string | Endpoint URL. For Anthropic, full Messages URL like `https://api.anthropic.com/v1/messages`. For OpenAI-compatible, the chat-completions URL. |
| `llm.auth_token` | string | API key. Sent as `Authorization: Bearer …` (OpenAI) or, for the legacy Anthropic path, `Authorization: Bearer …` by default (the preset `anthropic` provider defaults to `x-api-key` instead). Use `x-api-key` only by explicitly setting `llm.auth_header`. |
| `llm.auth_header` | string | Auth header name (`x-api-key`, `authorization`, or `bearer`). Anthropic-only; required for some Anthropic setups that need `x-api-key`. |
| `llm.model` | string | Model name. A `[<digits>m]` suffix is stripped automatically. |
| `llm.use_anthropic` | boolean | `true` (default) → Anthropic Messages protocol. `false` → OpenAI Chat Completions. |
| `llm.extra_body` | JSON object | Vendor-specific request fields merged into every chat request body. Example: `'{"thinking":{"type":"disabled"}}'`. |
| `language` | string | Forwarded into a directive appended to the system prompt; defaults to `English` when unset. See [Choosing a language](#choosing-a-language). |
| `telemetry.enabled` | boolean | Master switch for OpenTelemetry export. Off by default. |
| `telemetry.exporter` | string | `console` or `otlp`. |
| `telemetry.otlp_endpoint` | string | OTLP collector address (e.g., `localhost:4317`). |
| `telemetry.content_logging` | boolean | Include LLM prompts / responses in exported event data. |

Examples:

```bash
ocr config set llm.url           https://api.anthropic.com/v1/messages
ocr config set llm.auth_token    sk-ant-xxxxxxxxxx
ocr config set llm.model         claude-opus-4-6
ocr config set llm.use_anthropic true
ocr config set llm.extra_body   '{"thinking":{"type":"disabled"}}'
ocr config set language          English
ocr config set telemetry.enabled true
ocr config set telemetry.exporter otlp
ocr config set telemetry.otlp_endpoint localhost:4317

# Provider-based setup (recommended)
ocr config set provider          anthropic
ocr config set model             claude-opus-4-6
ocr config set providers.anthropic.api_key "$ANTHROPIC_API_KEY"

# Custom (non-built-in) provider
ocr config set provider          my-gateway
ocr config set custom_providers.my-gateway.url      https://gateway.internal.com/v1
ocr config set custom_providers.my-gateway.protocol openai
ocr config set custom_providers.my-gateway.model    llama-3-70b
ocr config set custom_providers.my-gateway.api_key   "$MY_API_KEY"
```

Booleans accept anything Go's `strconv.ParseBool` accepts (`true`, `false`,
`1`, `0`, `t`, `f`, …). `llm.extra_body` must be valid JSON.

## File schema reference

After the commands above, `~/.opencodereview/config.json` looks like:

```json
{
    "llm": {
        "url": "https://api.anthropic.com/v1/messages",
        "auth_token": "sk-ant-xxxxxxxxxx",
        "auth_header": "x-api-key",
        "model": "claude-opus-4-6",
        "use_anthropic": true,
        "extra_body": {
            "thinking": { "type": "disabled" }
        }
    },
    "language": "English",
    "telemetry": {
        "enabled": true,
        "exporter": "otlp",
        "otlp_endpoint": "localhost:4317"
    }
}
```

The provider-based form uses `provider`, `model`, `providers`, and
`custom_providers` instead of the legacy `llm` block:

```json
{
    "provider": "anthropic",
    "model": "claude-opus-4-6",
    "providers": {
        "anthropic": {
            "api_key": "sk-ant-xxxxxxxxxx",
            "model": "claude-opus-4-6"
        }
    },
    "custom_providers": {
        "my-gateway": {
            "url": "https://gateway.internal.com/v1",
            "protocol": "openai",
            "model": "llama-3-70b",
            "models": ["llama-3-70b", "llama-3-8b"],
            "api_key": "gw-xxxxxxxxxx",
            "auth_header": "authorization"
        }
    },
    "language": "English"
}
```

When `provider` is set, the `providers`/`custom_providers` maps drive
resolution; the legacy `llm` section is ignored for that config.

You can edit this file by hand if you prefer, but `ocr config set` will
remarshal with `"    "` indent on the next write.

## Interactive setup — `ocr config provider` / `ocr config model`

For provider and model selection without typing keys, OCR ships two
interactive Bubble Tea TUIs that also mutate `~/.opencodereview/config.json`.

```bash
ocr config provider
ocr config model
```

- `ocr config provider` — Interactive TUI for selecting a built-in or custom
  provider, then entering URL / protocol / API key / model. Saves the choice
  to config and runs `ocr llm test` automatically to verify the endpoint.
  For built-in providers, the API key may be read from the provider's env var
  (see [Built-in providers](#built-in-providers)) when not entered directly.
  Selecting a manual configuration populates the legacy `llm.*` block instead.
- `ocr config model` — Interactive TUI for selecting a model from the current
  provider's preset list, plus any user-added models stored under
  `providers.<name>.models` / `custom_providers.<name>.models`. Requires a
  provider to be set first (`ocr config provider`).

## Built-in providers

The following providers ship with OCR. Each has a preset `BaseURL`,
`Protocol`, and (where applicable) an API key environment variable used as a
fallback when `providers.<name>.api_key` is unset.

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

Any other provider name is treated as custom and must be configured under
`custom_providers` with at least `url` and `protocol`.

## Environment variable reference

| Variable | Purpose |
|---|---|
| `OCR_LLM_URL` | Endpoint URL — same shape as `llm.url`. |
| `OCR_LLM_TOKEN` | API key — same as `llm.auth_token`. |
| `OCR_LLM_MODEL` | Model name. |
| `OCR_LLM_AUTH_HEADER` | Auth header name (`x-api-key`, `authorization`, or `bearer`). Anthropic-only; same as `llm.auth_header`. Defaults to `authorization` when unset. |
| `OCR_USE_ANTHROPIC` | Unset → Anthropic protocol (default). Set to `true` / `1` / `yes` (case-insensitive) → Anthropic. Set to anything else (`false`, `0`, `no`, typos, …) → OpenAI. |
| `ANTHROPIC_BASE_URL` | Claude Code-compatible base URL. |
| `ANTHROPIC_AUTH_TOKEN` | Claude Code-compatible API key. |
| `ANTHROPIC_MODEL` | Claude Code-compatible model. |
| `OCR_ENABLE_TELEMETRY` | `1` to enable telemetry from env. |
| `OTEL_SERVICE_NAME` | Override service name in spans/metrics. |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | OTLP collector address — also forces the exporter to `otlp`. |
| `OTEL_EXPORTER_OTLP_PROTOCOL` | OTLP transport protocol (`grpc`, `http/protobuf`, or `http/json`). Defaults to `grpc`. |
| `OCR_CONTENT_LOGGING` | `1` to include prompts/responses in telemetry events. |

Per-provider API keys (`ANTHROPIC_API_KEY`, `OPENAI_API_KEY`,
`DASHSCOPE_API_KEY`, …) are used as a fallback when a built-in provider's
`api_key` field is unset. See the [Built-in providers](#built-in-providers)
table for each provider's env var name.

## Why `extra_body` exists

Some hosted providers add non-standard fields to the request body (for
example, Bedrock-style `thinking`, vendor-specific `temperature_strategy`,
streaming options). `llm.extra_body` is merged into every outgoing request,
so you can ship those fields without patching the source.

```bash
ocr config set llm.extra_body '{"thinking":{"type":"enabled","budget_tokens":2048}}'
```

## Choosing a language

The `language` key controls one thing only: a directive appended to every
system-role message in the review and `ocr llm test` prompts. The exact
string injected is:

```
\n\nAlways respond in <language>.
```

- *Unset* or empty — treated as `English`.
- `Chinese`, `English`, or any other string — passed through verbatim.

There is no language switching on built-in rule docs. The files embedded
under `internal/config/rules/rule_docs/` are loaded by fixed filename and
are mostly written in Chinese (with `default.md` as an English exception);
they appear in the prompt as-is regardless of the `language` setting. When
`language` is `English`, the prompt therefore contains an English directive
on top of mostly-Chinese rule text — strong models honour the directive and
produce English comments, weaker models may emit mixed output.

`language` has no environment-variable, CLI-flag, or per-project override —
the only place it can be set is the global `~/.opencodereview/config.json`,
via [`ocr config set`](#ocr-config-set--managing-opencodereviewconfigjson):

```bash
ocr config set language English
```

If you need fully English rule text, supply your own rules via `--rule`,
`<repo>/.opencodereview/rule.json`, or `~/.opencodereview/rule.json` (see
[Review Rules](../review-rules/#priority-chain)).

## Per-project vs. global config

The CLI itself is configured globally (`~/.opencodereview/config.json`) —
there is no project-local LLM config. **Review rules** *are* per-project,
however; see [Review Rules](../review-rules/#priority-chain).

## See Also

- [QuickStart](../quickstart/) — minimal setup and first review.
- [CLI Reference](../cli-reference/) — every flag the review command accepts.
- [Telemetry](../telemetry/) — how to wire up OTLP / console exporters.
