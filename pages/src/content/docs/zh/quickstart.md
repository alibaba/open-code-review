---
title: 快速开始
sidebar:
  order: 3
---

安装 OCR，连接到任意支持 Anthropic Messages API 或 OpenAI Chat Completions API
的 LLM，然后运行你的第一次代码评审。

## 前置条件

- 一个可用的 **Git** 安装——OCR 以子进程方式驱动 Git 读取 diff。
- 一个兼容 Anthropic 或 OpenAI 的 LLM 的 **API key**。
- 以下之一：
  - **Node.js ≥ 18**（推荐；最低支持 Node 14——通过 NPM 安装）。
  - 或仅用 `curl` + `chmod` 把静态二进制放进 `$PATH`。
  - 或 **Go ≥ 1.25**，如果你偏好从源码构建。

## 第 1 步——安装 CLI

### 方式 A：NPM（推荐）

```bash
npm install -g @alibaba-group/open-code-review
```

NPM 包安装一个小的 wrapper，它在安装时（通过 postinstall hook）为你的
操作系统 / 架构下载正确的二进制。如果运行时二进制缺失，wrapper 会报错而
不会去下载。安装后，你得到一个全局 `ocr` 命令：

```bash
ocr --version
```

### 方式 B：GitHub Release 二进制

从 [releases 页面](https://github.com/alibaba/open-code-review/releases)
选择对应平台的二进制，放进你的 `$PATH`：

```bash
# macOS (Apple Silicon)
curl -Lo ocr https://github.com/alibaba/open-code-review/releases/latest/download/opencodereview-darwin-arm64
chmod +x ocr && sudo mv ocr /usr/local/bin/ocr

# macOS (Intel)
curl -Lo ocr https://github.com/alibaba/open-code-review/releases/latest/download/opencodereview-darwin-amd64
chmod +x ocr && sudo mv ocr /usr/local/bin/ocr

# Linux x86_64
curl -Lo ocr https://github.com/alibaba/open-code-review/releases/latest/download/opencodereview-linux-amd64
chmod +x ocr && sudo mv ocr /usr/local/bin/ocr

# Linux ARM64
curl -Lo ocr https://github.com/alibaba/open-code-review/releases/latest/download/opencodereview-linux-arm64
chmod +x ocr && sudo mv ocr /usr/local/bin/ocr

# Windows (AMD64)
curl -Lo ocr.exe https://github.com/alibaba/open-code-review/releases/latest/download/opencodereview-windows-amd64.exe

# Windows (ARM64)
curl -Lo ocr.exe https://github.com/alibaba/open-code-review/releases/latest/download/opencodereview-windows-arm64.exe
```

### 方式 C：从源码构建

```bash
git clone https://github.com/alibaba/open-code-review.git
cd open-code-review
make build
sudo cp dist/opencodereview /usr/local/bin/ocr
```

> 各安装方式的详情见 [安装](../installation/)，包括 NPM wrapper 如何解析
> 平台二进制。

## 第 2 步——配置 LLM

在能解析出一个完整的 LLM 端点（URL + token + model）之前，OCR 会拒绝运行
评审。它按以下优先级顺序搜索四个来源：

1. `~/.opencodereview/config.json`
2. OCR 专属环境变量（`OCR_LLM_*`）
3. Claude Code 环境变量（`ANTHROPIC_*`）
4. 从你的 shell rc 文件（`~/.zshrc`、`~/.bashrc`、`~/.bash_profile`、
   `~/.profile`）中解析出的 `export ANTHROPIC_*` 行

### 最快路径：`ocr config set`

```bash
ocr config set llm.url           https://api.anthropic.com/v1/messages
ocr config set llm.auth_token    sk-ant-xxxxxxxxxx
ocr config set llm.model         claude-opus-4-6
ocr config set llm.use_anthropic true
```

这些值会持久化到 `~/.opencodereview/config.json`。

### 替代方式：环境变量

优先级最高——适合不想在磁盘上留配置文件的 CI / 容器：

```bash
export OCR_LLM_URL=https://api.anthropic.com/v1/messages
export OCR_LLM_TOKEN=sk-ant-xxxxxxxxxx
export OCR_LLM_MODEL=claude-opus-4-6
export OCR_USE_ANTHROPIC=true   # 默认 true；设为 false 走 OpenAI 协议
```

### 已经在用 Claude Code？

OCR 会自动读取 Claude Code 使用的同一批变量，无需额外配置：

```bash
export ANTHROPIC_BASE_URL=https://api.anthropic.com
export ANTHROPIC_AUTH_TOKEN=sk-ant-xxxxxxxxxx
export ANTHROPIC_MODEL=claude-opus-4-6
```

如果 `ANTHROPIC_BASE_URL` 缺少带版本的路径，OCR 会自动追加
`/v1/messages`。

### 使用 OpenAI 兼容端点？

把 `llm.use_anthropic` 设为 `false`（或 `OCR_USE_ANTHROPIC=false`）：

```bash
ocr config set llm.url           https://api.openai.com/v1/chat/completions
ocr config set llm.auth_token    sk-xxxxxxxxxx
ocr config set llm.model         gpt-4o
ocr config set llm.use_anthropic false
```

> 完整的 key 参考见 [配置](../configuration/)，包括用于厂商专属请求字段
> 的 `llm.extra_body`，以及用于切换评审评论语言的 `language`。

## 第 3 步——测试连通性

```bash
ocr llm test
```

预期输出（模型名会有所不同）：

```
Source: OCR config file
URL:    https://api.anthropic.com/v1/messages
Model:  claude-opus-4-6
Hello! …
```

如果反而报出 `no valid LLM endpoint configured` 这类错误，请重新检查上面的
配置 key。401 / 403 表示 token 错误或已过期。

## 第 4 步——运行第一次评审

进入任意 Git 仓库并运行：

```bash
cd path/to/your-repo

# 工作区模式——评审 staged + unstaged + untracked 变更（默认）
ocr review

# 分支区间——评审 `main..feature-branch`
ocr review --from main --to feature-branch

# 单个 commit——评审该 commit 引入的 diff
ocr review --commit abc123
```

你会看到持续输出的进度信息，最后每个文件出现一条或多条评审评论。

> 工作区模式包含 **untracked** 文件。如果你只想评审已暂存的内容，请先用
> `git add` 选择性暂存。

> 以上三种是基础用法。`ocr review` 的完整参数（并发调优、输出格式、
> audience 模式、背景上下文等）及其他所有子命令（`config`、`rules`、
> `llm test`、`viewer`）见 [CLI 参考](../cli-reference/)。

### 想先看看 *会* 评审什么？

```bash
ocr review --preview         # 工作区
ocr review -c abc123 -p      # commit
```

`--preview` 运行每个过滤步骤但绝不调用 LLM，因此不消耗任何 token。它打印文件列表
及每个文件的状态（`added` / `modified` / `deleted` / `renamed` / `binary`），
对于被排除的文件还会给出原因（`binary`、`unsupported_ext`、`default_path`、
`user_exclude`、`deleted`）。

### 给工具用的 JSON 输出

```bash
ocr review --format json --audience agent > review.json
```

- `--format json` 输出一个机器可读的评论数组，每条含 `path`、`content`、
  `start_line`、`end_line`、`existing_code`、`suggestion_code` 和可选的
  `thinking`。
- `--audience agent` 屏蔽人性化的进度 UI，让 stdout 只剩 JSON / 最终
  摘要——正是上游 agent 或 CI 脚本所需。

## 第 5 步——查看结果

每条评论包含：

| 字段 | 含义 |
|---|---|
| `path` | 该评论所针对的文件。 |
| `content` | 评审评论本身，使用配置的 `language`。 |
| `start_line` / `end_line` | 文件 **新** 版本中的行范围。两者都为 `0` 表示 OCR 无法精确定位评论——问题是真实的，但需自行定位到准确位置。 |
| `existing_code` | 评论所指的 diff 片段。内部用于行解析；在 `start_line` 为 `0` 时有用。 |
| `suggestion_code` | 可选的修复片段。 |
| `thinking` | 可选的模型推理。仅部分模型存在。 |

## 第 6 步——查看历史会话

每次评审都会以 JSONL 转录形式持久化到
`~/.opencodereview/sessions/...`。在本地 Web UI 中浏览它们：

```bash
ocr viewer            # http://localhost:5483
ocr viewer --addr :3000
```

> 完整 UI 介绍见 [会话查看器](../viewer/)。

## 另见

- [CLI 参考](../cli-reference/)——每个子命令、参数与输出模式。
- [评审规则](../review-rules/)——自定义评审内容。
- [集成](../integrations/)——把 OCR 嵌入 Claude Code、Agent skill 或 CI。
- [遥测](../telemetry/)——经 OTLP 上报 trace 与 metrics。
- [FAQ](../faq/)——已知错误与对策。
