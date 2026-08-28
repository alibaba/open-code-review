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
preset. Most require an API key. If `providers.<name>.api_key` is unset,
OCR falls back to the corresponding environment variable. The `codex`
provider instead uses the ChatGPT credentials created by `ocr auth login`.

| Name | Protocol | Base URL | API key env var |
|---|---|---|---|
| `anthropic` | anthropic | `https://api.anthropic.com` | `ANTHROPIC_API_KEY` |
| `bedrock` | anthropic-bedrock | derived from `aws_region` | — (AWS credential chain) |
| `codex` | openai-responses | `https://chatgpt.com/backend-api/codex` | — (`ocr auth login`) |
| `openai` | openai | `https://api.openai.com/v1` | `OPENAI_API_KEY` |
| `openai-responses` | openai-responses | `https://api.openai.com/v1` | `OPENAI_RESPONSES_API_KEY` |
| `gemini` | openai | `https://generativelanguage.googleapis.com/v1beta/openai` | `GEMINI_API_KEY` |
| `dashscope` | openai | `https://dashscope.aliyuncs.com/compatible-mode/v1` | `DASHSCOPE_API_KEY` |
| `dashscope-tokenplan` | openai | `https://token-plan.cn-beijing.maas.aliyuncs.com/compatible-mode/v1` | `DASHSCOPE_TOKENPLAN_KEY` |
| `volcengine` | openai | `https://ark.cn-beijing.volces.com/api/v3` | `ARK_API_KEY` |
| `deepseek` | openai | `https://api.deepseek.com` | `DEEPSEEK_API_KEY` |
| `tencent-tokenhub` | openai | `https://tokenhub.tencentmaas.com/v1` | `TENCENT_TOKENHUB_API_KEY` |
| `hy-tokenplan` | openai | `https://api.lkeap.cloud.tencent.com/plan/v3` | `TENCENT_HUNYUAN_TOKENPLAN_KEY` |
| `iflytek` | openai | `https://spark-api-open.xf-yun.com/v1` | `SPARK_API_KEY` |
| `kimi` | openai | `https://api.moonshot.cn/v1` | `MOONSHOT_API_KEY` |
| `kimi-global` | openai | `https://api.moonshot.ai/v1` | `MOONSHOT_GLOBAL_API_KEY` |
| `z-ai` | openai | `https://open.bigmodel.cn/api/paas/v4` | `Z_AI_API_KEY` |
| `mimo` | openai | `https://api.xiaomimimo.com/v1` | `MIMO_API_KEY` |
| `minimax` | openai | `https://api.minimax.io/v1` | `MINIMAX_GLOBAL_API_KEY` |
| `minimax-cn` | openai | `https://api.minimaxi.com/v1` | `MINIMAX_API_KEY` |
| `baidu-qianfan` | openai | `https://qianfan.baidubce.com/v2` | `QIANFAN_API_KEY` |
| `siliconflow`  | openai | `https://api.siliconflow.com/v1` | `SILICONFLOW_GLOBAL_API_KEY` |
| `siliconflow-cn`  | openai | `https://api.siliconflow.cn/v1` | `SILICONFLOW_API_KEY` |
| `novita` | openai | `https://api.novita.ai/openai` | `NOVITA_API_KEY` |
| `xai` | openai | `https://api.x.ai/v1` | `XAI_API_KEY` |

### Overriding a built-in provider's Base URL

Every built-in provider has a preset Base URL (shown in the table above).
To point a built-in provider at a different endpoint — for example a
self-hosted LiteLLM gateway that is rarely at the preset default
`http://localhost:4000/v1` — set `providers.<name>.url`:

```bash
ocr config set provider                   litellm
ocr config set model                      openai/gpt-5.4
ocr config set providers.litellm.api_key  "$LITELLM_API_KEY"
ocr config set providers.litellm.url      https://gateway.internal:8000/v1
```

The configured `url` takes precedence over the preset Base URL. When
`providers.<name>.url` is unset (or cleared), OCR falls back to the
preset default — so you only need to set it when your endpoint differs.

### AWS Bedrock

`bedrock` speaks the same Messages API as `anthropic`, but requests are
SigV4-signed from the standard AWS credential chain instead of carrying an API
key, and the region decides the host. There is no `api_key` to set, and none is
accepted as a substitute for a signature:

```bash
ocr config set provider                      bedrock
ocr config set model                         us.anthropic.claude-sonnet-4-6
ocr config set providers.bedrock.aws_region  us-west-2
ocr config set providers.bedrock.aws_profile example-profile
```

| Field | Meaning |
|---|---|
| `providers.bedrock.aws_region` | Region whose `bedrock-runtime` host serves the request. Falls back to `AWS_REGION` or the active profile. |
| `providers.bedrock.aws_profile` | Named profile to resolve credentials from. Falls back to `AWS_PROFILE` or the ambient chain. |

Both fields are optional: left unset, the standard chain decides, as with any
other AWS tool. Pinning them makes a run reproducible without exporting
`AWS_PROFILE` first, which matters most on CI runners carrying a different
default.

Model identifiers are scoped to an account **and** a region, so the list OCR
ships is a starting point rather than a closed set — an inference profile ID or
an application inference profile ARN valid for your account is accepted even
when absent from it. Run `aws bedrock list-inference-profiles --region <region>`
to see what an account offers; a version suffix such as `-v1:0` is invalid for
the newer families.

`ocr llm test` reports the region and profile in place of a URL, because bedrock
has no configured URL — the region decides the host:

```
Source: provider:bedrock
Region: us-east-1
Profile: example-profile
Model:  claude-sonnet-5
✓ Connection test successful
```

Bedrock is **not** available through `llm.protocol` or `OCR_LLM_PROTOCOL`. That
block describes one URL and one token, has nowhere to put a region or a profile,
and bedrock uses neither value it does carry, so the combination is rejected
rather than accepted and ignored.

### Custom providers

Any provider name not in the table above is treated as custom and must
supply at least `url` and `protocol` (`protocol` is `anthropic`,
`openai`, `openai-responses`, or `anthropic-bedrock`):

```bash
ocr config set provider                             my-gateway
ocr config set custom_providers.my-gateway.url      https://gateway.internal.com/v1
ocr config set custom_providers.my-gateway.protocol openai
ocr config set custom_providers.my-gateway.model    llama-3-70b
ocr config set custom_providers.my-gateway.api_key  "$MY_API_KEY"
```

Use `openai-responses` when a provider or model requires the OpenAI
Responses API (`/v1/responses`):

```bash
ocr config set provider                                               openai-responses-gateway
ocr config set custom_providers.openai-responses-gateway.url          https://api.openai.com/v1
ocr config set custom_providers.openai-responses-gateway.protocol     openai-responses
ocr config set custom_providers.openai-responses-gateway.model        gpt-5
ocr config set custom_providers.openai-responses-gateway.api_key      "$OPENAI_API_KEY"
```

A custom provider on the `anthropic-bedrock` protocol needs no `url` — the
region decides the host — and takes the same AWS fields as the built-in. This is
how a second region or profile gets its own entry:

```bash
ocr config set provider                                bedrock-eu
ocr config set custom_providers.bedrock-eu.protocol    anthropic-bedrock
ocr config set custom_providers.bedrock-eu.aws_region  eu-west-1
ocr config set custom_providers.bedrock-eu.aws_profile eu-profile
ocr config set custom_providers.bedrock-eu.model       eu.anthropic.claude-sonnet-4-6
```

The `url` can be either the API base URL or the full `/responses` endpoint — OCR normalizes it either way.

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

### API key from a command

Instead of storing a key in the config file, `api_key_cmd` fetches it at
runtime from a secret manager (1Password, `pass`, `gopass`, …). Its trimmed,
single-line stdout becomes the key. The same option is available for the
legacy `llm` block as `auth_token_cmd`.

```bash
ocr config set providers.anthropic.api_key_cmd "op read op://dev/anthropic/api-key"
```

Your OS keyring works the same way, through the tool it already ships with, so
the key lives in the Keychain or Secret Service rather than in `config.json`:

```bash
# macOS Keychain
ocr config set providers.anthropic.api_key_cmd \
  "security find-generic-password -s ocr-anthropic -w"

# Linux (Secret Service: GNOME Keyring, KWallet, …)
ocr config set providers.anthropic.api_key_cmd \
  "secret-tool lookup service ocr-anthropic"
```

Precedence: a static `api_key` always wins (if both are set, the command is
ignored and a warning is printed); otherwise `api_key_cmd` runs; only if
neither is set does OCR fall back to the provider's environment variable.

The command runs once per `ocr` invocation and must succeed: a non-zero exit,
empty output, multi-line output, or more than 64KiB of output is a hard error
(OCR never silently falls back). It must complete within 60 seconds, which
includes any time you spend answering a prompt. The command inherits your
terminal's stdin and stderr, so interactive prompts (pinentry, Touch ID) both
appear and can be answered. If the command leaves a background daemon holding
its stdout pipe (`gpg-agent`, a first-use `op` daemon), the credential still
arrives but every `ocr` run pauses an extra 5 seconds waiting for that pipe to
close — redirect the daemon's output (`>/dev/null 2>&1`) to get rid of the wait.

On Windows the command runs through `cmd.exe`, not `sh`, so a command written
for one is generally not portable to the other: `%VAR%` and `^` are `cmd.exe`
metacharacters, while `$VAR` expansion and `\` escaping do not apply there.
Quoted arguments are passed through verbatim, so
`op read "op://Private/My Vault/api-key"` works as written.

Since the value is executed as a shell command, `config.json` is trusted
input — keep it owned by you and not writable by anyone else (OCR writes it
with `0600` permissions).

### Additional retry status codes

Some LLM providers use non-standard 4xx status codes for transient errors, such
as returning `403` or `400` for rate limiting. Use `retry_codes` to make OCR
retry these requests using the existing SDK retry mechanism.

`retry_codes` is an array of integers. It can be set as `llm.retry_codes` or
`custom_providers.<name>.retry_codes`. When using `ocr config set`, pass the codes as
a comma-separated list:

```bash
ocr config set llm.retry_codes 403,400
ocr config set custom_providers.my-gateway.retry_codes 403,400
```

Only 4xx HTTP status codes are accepted. `408`, `409`, and `429` are already
retried by the SDK. When read from the config file, these redundant codes are
ignored. When supplied through `ocr config set`, OCR also prints a warning and
omits them from the saved value. All 5xx responses are already retried by the
SDK and cannot be added to `retry_codes`.

### Prompt limit

`max_tokens` is the **prompt** (input) ceiling for a single review unit:
a file group for `ocr review`, a file for `ocr scan`. The embedded
templates default to 200,000 tokens for `ocr review` and 58,888 for
`ocr scan`. Change it for a model with a different context window by
saving `max_tokens`:

```bash
ocr config set max_tokens 400000
```

The setting applies to both `ocr review` and `ocr scan`. Use `--max-tokens`
for a one-off override without changing the saved configuration:

```bash
ocr review --max-tokens 400000
ocr scan --max-tokens 400000
```

The per-run flag takes precedence over `max_tokens`; when neither is set, OCR
uses the embedded task-template default. The limit is independent of the
model's **output** cap (`MAX_COMPLETION_TOKENS`, `16384` in both templates)
and of `--max-tokens-budget`, which caps total token use for a whole run.
Restore the embedded default with `ocr config unset max_tokens`.

### Review effort

`effort` sets how many review rounds each file group gets: `low` = 1,
`medium` (the default) = 2, `high` = 3. More rounds find more issues at
proportionally higher cost.

```bash
ocr config set effort high
ocr config unset effort      # back to the default medium
```

`--effort low|medium|high` overrides the saved value for a single run.

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

### Session affinity for prompt caching

OCR derives a prompt-cache affinity key for every LLM conversation, scoped
to the review session and the task within it
(`<session-id>-<task-type>-<scope-hash>`). Prompt caches match on prefixes,
so per-conversation keys keep each growing conversation (such as a file's
review tool-loop) on a consistent cache node instead of pinning the whole
run to one hot key; the session-ID prefix lets provider-side cache logs be
correlated with `ocr session` records.

To opt in, embed the `{ocr_session_key}` template variable in
`extra_headers` or `extra_body` values wherever your provider expects the
key — OCR substitutes the conversation's key per request and sends nothing
otherwise:

```bash
# By OpenAI-style request body field (e.g. prompt_cache_key)
ocr config set providers.openai.extra_body '{"prompt_cache_key": "{ocr_session_key}"}'

# By HTTP header (e.g. x-session-affinity)
ocr config set custom_providers.my-gateway.extra_headers "x-session-affinity={ocr_session_key}"
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
