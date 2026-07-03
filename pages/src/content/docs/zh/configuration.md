---
title: 配置
sidebar:
  order: 5
---

## 端点解析

当 `ocr review` 或 `ocr llm test` 运行时，它会按顺序尝试四个来源，
并使用第一个能给出完整 `(URL, token, model)` 三元组的来源：

| 优先级 | 来源 | 读取内容 |
|---|---|---|
| 1 | `~/.opencodereview/config.json` | 若设置了 `provider`，则通过 `providers`/`custom_providers` 映射解析（provider 优先；见[内置 provider](#built-in-providers)）。仅当未设置 provider 时才回退到遗留的 `llm` 段。 |
| 2 | OCR 环境变量 | `OCR_LLM_URL`、`OCR_LLM_TOKEN`、`OCR_LLM_MODEL`、`OCR_USE_ANTHROPIC`、`OCR_LLM_AUTH_HEADER`。 |
| 3 | Claude Code 环境变量 | `ANTHROPIC_BASE_URL`、`ANTHROPIC_AUTH_TOKEN`、`ANTHROPIC_MODEL`。 |
| 4 | Shell rc 文件 | 从 `~/.zshrc`、`~/.bashrc`、`~/.bash_profile`、`~/.profile` 中解析出的 `export ANTHROPIC_*=…` 行。 |

对于 Claude Code 风格的来源，若 `ANTHROPIC_BASE_URL` 缺少带版本的路径
（`/v1/...`），OCR 会自动追加 `/v1/messages`。

如果没有一种策略能给出完整三元组，OCR 会以如下信息退出：

```
no valid LLM endpoint configured; one of OCR_LLM_URL/OCR_LLM_TOKEN/OCR_LLM_MODEL,
~/.opencodereview/config.json, or ANTHROPIC_BASE_URL/ANTHROPIC_AUTH_TOKEN/
ANTHROPIC_MODEL must be set
```

> 解析在第一个**报错**的来源处停止，而不仅仅是第一个为空的来源。尤其要注意：
> 如果 `config.json` 中设置了 `provider` 但该条目配置有误（未知的 provider 名、
> 缺少 `api_key` 且无环境变量回退、缺少 `model`、自定义 provider 缺少
> `url`/`protocol`），OCR 会以该错误退出，并**不会**继续回退到 OCR 环境变量、
> Claude Code 或 rc 文件来源。要切换到基于环境变量的配置，请先取消
> 设置 `provider` key。

> 来源优先级意味着当配置文件已完整填充时，**环境变量不会覆盖任何值**。要让
> 环境变量生效，要么从 `~/.opencodereview/config.json` 删除相关 `llm.*` key，
> 要么用 `ocr config set` 切换到新值。

## `ocr config set` ——管理 `~/.opencodereview/config.json`

```bash
ocr config set <key> <value>
```

`config set` 通过 key/value 对修改文件，并做具备 schema 感知的解析。交互式 TUI
命令 `ocr config provider` 和 `ocr config model` 也写入同一个文件（见
[交互式设置](#interactive-setup--ocr-config-provider--ocr-config-model)）。识别的
key：

| Key | 类型 | 说明 |
|---|---|---|
| `provider` | string | 设置当前 provider（内置名或自定义）。切换 provider 会清空 model。 |
| `model` | string | 为当前 provider 设置 model（存在 provider 条目下；若无 provider 则存到顶层 `model`）。 |
| `providers.<name>.<field>` | varies | 内置 provider 的按字段设置：`api_key`、`url`、`protocol`、`model`、`models`、`auth_header`、`extra_body`。 |
| `custom_providers.<name>.<field>` | varies | 同上字段，用于自定义（非内置）provider。自定义 provider 至少要设置 `url` 和 `protocol`。 |
| `llm.url` | string | 端点 URL。Anthropic 用完整的 Messages URL，如 `https://api.anthropic.com/v1/messages`。OpenAI 兼容则用 chat-completions URL。 |
| `llm.auth_token` | string | API key。以 `Authorization: Bearer …` 发送（OpenAI）；遗留 Anthropic 路径默认也是 `Authorization: Bearer …`（预设 `anthropic` provider 改为默认 `x-api-key`）。仅在显式设置 `llm.auth_header` 时才用 `x-api-key`。 |
| `llm.auth_header` | string | Auth header 名（`x-api-key`、`authorization` 或 `bearer`）。仅 Anthropic 用；某些需要 `x-api-key` 的 Anthropic 设置必需。 |
| `llm.model` | string | 模型名。`[<数字>m]` 后缀会被自动去除。 |
| `llm.use_anthropic` | boolean | `true`（默认）→ Anthropic Messages 协议。`false` → OpenAI Chat Completions。 |
| `llm.extra_body` | JSON object | 厂商专属的请求字段，合并进每次 chat 请求体。示例：`'{"thinking":{"type":"disabled"}}'`。 |
| `language` | string | 转发为追加到 system prompt 的指令；未设置时默认 `English`。见[选择语言](#choosing-a-language)。 |
| `telemetry.enabled` | boolean | OpenTelemetry 导出的总开关。默认关闭。 |
| `telemetry.exporter` | string | `console` 或 `otlp`。 |
| `telemetry.otlp_endpoint` | string | OTLP collector 地址（如 `localhost:4317`）。 |
| `telemetry.content_logging` | boolean | 在导出的事件数据中包含 LLM prompt / 响应。 |

示例：

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

# 基于 provider 的设置（推荐）
ocr config set provider          anthropic
ocr config set model             claude-opus-4-6
ocr config set providers.anthropic.api_key "$ANTHROPIC_API_KEY"

# 自定义（非内置）provider
ocr config set provider          my-gateway
ocr config set custom_providers.my-gateway.url      https://gateway.internal.com/v1
ocr config set custom_providers.my-gateway.protocol openai
ocr config set custom_providers.my-gateway.model    llama-3-70b
ocr config set custom_providers.my-gateway.api_key   "$MY_API_KEY"
```

布尔值接受 Go `strconv.ParseBool` 接受的任何形式（`true`、`false`、`1`、`0`、
`t`、`f`……）。`llm.extra_body` 必须是合法 JSON。

## 文件 schema 参考

执行上述命令后，`~/.opencodereview/config.json` 形如：

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

基于 provider 的形式使用 `provider`、`model`、`providers` 和
`custom_providers`，而非遗留的 `llm` 块：

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

当设置了 `provider` 时，由 `providers`/`custom_providers` 映射驱动解析；该配置下
遗留的 `llm` 段被忽略。

你也可以手动编辑此文件，但下次写入时 `ocr config set` 会以 `"    "` 缩进
重新序列化。

## 交互式设置——`ocr config provider` / `ocr config model`

为免去手动键入 key 来选择 provider 和 model，OCR 提供两个交互式 Bubble Tea TUI，
二者同样会修改 `~/.opencodereview/config.json`。

```bash
ocr config provider
ocr config model
```

- `ocr config provider`——选择内置或自定义 provider，并输入 URL / protocol /
  API key / model 的交互式 TUI。选择会保存到 config，并自动运行 `ocr llm test`
  验证端点。对于内置 provider，若未直接输入，API key 可从该 provider 的环境变量
  读取（见[内置 provider](#built-in-providers)）。若选择手动配置，则改为填充遗留的
  `llm.*` 块。
- `ocr config model`——从当前 provider 的预设列表，以及
  `providers.<name>.models` / `custom_providers.<name>.models` 下用户添加的
  model 中选择模型的交互式 TUI。需要先设置 provider（`ocr config provider`）。

## 内置 provider

以下 provider 随 OCR 发布。每个都有预设的 `BaseURL`、`Protocol`，以及
（如适用）一个 API key 环境变量，在 `providers.<name>.api_key` 未设置时作为
回退。

| 名称 | Protocol | Base URL | API key 环境变量 |
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

任何其他 provider 名都被视为自定义，必须在 `custom_providers` 下配置，且至少
要有 `url` 和 `protocol`。

## 环境变量参考

| 变量 | 用途 |
|---|---|
| `OCR_LLM_URL` | 端点 URL——与 `llm.url` 同形。 |
| `OCR_LLM_TOKEN` | API key——与 `llm.auth_token` 相同。 |
| `OCR_LLM_MODEL` | 模型名。 |
| `OCR_LLM_AUTH_HEADER` | Auth header 名（`x-api-key`、`authorization` 或 `bearer`）。仅 Anthropic；与 `llm.auth_header` 相同。未设置时默认 `authorization`。 |
| `OCR_USE_ANTHROPIC` | 未设置 → Anthropic 协议（默认）。设为 `true` / `1` / `yes`（不区分大小写）→ Anthropic。设为其他值（`false`、`0`、`no`、拼写错误……）→ OpenAI。 |
| `ANTHROPIC_BASE_URL` | Claude Code 兼容的 base URL。 |
| `ANTHROPIC_AUTH_TOKEN` | Claude Code 兼容的 API key。 |
| `ANTHROPIC_MODEL` | Claude Code 兼容的 model。 |
| `OCR_ENABLE_TELEMETRY` | `1` 表示从环境变量启用遥测。 |
| `OTEL_SERVICE_NAME` | 覆盖 span/metric 中的 service name。 |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | OTLP collector 地址——同时强制 exporter 为 `otlp`。 |
| `OTEL_EXPORTER_OTLP_PROTOCOL` | OTLP 传输协议（`grpc`、`http/protobuf` 或 `http/json`）。默认 `grpc`。 |
| `OCR_CONTENT_LOGGING` | `1` 表示在遥测事件中包含 prompt/响应。 |

各 provider 的 API key（`ANTHROPIC_API_KEY`、`OPENAI_API_KEY`、
`DASHSCOPE_API_KEY`……）在内置 provider 的 `api_key` 字段未设置时作为回退。
各 provider 的环境变量名见[内置 provider](#built-in-providers)表。

## 为什么有 `extra_body`

一些托管 provider 会在请求体中加入非标准字段（例如 Bedrock 风格的 `thinking`、
厂商专属的 `temperature_strategy`、流式选项）。`llm.extra_body` 会被合并进每个
发出的请求，因此你无需改源码即可发送这些字段。

```bash
ocr config set llm.extra_body '{"thinking":{"type":"enabled","budget_tokens":2048}}'
```

## 选择语言

`language` key 只控制一件事：追加到评审和 `ocr llm test` prompt 中每条
system-role 消息的一条指令。注入的精确字符串是：

```
\n\nAlways respond in <language>.
```

- *未设置*或为空——按 `English` 对待。
- `Chinese`、`English` 或任何其他字符串——原样透传。

内置 rule docs 不支持语言切换。`internal/config/rules/rule_docs/` 下嵌入的文件按
固定文件名加载，多数以中文撰写（`default.md` 是英文例外）；无论 `language`
如何设置，它们都原样出现在 prompt 中。因此当 `language` 设为 `English` 时，prompt
里会是一条英文指令叠加大段中文 rule 文本——强模型会遵从指令产出英文评论，
弱模型可能输出中英混杂的内容。

`language` 没有环境变量、CLI 参数或项目级覆盖——唯一能设置它的地方是全局
`~/.opencodereview/config.json`，通过
[`ocr config set`](#ocr-config-set--managing-opencodereviewconfigjson)：

```bash
ocr config set language English
```

如果你需要纯英文 rule 文本，请通过 `--rule`、`<repo>/.opencodereview/rule.json`
或 `~/.opencodereview/rule.json` 提供自己的规则（见
[评审规则](../review-rules/#priority-chain)）。

## 项目级 vs 全局配置

CLI 本身是全局配置的（`~/.opencodereview/config.json`）——没有项目级 LLM 配置。
但**评审规则**是项目级的；见[评审规则](../review-rules/#priority-chain)。

## 另见

- [快速开始](../quickstart/)——最小化设置与首次评审。
- [CLI 参考](../cli-reference/)——review 命令接受的每个参数。
- [遥测](../telemetry/)——如何接入 OTLP / console exporter。
