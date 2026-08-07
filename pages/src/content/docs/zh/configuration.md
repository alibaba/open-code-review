---
title: 配置
sidebar:
  order: 5
---

配置文件在 `~/.opencodereview/config.json`，你有三种方式编辑它：

- **交互式 TUI** —— `ocr config provider` / `ocr config model`，带引导菜单。
- **命令行** —— `ocr config set <key> <value>`，适合脚本与 CI。
- **手动编辑（不推荐）** —— 该 JSON 文件（下次 `ocr config set` 写入时会重新格式化）。

## 配置模型

### 推荐：交互式设置

```bash
ocr config provider
```

它会让你选择一个内置或自定义 provider、填入 API key、挑选 model，保存到配置文件后自动运行一次 `ocr llm test` 验证端点。之后想换模型：

```bash
ocr config model
```

### 非交互设置（CI / 无 TUI 环境）

用 `ocr config set` 写入同一份配置：

```bash
ocr config set provider                    anthropic
ocr config set model                       claude-opus-4-6
ocr config set providers.anthropic.api_key sk-ant-xxxxxxxxxx
```

### 内置 provider

下列 provider 随 OCR 发布，已预置 Base URL 与协议，选中后只需填 API key。
若 `providers.<name>.api_key` 未设置，会自动回退到对应的环境变量。

| 名称 | 协议 | Base URL | API key 环境变量 |
|---|---|---|---|
| `anthropic` | anthropic | `https://api.anthropic.com` | `ANTHROPIC_API_KEY` |
| `openai` | openai | `https://api.openai.com/v1` | `OPENAI_API_KEY` |
| `dashscope` | openai | `https://dashscope.aliyuncs.com/compatible-mode/v1` | `DASHSCOPE_API_KEY` |
| `dashscope-tokenplan` | openai | `https://token-plan.cn-beijing.maas.aliyuncs.com/compatible-mode/v1` | `DASHSCOPE_TOKENPLAN_KEY` |
| `volcengine` | openai | `https://ark.cn-beijing.volces.com/api/v3` | `ARK_API_KEY` |
| `deepseek` | openai | `https://api.deepseek.com` | `DEEPSEEK_API_KEY` |
| `tencent-tokenhub` | openai | `https://tokenhub.tencentmaas.com/v1` | `TENCENT_TOKENHUB_API_KEY` |
| `hy-tokenplan` | openai | `https://api.lkeap.cloud.tencent.com/plan/v3` | `TENCENT_HUNYUAN_TOKENPLAN_KEY` |
| `iflytek` | openai | `https://spark-api-open.xf-yun.com/v1` | `SPARK_API_KEY` |
| `kimi` | openai | `https://api.moonshot.cn/v1` | `MOONSHOT_API_KEY` |
| `z-ai` | openai | `https://open.bigmodel.cn/api/paas/v4` | `Z_AI_API_KEY` |
| `mimo` | openai | `https://api.xiaomimimo.com/v1` | `MIMO_API_KEY` |
| `minimax` | openai | `https://api.minimax.io/v1` | `MINIMAX_GLOBAL_API_KEY` |
| `minimax-cn` | openai | `https://api.minimaxi.com/v1` | `MINIMAX_API_KEY` |
| `baidu-qianfan` | openai | `https://qianfan.baidubce.com/v2` | `QIANFAN_API_KEY` |
| `siliconflow` | openai | `https://api.siliconflow.com/v1` | `SILICONFLOW_GLOBAL_API_KEY` |

### 自定义 provider

任何不在上表中的 provider 名都视为自定义，至少要提供 `url` 和 `protocol`
（`protocol` 取 `anthropic`、`openai` 或 `openai-responses`）：

```bash
ocr config set provider                             my-gateway
ocr config set custom_providers.my-gateway.url      https://gateway.internal.com/v1
ocr config set custom_providers.my-gateway.protocol openai
ocr config set custom_providers.my-gateway.model    llama-3-70b
ocr config set custom_providers.my-gateway.api_key  "$MY_API_KEY"
```

当 provider 或模型要求使用 OpenAI Responses API（`/v1/responses`）时，使用
`openai-responses` 协议：

```bash
ocr config set provider                                               openai-responses-gateway
ocr config set custom_providers.openai-responses-gateway.url          https://api.openai.com/v1
ocr config set custom_providers.openai-responses-gateway.protocol     openai-responses
ocr config set custom_providers.openai-responses-gateway.model        gpt-5
ocr config set custom_providers.openai-responses-gateway.api_key      "$OPENAI_API_KEY"
```

`url` 既可以填 API 的 Base URL，也可以填完整的 `/responses` 端点，OCR 会自动归一化处理。

用 Ollama 跑本地模型，就是一个指向本地 OpenAI 兼容端点的自定义 provider：

```bash
ocr config set provider                          ollama
ocr config set custom_providers.ollama.url       http://127.0.0.1:11434/v1
ocr config set custom_providers.ollama.protocol  openai
ocr config set custom_providers.ollama.model     qwen3:32b
ocr config set custom_providers.ollama.api_key   ollama
```

Ollama 会忽略 API key，但自定义 provider 要求非空的 `api_key`（自定义
provider 没有环境变量回退），所以设任意占位值即可。模型本身必须支持原生
工具调用——选型前请先看 FAQ 中的
["No tool calls parsed"（本地模型 / Ollama）](../faq/#no-tool-calls-parsed-本地模型-ollama)。

### 超时

每个 LLM 请求都有 HTTP 超时，默认 **300 秒**。慢的本地模型（或大文件）可能
需要更长的时间。三个配置项，作用域递增：

- `providers.<name>.timeout_sec` / `custom_providers.<name>.timeout_sec`
  ——per-provider，单位秒。
- `llm.timeout_sec`——用于旧版 `llm` 配置段，单位秒。
- `OCR_LLM_TIMEOUT` 环境变量——整数秒；对每条解析路径都覆盖配置文件里
  的值。

`ocr config set` 不支持 `timeout_sec` key——直接编辑
`~/.opencodereview/config.json`：

```json
{
  "custom_providers": {
    "ollama": { "url": "http://127.0.0.1:11434/v1", "protocol": "openai", "timeout_sec": 900 }
  }
}
```

### 每文件提示词上限

OCR 默认为每次文件评审设置 58,888 token 的提示词上限。如果模型上下文窗口更大，
可以通过保存 `max_tokens` 来提高上限：

```bash
ocr config set max_tokens 200000
```

该设置同时作用于 `ocr review` 和 `ocr scan`。使用 `--max-tokens` 可以在不修改
已保存配置的情况下临时覆盖一次：

```bash
ocr review --max-tokens 200000
ocr scan --max-tokens 200000
```

单次运行的参数优先级高于 `max_tokens`；如果两者都未设置，OCR 会使用内置任务模板
的默认值。该上限按文件计算，与模型的输出 token 上限、以及限制单次运行总 token
用量的 `--max-tokens-budget` 都相互独立。可以用 `ocr config unset max_tokens`
恢复为内置默认值。

### 验证连通性

```bash
ocr llm test
```

### 复用已有的环境变量

如果你已经配好了 Claude Code 的 `ANTHROPIC_*`，或 OCR 自己的 `OCR_LLM_*`环境变量，OCR 会自动识别，无需再写配置文件。

### 使用 CC-Switch

如果你使用 [CC-Switch](https://github.com/farion1231/cc-switch) 并开启了
[路由服务](https://www.ccswitch.io/zh/docs?section=proxy&item=service)，
可以将供应商的 `url` 配置成 CC-Switch 启动的代理地址，无需额外配置：

```bash
# Claude（Anthropic 兼容）
ocr config set providers.anthropic.url http://127.0.0.1:15721

# Codex / OpenAI 兼容 — 将该供应商的 url 键设为代理地址
ocr config set providers.<name>.url http://127.0.0.1:15721/v1
```

`api_key` 可设置为任意值。`extra_body`（及其他按供应商字段）依然生效。

### 发送厂商专属字段

某些 provider 需要非标准的请求字段（如 Bedrock 风格的 `thinking`）。用`extra_body`（合并进每次请求）即可发送，无需改源码：

```bash
ocr config set providers.anthropic.extra_body '{"thinking":{"type":"disabled"}}'
```

## 配置评审语言

`language` 决定评审评论用哪种语言输出，未设置时默认英文：

```bash
ocr config set language 中文
ocr config set language English
```

## 另见

- [快速开始](../quickstart/)——最小化设置与首次评审。
- [CLI 参考](../cli-reference/)——review 命令接受的每个参数。
