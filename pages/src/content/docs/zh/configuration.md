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
| `bedrock` | anthropic-bedrock | 由 `aws_region` 决定 | —（AWS 凭证链） |
| `claude-code` | claude-cli | —（本地 `claude` CLI） | —（CLI 登录） |
| `codex` | codex-cli | —（本地 `codex` CLI） | —（CLI 登录） |
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
| `siliconflow` | openai | `https://api.siliconflow.com/v1` | `SILICONFLOW_GLOBAL_API_KEY` |
| `siliconflow-cn`  | openai | `https://api.siliconflow.cn/v1` | `SILICONFLOW_API_KEY` |
| `novita` | openai | `https://api.novita.ai/openai` | `NOVITA_API_KEY` |
| `xai` | openai | `https://api.x.ai/v1` | `XAI_API_KEY` |

### 覆盖内置 provider 的 Base URL

每个内置 provider 都有一个预设 Base URL（见上表）。要将内置 provider
指向不同的端点——例如自建的 LiteLLM 网关，其地址很少是预设默认值
`http://localhost:4000/v1`——设置 `providers.<name>.url`：

```bash
ocr config set provider                   litellm
ocr config set model                      openai/gpt-5.4
ocr config set providers.litellm.api_key  "$LITELLM_API_KEY"
ocr config set providers.litellm.url      https://gateway.internal:8000/v1
```

配置的 `url` 优先于预设 Base URL。当 `providers.<name>.url` 未设置（或
被清除）时，OCR 回退到预设默认值——因此只需在端点不同时才设置。

### AWS Bedrock

`bedrock` 使用与 `anthropic` 相同的 Messages API，但请求不携带 API key，而是
用标准 AWS 凭证链做 SigV4 签名，主机由区域决定。没有 `api_key` 需要设置，也不
接受用它替代签名：

```bash
ocr config set provider                      bedrock
ocr config set model                         us.anthropic.claude-sonnet-4-6
ocr config set providers.bedrock.aws_region  us-west-2
ocr config set providers.bedrock.aws_profile example-profile
```

| 字段 | 含义 |
|---|---|
| `providers.bedrock.aws_region` | 处理请求的 `bedrock-runtime` 主机所在区域。未设置时回退到 `AWS_REGION` 或当前 profile。 |
| `providers.bedrock.aws_profile` | 解析凭证所用的具名 profile。未设置时回退到 `AWS_PROFILE` 或环境中的凭证链。 |

两者都是可选的：不设置时由标准凭证链决定，与其他 AWS 工具一致。显式固定可以
让运行结果可复现，无需先导出 `AWS_PROFILE`——在默认值不同的 CI runner 上尤为
重要。

模型标识符同时受账号**和**区域限制，因此 OCR 内置的列表只是起点，而非封闭集合：
只要在你的账号中有效，推理配置文件 ID 或应用推理配置文件 ARN 即使不在列表中也
会被接受。运行 `aws bedrock list-inference-profiles --region <region>` 可以查看
账号提供了哪些；注意新系列不接受 `-v1:0` 这类版本后缀。

bedrock 没有配置的 URL——主机由区域决定——所以 `ocr llm test` 显示区域和 profile
而不是 URL：

```
Source: provider:bedrock
Region: us-east-1
Profile: example-profile
Model:  claude-sonnet-5
✓ Connection test successful
```

`llm.protocol` 和 `OCR_LLM_PROTOCOL` **不支持** bedrock。该配置块描述的是一个
URL 加一个 token，没有地方放区域或 profile，而 bedrock 这两个值都不使用，因此
会被明确拒绝，而不是接受后悄悄忽略。

### Claude Code CLI 与 Codex CLI（无需 API key） {#claude-code-cli-and-codex-cli-no-api-key}

OCR 可以在本地安装的 Claude Code 或 Codex CLI 上运行，而不需要 API key。模型通过
你已有的 CLI 登录访问，因此没有 `api_key`、没有 `url`，除了选择 provider 之外无需
保存任何内容。

**前置条件。** CLI 必须已安装并登录：

- `claude-code` 需要 `claude` 二进制文件在 `PATH` 上，并已完成 `claude login`。
- `codex` 需要 `codex` 二进制文件在 `PATH` 上，并已完成 `codex login`。

已在 Claude Code 2.1.261 与 Codex 0.153.2 上测试。

**工作原理。** OCR 自己运行评审循环，只把 CLI 当作模型来调用：它在每轮 agent 循环
中派生一个 CLI 进程，并与之交换请求和回复。CLI 自身的工具保持关闭——Claude Code
在禁用工具的状态下运行，Codex 在只读沙箱中运行——因为驱动评审的是 OCR 而非 CLI。
这会使用你自己的 CLI 登录，自动化使用须遵守各厂商的服务条款。

**配置方式。** 选择 provider 和 CLI 接受的一个 model：

```bash
ocr config set provider claude-code
ocr config set model    claude-sonnet-5
```

```bash
ocr config set provider codex
ocr config set model    gpt-5.5
```

**可选字段。** 两者都是可选的，且仅由 CLI provider 使用：

| 字段 | 含义 |
|---|---|
| `providers.<name>.cli_path` | CLI 二进制文件路径，覆盖在 `PATH` 上查找的名称。 |
| `providers.<name>.cli_args` | 传给每次派生的额外参数，为 JSON 字符串数组，如 `'["--add-dir","/extra"]'`。 |

```bash
ocr config set providers.claude-code.cli_path /opt/claude/bin/claude
ocr config set providers.claude-code.cli_args '["--add-dir","/extra"]'
```

**速率限制。** 并行的评审组意味着并行的 CLI 进程。`--concurrency N`（默认 8）限制
同时运行的数量，是控制在订阅速率限制之内的手段——如果 CLI 开始限流，就调低它。

`ocr llm test` 会打印 `Backend:` 行而非 `URL:`，因为 CLI 后端没有 URL：

```
Source:  provider:claude-code
Backend: claude (local CLI, uses its own login)
Model:   claude-sonnet-5
✓ Connection test successful
```

### 自定义 provider

任何不在上表中的 provider 名都视为自定义，至少要提供 `url` 和 `protocol`
（`protocol` 取 `anthropic`、`openai`、`openai-responses`、
`anthropic-bedrock`、`claude-cli` 或 `codex-cli`）：

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

使用 `anthropic-bedrock` 协议的自定义 provider 不需要 `url`——主机由区域决定——
并且可以使用与内置 provider 相同的 AWS 字段。第二个区域或 profile 就是这样拥有
自己的条目的：

```bash
ocr config set provider                                bedrock-eu
ocr config set custom_providers.bedrock-eu.protocol    anthropic-bedrock
ocr config set custom_providers.bedrock-eu.aws_region  eu-west-1
ocr config set custom_providers.bedrock-eu.aws_profile eu-profile
ocr config set custom_providers.bedrock-eu.model       eu.anthropic.claude-sonnet-4-6
```

使用 `claude-cli` 或 `codex-cli` 协议的自定义 provider 既不需要 `url` 也不需要
`api_key`——它在你自己的登录下运行本地 CLI（见上文
[Claude Code CLI 与 Codex CLI](#claude-code-cli-and-codex-cli-no-api-key)）。仅当
你要为同一个 CLI 运行第二套配置时才使用自定义条目；否则请选用内置的
`claude-code` 或 `codex` provider。

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

### 通过命令获取 API key

除了把 key 直接写进配置文件，还可以用 `api_key_cmd` 在运行时从密钥管理器
（1Password、`pass`、`gopass` 等）获取。命令去除首尾空白后的单行 stdout 即为
key。旧版 `llm` 配置块也有对应的 `auth_token_cmd`。

```bash
ocr config set providers.anthropic.api_key_cmd "op read op://dev/anthropic/api-key"
```

操作系统自带的密钥环同理，直接用系统已有的命令即可，key 保存在 Keychain 或
Secret Service 中，而不是 `config.json` 里：

```bash
# macOS Keychain
ocr config set providers.anthropic.api_key_cmd \
  "security find-generic-password -s ocr-anthropic -w"

# Linux（Secret Service：GNOME Keyring、KWallet 等）
ocr config set providers.anthropic.api_key_cmd \
  "secret-tool lookup service ocr-anthropic"
```

优先级：静态 `api_key` 始终优先（两者都设置时忽略命令并打印警告）；否则运行
`api_key_cmd`；只有两者都未设置时，OCR 才回退到 provider 对应的环境变量。

命令在每次 `ocr` 调用时运行一次，且必须成功：非零退出、空输出、多行输出或超过
64KiB 的输出都会被视为硬错误（OCR 绝不会静默回退）。命令须在 60 秒内完成，这也
包括你回应提示所花的时间。命令会继承你终端的 stdin 和 stderr，因此交互式提示
（pinentry、Touch ID）既能显示也能作答。如果命令留下了仍持有其 stdout 管道的后台
守护进程（`gpg-agent`、首次使用时启动的 `op` 守护进程），凭据依然能取到，但每次
`ocr` 调用都会额外等待 5 秒直到该管道关闭——把守护进程的输出重定向掉
（`>/dev/null 2>&1`）即可消除这段等待。

在 Windows 上命令通过 `cmd.exe` 而非 `sh` 执行，因此为其中一方编写的命令通常
无法直接移植到另一方：`%VAR%` 和 `^` 是 `cmd.exe` 的元字符，而 `$VAR` 展开和 `\`
转义在那里并不适用。带引号的参数会原样传递，因此
`op read "op://Private/My Vault/api-key"` 可以按原样使用。

由于这个值会作为 shell 命令执行，`config.json` 属于可信输入——请确保它归你所有、
其他用户不可写（OCR 写入时使用 `0600` 权限）。

### 额外的重试状态码

有些 LLM 提供商会用非标准的 4xx 状态码表示临时错误，例如在限流时返回 `403` 或
`400`。可通过 `retry_codes` 让 OCR 对这类请求使用 SDK 现有的重试机制。

`retry_codes` 是整数数组，可配置为 `llm.retry_codes` 或
`custom_providers.<name>.retry_codes`。通过 `ocr config set` 设置时，以逗号分隔
传入状态码：

```bash
ocr config set llm.retry_codes 403,400
ocr config set custom_providers.my-gateway.retry_codes 403,400
```

只接受 4xx HTTP 状态码。`408`、`409` 和 `429` 已由 SDK 重试；直接从配置文件
读取时，这些冗余状态码会被忽略。通过 `ocr config set` 设置时，OCR 还会输出
警告，并且不会把这些状态码保存到配置中。所有 5xx 响应也已由 SDK 默认重试，
因此不能加入 `retry_codes`。

### 每文件提示词上限

OCR 默认为 `ocr review` 的每次评审设置 200,000 token 的提示词上限
（`ocr scan` 使用更小的 58,888）。如果模型上下文窗口不同，可以通过保存
`max_tokens` 来调整：

```bash
ocr config set max_tokens 400000
```

该设置同时作用于 `ocr review` 和 `ocr scan`。使用 `--max-tokens` 可以在不修改
已保存配置的情况下临时覆盖一次：

```bash
ocr review --max-tokens 400000
ocr scan --max-tokens 120000
```

单次运行的参数优先级高于 `max_tokens`；如果两者都未设置，OCR 会使用内置任务模板
的默认值。该上限只约束**提示词**：模型的输出上限由单独的
`MAX_COMPLETION_TOKENS`（默认 `16384`）控制，因此调高 `max_tokens` 不会连带放大
输出预算。它同样与限制单次运行总 token 用量的 `--max-tokens-budget` 相互独立。
可以用 `ocr config unset max_tokens` 恢复为内置默认值。

### 评审投入档位（effort）

`effort` 决定每个文件组要跑几轮 main 循环：`low` = 1 轮，`medium` = 2 轮（默认），
`high` = 3 轮。轮数越多召回越高，耗时与 token 消耗也越多。

```bash
ocr config set effort high     # 持久化
ocr review --effort low        # 仅本次运行
ocr config unset effort        # 恢复默认的 medium
```

优先级为：`--effort` 参数 > 已保存的 `effort` > 默认 `medium`。

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

### 提示词缓存的会话亲和性

OCR 为每个 LLM 对话派生一个提示词缓存亲和性密钥，作用域为评审会话及其中的任务（`<会话 ID>-<任务类型>-<作用域哈希>`）。提示词缓存按前缀匹配，因此按对话划分密钥可让每个不断增长的对话（如某个文件的评审工具循环）稳定路由到同一缓存节点，而不是把整次运行压在一个热点密钥上；密钥中的会话 ID 前缀可将供应商侧缓存日志与 `ocr session` 记录对应。

要启用，请在供应商期望的位置——`extra_headers` 或 `extra_body` 的值中——嵌入 `{ocr_session_key}` 模板变量。OCR 会在每个请求中将其替换为该对话的密钥；未配置时不会发送任何内容：

```bash
# 通过 OpenAI 风格的请求体字段传递（例如 prompt_cache_key）
ocr config set providers.openai.extra_body '{"prompt_cache_key": "{ocr_session_key}"}'

# 通过 HTTP 请求头传递（例如 x-session-affinity）
ocr config set custom_providers.my-gateway.extra_headers "x-session-affinity={ocr_session_key}"
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
