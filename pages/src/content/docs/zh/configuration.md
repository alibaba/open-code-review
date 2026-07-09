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
| `kimi` | openai | `https://api.moonshot.cn/v1` | `MOONSHOT_API_KEY` |
| `z-ai` | openai | `https://open.bigmodel.cn/api/paas/v4` | `Z_AI_API_KEY` |
| `mimo` | openai | `https://api.xiaomimimo.com/v1` | `MIMO_API_KEY` |
| `minimax` | openai | `https://api.minimaxi.com/v1` | `MINIMAX_API_KEY` |
| `baidu-qianfan` | openai | `https://qianfan.baidubce.com/v2` | `QIANFAN_API_KEY` |

### 自定义 provider

任何不在上表中的 provider 名都视为自定义，至少要提供 `url` 和 `protocol`
（`protocol` 取 `anthropic` 或 `openai`）：

```bash
ocr config set provider                             my-gateway
ocr config set custom_providers.my-gateway.url      https://gateway.internal.com/v1
ocr config set custom_providers.my-gateway.protocol openai
ocr config set custom_providers.my-gateway.model    llama-3-70b
ocr config set custom_providers.my-gateway.api_key  "$MY_API_KEY"
```

### 验证连通性

```bash
ocr llm test
```

### 复用已有的环境变量

如果你已经配好了 Claude Code 的 `ANTHROPIC_*`，或 OCR 自己的 `OCR_LLM_*`环境变量，OCR 会自动识别，无需再写配置文件。

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
