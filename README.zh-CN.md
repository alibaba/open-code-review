<div align="center">
  <a href="https://open-codereview.ai">
    <img src="imgs/logo-core.svg" alt="OpenCodeReview logo" width="180" />
  </a>
  <h1>OpenCodeReview</h1>
</div>

<p align="center">
  <a href="https://trendshift.io/repositories/41087" target="_blank">
    <img src="https://trendshift.io/api/badge/trendshift/repositories/41087/weekly?language=Go" alt="alibaba%2Fopen-code-review | Trendshift" style="width: 320px; height: 70px;" width="320" height="70" />
  </a>
</p>
<p align="center">
  <a href="https://www.npmjs.com/package/@alibaba-group/open-code-review"><img alt="npm" src="https://img.shields.io/npm/v/@alibaba-group/open-code-review?style=flat-square" /></a>
  <a href="https://github.com/alibaba/open-code-review/actions/workflows/release.yml"><img alt="Build status" src="https://img.shields.io/github/actions/workflow/status/alibaba/open-code-review/release.yml?style=flat-square" /></a>
  <a href="https://github.com/alibaba/open-code-review/blob/main/LICENSE"><img alt="License" src="https://img.shields.io/github/license/alibaba/open-code-review?style=flat-square" /></a>
  <a href="https://deepwiki.com/alibaba/open-code-review"><img alt="Ask DeepWiki" src="https://deepwiki.com/badge.svg" /></a>
  <a href="https://www.bestpractices.dev/projects/13328"><img alt="OpenSSF Best Practices" src="https://img.shields.io/badge/OpenSSF-Silver-4C566A?style=flat-square" /></a>
</p>
<p align="center">
  <a href="#supported-platforms"><img alt="Windows" src="https://img.shields.io/badge/Windows-supported-blue.svg" /></a>
  <a href="#supported-platforms"><img alt="macOS" src="https://img.shields.io/badge/macOS-supported-blue.svg" /></a>
  <a href="#supported-platforms"><img alt="Linux" src="https://img.shields.io/badge/Linux-supported-blue.svg" /></a>
  <a href="#supported-agents"><img alt="Claude Code" src="https://img.shields.io/badge/Claude_Code-supported-blueviolet.svg" /></a>
  <a href="#supported-agents"><img alt="Codex" src="https://img.shields.io/badge/Codex-supported-blueviolet.svg" /></a>
  <a href="#supported-agents"><img alt="Cursor" src="https://img.shields.io/badge/Cursor-supported-blueviolet.svg" /></a>
</p>
<p align="center">
  <a href="README.md">English</a> | 简体中文 | <a href="README.ja-JP.md">日本語</a> | <a href="README.ko-KR.md">한국어</a> | <a href="README.ru-RU.md">Русский</a>
</p>

---

## Open Code Review 是什么？

Open Code Review 是一款 AI 驱动的代码审查 CLI 工具。它的前身是阿里集团内部官方 AI 代码审查助手，过去两年在内部服务了数万开发者，识别了数百万个代码缺陷。经过大规模充分验证后，我们将其孵化为开源项目，对社区开放。只需配置一个模型端点即可使用。

它读取 Git diff，通过具备工具调用能力的 Agent 将变更文件发送至可配置的 LLM，生成具有行级精度的结构化审查意见。Agent 可以读取完整文件内容、搜索代码库、检查其他变更文件以获取上下文，从而进行深度审查——而非仅停留在表面的 diff 反馈。除了 diff 审查，`ocr scan` 可以审查整个文件，适用于审计不熟悉的代码库或没有有意义 diff 的目录。

访问[官方网站](https://open-codereview.ai)了解更多信息。

![Highlights](imgs/highlights-zh.png)

## 基准测试

> 相比通用 Agent（Claude Code），Open Code Review 在相同底层模型下取得了显著更高的 **准确率（Precision）** 与 **F1 综合得分**，同时仅消耗 **约 1/9 的 token**、审查更快。但召回率（Recall）低于通用 Agent——这是以精准度换取低噪声的设计取舍。

基于真实场景的代码审查基准测试，从 **50** 个热门开源仓库中精选 **200** 个真实的 Pull Request，覆盖 **10** 种编程语言——由 80+ 位资深工程师交叉标注验证（共 **1,505** 个标注缺陷）。

| 指标 | 含义 | 为什么重要 |
|------|------|-----------|
| **F1** | 准确率与召回率的调和均值 | 综合衡量审查质量的最佳单一指标 |
| **准确率 (Precision)** | 报告的问题中真正有效的比例 | 越高 = 误报越少，减少人工确认成本 |
| **召回率 (Recall)** | 真实缺陷中被发现的比例 | 越高 = 漏报越少，更多问题不会遗漏 |
| **平均耗时 (Avg Time)** | 每次审查的实际耗时 | 决定 CI 流水线的等待时间 |
| **平均 Token (Avg Token)** | 每次审查消耗的总 token 数 | 直接影响 API 使用成本 |

![Benchmark](imgs/benchmark-zh.png)

## 为什么选择 Open Code Review？

### 通用 Agent 的局限

如果你深度用过 Claude Code 等通用 Agent + Skills 方案做代码审查，可能对以下问题深有同感：

- **覆盖不全** —— 变更较大时，Agent 倾向于"偷懒"，选择性地审查部分文件，导致遗漏。
- **位置漂移** —— 报告的问题与实际代码位置常常对不上，出现行号或文件偏移。
- **效果不稳定** —— 基于自然语言驱动的 Skills 难以调试，审查质量因提示词的细微差异而大幅波动。

这些问题的根源在于：纯语言驱动的架构缺乏对审查流程的强约束。

### 核心设计：确定性工程 × Agent 混合驱动

Open Code Review 的核心设计理念是将确定性工程与 Agent 结合，各司其职。

**确定性工程——负责强约束**

对代码审查场景中"不能出错"的环节，由工程逻辑而非语言模型来保证：

- **精准的文件筛选** —— 明确哪些文件需要审查、哪些应当过滤，确保真正重要的改动一个不漏。
- **智能的文件打包** —— 将关联文件归并为同一审查单元（例如 `message_en.properties` 与 `message_zh.properties` 会被打包在一起）。每个包会作为 sub-agent 进行任务，它们之间的上下文是隔离的——这一分治策略在超大变更场景下表现更为稳定，同时天然支持并发审查。
- **精细化规则匹配** —— 针对不同文件的特征，匹配对应的审查规则，确保模型的注意力足够聚焦，从源头规避信息噪声的干扰。相比纯语言驱动的规则引导，基于模板引擎的规则匹配行为更稳定、结果更可预期。
- **外挂的定位与反思组件** —— 独立的评论定位模块与评论反思模块，系统性地提升 AI 反馈的位置准确性与内容准确性。

**Agent——负责动态决策**

将 Agent 的优势集中发挥在它真正擅长的地方——动态决策、动态召回上下文：

- **场景化提示词调优** —— 针对代码审查场景深度优化提示词模板，在提升效果的同时有效降低 Token 消耗。
- **场景化工具集沉淀** —— 基于对大量线上数据中工具调用轨迹的深入分析，包括不同工具的调用频率分布、单一工具的重复调用率、新增工具对整体调用链路的影响等多维度分析，从而对通用 Agent 工具集进行取舍与拆分，最终沉淀出一套在代码审查场景下效果更稳定、行为更可预期的专属工具集。

## 如何使用

### 前置条件

- **Git >= 2.41** — Open Code Review 依赖 Git 进行 diff 生成、代码搜索和仓库操作。

### CLI

#### 安装

**通过 NPM 安装（推荐）**

```bash
npm install -g @alibaba-group/open-code-review
```

安装后，`ocr` 命令即可全局使用。

**更新**

如果通过 NPM 安装，可手动更新到最新版本：

```bash
npm install -g @alibaba-group/open-code-review@latest
```

通过 NPM 安装的 `ocr` 还会默认在后台检查新版本并自动升级；如需关闭自动更新，可设置 `OCR_NO_UPDATE=1`。

如果通过安装脚本或手动下载二进制文件安装，重新运行对应的安装/下载命令即可替换为最新 release。需要固定版本时，可继续通过 `OCR_VERSION` 指定 release tag。

**从 GitHub Release 下载**

使用一条命令为你的操作系统/架构安装最新二进制文件（macOS / Linux）：

```bash
curl -fsSL https://raw.githubusercontent.com/alibaba/open-code-review/main/install.sh | sh
```

该脚本会自动选择匹配的发布二进制文件，校验其 SHA-256 校验和，并将其作为 `ocr` 安装到 `/usr/local/bin`。可通过 `OCR_INSTALL_DIR` 覆盖安装目录，或通过 `OCR_VERSION` 指定发布版本：

```bash
OCR_INSTALL_DIR="$HOME/.local/bin" OCR_VERSION=v1.3.13 \
  sh -c "$(curl -fsSL https://raw.githubusercontent.com/alibaba/open-code-review/main/install.sh)"
```

在 Windows 上（PowerShell 5.1+）：

```powershell
irm https://raw.githubusercontent.com/alibaba/open-code-review/main/install.ps1 | iex
```

该脚本会自动选择匹配的 Windows 发布二进制文件，校验其 SHA-256 校验和，并将其作为 `ocr.exe` 安装到 `%LOCALAPPDATA%\Programs\ocr`。可通过 `OCR_INSTALL_DIR` 覆盖安装目录，或通过 `OCR_VERSION` 指定发布版本：

```powershell
$env:OCR_INSTALL_DIR = "$env:USERPROFILE\bin"
$env:OCR_VERSION = "v1.3.13"
irm https://raw.githubusercontent.com/alibaba/open-code-review/main/install.ps1 | iex
```

将远程脚本直接管道到 shell 会执行来自互联网的代码。建议先下载并检查后再运行：

```bash
curl -fsSL https://raw.githubusercontent.com/alibaba/open-code-review/main/install.sh -o install.sh
less install.sh && sh install.sh
```

```powershell
irm https://raw.githubusercontent.com/alibaba/open-code-review/main/install.ps1 -OutFile install.ps1
notepad install.ps1   # 检查后执行: .\install.ps1
```

<details>
<summary>手动下载（所有平台，包括 Windows）</summary>

从 [GitHub Releases](https://github.com/alibaba/open-code-review/releases) 下载适用于你平台的二进制文件：

```bash
# macOS (Apple Silicon)
curl -Lo ocr https://github.com/alibaba/open-code-review/releases/latest/download/opencodereview-darwin-arm64
chmod +x ocr && sudo mv ocr /usr/local/bin/ocr

# macOS (Intel)
curl -Lo ocr https://github.com/alibaba/open-code-review/releases/latest/download/opencodereview-darwin-amd64
chmod +x ocr && sudo mv ocr /usr/local/bin/ocr

# Linux (x86_64)
curl -Lo ocr https://github.com/alibaba/open-code-review/releases/latest/download/opencodereview-linux-amd64
chmod +x ocr && sudo mv ocr /usr/local/bin/ocr

# Linux (ARM64)
curl -Lo ocr https://github.com/alibaba/open-code-review/releases/latest/download/opencodereview-linux-arm64
chmod +x ocr && sudo mv ocr /usr/local/bin/ocr

# Windows (x86_64) — 将 ocr.exe 移动到 PATH 目录中
curl -Lo ocr.exe https://github.com/alibaba/open-code-review/releases/latest/download/opencodereview-windows-amd64.exe

# Windows (ARM64) — 将 ocr.exe 移动到 PATH 目录中
curl -Lo ocr.exe https://github.com/alibaba/open-code-review/releases/latest/download/opencodereview-windows-arm64.exe
```

</details>

**从源码构建**

```bash
git clone https://github.com/alibaba/open-code-review.git
cd open-code-review
make build
sudo cp dist/opencodereview /usr/local/bin/ocr
```

#### 快速开始

**1. 配置 LLM**

**在审查代码之前，必须先配置 LLM。**

OCR 通过**供应商（Provider）**模式统一管理 LLM 配置，内置了多种主流供应商，也支持添加自定义供应商以对接私有部署或其他兼容端点。配置存储于 `~/.opencodereview/config.json`。

**方式 A：交互式设置（推荐）**

```bash
ocr config provider          # 选择内置供应商或添加自定义供应商
ocr config model             # 为当前供应商选择模型
```

![Provider setup](imgs/providers.jpg)

交互式界面会引导你完成供应商选择、API Key 输入和模型配置，完成后自动测试连通性。

运行 `ocr llm providers` 可查看所有内置供应商。内置供应商预设了 API 地址和协议，只需提供 API Key 即可使用。如果对应的环境变量已设置（如 `ANTHROPIC_API_KEY`、`OPENAI_API_KEY`），API Key 会自动读取，无需手动输入。

添加**自定义供应商**同样通过交互式界面完成 —— 需提供供应商名称、API 地址、协议类型（`anthropic` 或 `openai`）和 API Key。

**方式 B：命令行设置（适用于 CI/CD 等无交互环境）**

通过 `ocr config set` 命令直接写入供应商配置，适用于脚本和自动化场景。

使用内置供应商：

```bash
ocr config set provider anthropic
ocr config set providers.anthropic.api_key your-api-key-here
ocr config set providers.anthropic.model claude-sonnet-4-6
```

使用自定义供应商（对接私有网关或其他兼容端点）：

```bash
ocr config set provider my-gateway
ocr config set custom_providers.my-gateway.url https://my-llm-gateway.internal/v1
ocr config set custom_providers.my-gateway.protocol openai
ocr config set custom_providers.my-gateway.api_key your-api-key-here
ocr config set custom_providers.my-gateway.model gpt-4o
```

> 自定义供应商的 `url` 和 `protocol` 为必填项。`protocol` 支持 `anthropic`、`openai`、`openai-responses`。

可选配置项：

| 键 | 描述 |
|----|------|
| `providers.<name>.auth_header` | 认证头：`x-api-key` 或 `authorization`（默认 `authorization`） |
| `providers.<name>.extra_body` | 合并到请求体的自定义 JSON 字段 |
| `providers.<name>.extra_headers` | 逗号分隔的 `key=value` 键值对，为每个请求添加自定义 HTTP 头 |
| `providers.<name>.models` | 用于交互式选择的模型列表 |

**`extra_headers`（可选）：** 为每个 LLM API 请求添加自定义 HTTP 头。适用于代理、网关或需要额外头的企业端点（例如组织 ID、链路追踪 ID）。格式为逗号分隔的 `key=value` 键值对。包含逗号的值请用双引号包裹：

```bash
ocr config set llm.extra_headers "X-Org-ID=org-123,X-Forwarded-For=\"1.2.3.4,5.6.7.8\""
```

也可以按供应商单独设置额外头：

```bash
ocr config set providers.anthropic.extra_headers "X-Org-ID=org-123"
```

**环境变量（优先级最高）**

环境变量会覆盖配置文件中的设置，适用于 CI/CD 场景中不便写入配置文件的情况：

```bash
export OCR_LLM_URL=https://api.anthropic.com/v1/messages
export OCR_LLM_TOKEN=your-api-key-here
export OCR_LLM_MODEL=claude-opus-4-6
export OCR_USE_ANTHROPIC=true
```

若要走 OpenAI Responses API（GPT-5.x / o-系列模型），请改用 `OCR_LLM_PROTOCOL`：

```bash
export OCR_LLM_URL=https://api.openai.com/v1
export OCR_LLM_TOKEN=your-openai-key
export OCR_LLM_MODEL=gpt-5.4
export OCR_LLM_PROTOCOL=openai-responses
```

`OCR_LLM_PROTOCOL` 接受 `anthropic`、`openai`、`openai-responses`，与 `OCR_USE_ANTHROPIC` 同时设置时优先使用前者。

同时兼容 Claude Code 环境变量（`ANTHROPIC_BASE_URL`、`ANTHROPIC_AUTH_TOKEN`、`ANTHROPIC_MODEL`），并解析 `~/.zshrc` / `~/.bashrc` 中的相关导出。

> **CC-Switch 用户特别提醒**：如果你使用 [CC-Switch](https://github.com/farion1231/cc-switch) 并开启了[路由服务](https://www.ccswitch.io/zh/docs?section=proxy&item=service)，可以将供应商的 `url` 配置成 CC-Switch 启动的代理地址，无需额外配置：
> - 路由 **Claude** 供应商：`providers.anthropic.url` 设为 `http://127.0.0.1:15721`
> - 路由 **Codex** 供应商：对应供应商的 `url` 设为 `http://127.0.0.1:15721/v1`
> - `api_key` 可设置为任意值，`extra_body` 设置依然生效

**2. 测试连通性**

```bash
ocr llm test
```

**3. 开始审查**

```bash
cd your-project

# 工作区模式 —— 审查所有暂存、未暂存和未跟踪的变更
ocr review

# 分支范围 —— 比较两个引用
ocr review --from main --to feature-branch

# 单个提交
ocr review --commit abc123

# 恢复中断的区间或单 commit 评审
ocr session list
ocr review --from main --to feature-branch --resume <session-id>

# 全量文件扫描 —— 审查整个文件而非 diff（无需 git 历史）
ocr scan                          # 扫描整个仓库
ocr scan --path internal/agent    # 扫描指定目录或文件

# 委托模式 — 让你的 AI 编程 agent 自己执行评审
# OCR 负责文件选择和规则解析；无需配置 LLM
ocr delegate preview
ocr delegate rule src/main.go src/handler.go
```

### 集成到编程 Agent

OCR 可以无缝集成到 AI 编程 Agent 中，作为斜杠命令使用，在 Agent 工作流中直接进行代码审查。

#### 方式一：作为 Skill 安装

使用 `npx` 将 OCR skill 安装到项目中：

```bash
npx skills add alibaba/open-code-review --skill open-code-review
```

此命令从 [skills 注册表](skills/open-code-review/SKILL.md)安装 `open-code-review` skill，教会你的编程 Agent 如何调用 `ocr` 进行代码审查、按优先级分类问题，并可选择性地应用修复。

**委托模式** — 如果你希望编程 agent 自身执行评审（OCR 仅负责文件选择和规则解析，OCR 侧无需配置 LLM）：

```bash
npx skills add alibaba/open-code-review --skill open-code-review-delegate
```

详见 [skills/open-code-review-delegate/SKILL.md](skills/open-code-review-delegate/SKILL.md)。

#### 方式二：作为 Claude Code Plugin 安装

对于 [Claude Code](https://docs.anthropic.com/en/docs/claude-code)，在 Claude Code 中通过以下命令安装命令插件：

```bash
/plugin marketplace add alibaba/open-code-review
/plugin install open-code-review@open-code-review
```

此命令注册 `/open-code-review:review` 斜杠命令，运行 OCR 并自动过滤和修复问题。同时提供 `/open-code-review:delegate-review` 委托模式命令（agent 使用自身能力进行评审，OCR 负责文件选择和规则解析）。

#### 方式三：作为 Codex Plugin 安装

对于本地 Codex，可以从此仓库安装 Open Code Review plugin：

```bash
codex plugin marketplace add alibaba/open-code-review
codex
/plugins
```

对于本地 checkout 或 fork：

```bash
codex plugin marketplace add .
codex
/plugins
```

安装并启用 `Open Code Review` 后，启动新的 Codex thread 并显式调用：

```text
@Open Code Review review my current changes
@Open Code Review review this branch against main
@Open Code Review review and fix high-confidence issues
```

这会注册一个 Codex skill，用于运行本地 OCR CLI：

```bash
ocr review --audience agent
```

此集成不会改变 OCR 的内部 LLM backend，也不需要为 Codex 配置 OpenAI Responses API endpoint。OCR 本身仍需要按照 CLI setup 部分安装并配置 `ocr` CLI。

韩文指南：[`plugins/open-code-review/CODEX.ko-KR.md`](plugins/open-code-review/CODEX.ko-KR.md)

#### 方式四：作为 Cursor Plugin 安装

对于 [Cursor](https://www.cursor.com/)，可以从此仓库安装 Open Code Review plugin：

```
cursor-plugin marketplace add alibaba/open-code-review
```

也可以手动添加 marketplace。在 Cursor 中打开 `/plugins`，搜索 `Open Code Review` 并安装。

对于本地 checkout 或 fork：

```
cursor-plugin marketplace add .
```

安装后，在 Cursor 中调用：

```text
@Open Code Review review my current changes
@Open Code Review review this branch against main
@Open Code Review review and fix high-confidence issues
```

这会注册一个 Cursor skill，用于运行本地 OCR CLI：

```bash
ocr review --audience agent
```

此集成不会改变 OCR 的内部 LLM backend。OCR 本身仍需要按照 CLI setup 部分安装并配置 `ocr` CLI。

#### 方式五：直接复制命令文件

如果不想使用任何包管理器，可以直接复制命令文件，在 Claude Code 中使用 `/open-code-review` 斜杠命令。

**项目级**（通过 git 与团队共享）：

```bash
mkdir -p .claude/commands
curl -o .claude/commands/open-code-review.md \
  https://raw.githubusercontent.com/alibaba/open-code-review/main/plugins/open-code-review/claude-code/commands/review.md
```

**用户级**（个人全局使用，适用于所有项目）：

```bash
mkdir -p ~/.claude/commands
curl -o ~/.claude/commands/open-code-review.md \
  https://raw.githubusercontent.com/alibaba/open-code-review/main/plugins/open-code-review/claude-code/commands/review.md
```

委托模式（OCR 侧无需配置 LLM）：

```bash
# 项目级
mkdir -p .claude/commands
curl -o .claude/commands/open-code-review-delegate.md \
  https://raw.githubusercontent.com/alibaba/open-code-review/main/plugins/open-code-review/claude-code/commands/delegate-review.md

# 用户级
mkdir -p ~/.claude/commands
curl -o ~/.claude/commands/open-code-review-delegate.md \
  https://raw.githubusercontent.com/alibaba/open-code-review/main/plugins/open-code-review/claude-code/commands/delegate-review.md
```

> **前提条件**：所有集成方式都需要安装 `ocr` CLI。标准模式还需要配置 LLM — 参见上文[安装](#安装)和[配置 LLM](#1-配置-llm)。委托模式 OCR 侧**不需要** LLM 配置。

### CI/CD 集成

OCR 可以集成到 CI/CD 流水线中，在 Merge Request / Pull Request 时自动进行代码审查。

CI 集成的核心命令：

```bash
ocr review \
  --from "origin/main" \
  --to "origin/feature-branch" \
  --format json
```

`--format json` 参数输出适合 CI 脚本解析的机器可读结果。

每条评审结果都带有两个结构化字段，便于 CI 集成在无需解析评论文本的情况下排序、分组、过滤或卡点构建：

| 字段 | 允许的取值 | 说明 |
|------|-----------|------|
| `category` | `bug`、`security`、`performance`、`maintainability`、`test`、`style`、`documentation`、`other` | 问题所属的类别。 |
| `severity` | `critical`、`high`、`medium`、`low` | 问题的严重程度。 |

在 JSON 输出中，这两个字段与 `content`、`start_line` 等平级；在终端中，它们会以内联的 `[category · severity]` 徽章形式显示在评论前，并按严重程度着色。

集成示例请参见 [`examples/`](./examples/) 目录：

- [`github_actions/`](./examples/github_actions/) — GitHub Actions 集成示例
- [`gitlab_ci/`](./examples/gitlab_ci/) — GitLab CI 集成示例
- [`gitflic_ci/`](./examples/gitflic_ci/) — GitFlic CI 集成示例
- [`gerrit_ci/`](./examples/gerrit_ci/) — Gerrit (Jenkins / Gerrit Trigger) 集成示例

#### GitHub Action

对于 GitHub，本仓库还在仓库根目录提供了一个开箱即用的 composite Action（[`action.yml`](./action.yml)）。你无需自己编写 `ocr review` 脚本，直接引用它即可完成完整流程——checkout、安装 OCR、执行审查、发布行内评论与汇总评论、上传 artifacts，以及重试与幂等处理：

```yaml
- uses: alibaba/open-code-review@main
  with:
    llm_url: ${{ secrets.OCR_LLM_URL }}
    llm_auth_token: ${{ secrets.OCR_LLM_AUTH_TOKEN }}
    llm_model: ${{ vars.OCR_LLM_MODEL }}
    llm_use_anthropic: ${{ vars.OCR_LLM_USE_ANTHROPIC }}
```

为保障可复现性，请固定到某个版本标签或 commit SHA。完整的 workflow 示例以及 inputs、outputs 与评论发布模式（置顶汇总、增量非破坏式发布）的完整列表，请参见 [`examples/github_actions/`](./examples/github_actions/) 目录。

## 文档

完整文档见 **[open-codereview.ai/docs](https://open-codereview.ai/docs)**：

- [快速开始](https://open-codereview.ai/docs/quickstart) —— 安装并运行你的第一次评审
- [安装](https://open-codereview.ai/docs/installation) —— 覆盖各平台与包管理器
- [CLI 参考](https://open-codereview.ai/docs/cli-reference) —— 所有命令与参数
- [评审规则](https://open-codereview.ai/docs/review-rules) —— 规则优先级链、文件格式与路径过滤
- [配置](https://open-codereview.ai/docs/configuration) —— 配置项与环境变量
- [MCP 服务器](https://open-codereview.ai/docs/mcp) —— 用外部工具扩展评审 agent
- [编程 Agent 集成](https://open-codereview.ai/docs/claude-code) —— Claude Code、Agent Skill 与委托模式
- [CI/CD 集成](https://open-codereview.ai/docs/cicd) —— 在流水线中运行评审
- [架构](https://open-codereview.ai/docs/architecture) · [工具](https://open-codereview.ai/docs/tools) · [会话查看器](https://open-codereview.ai/docs/viewer) · [遥测](https://open-codereview.ai/docs/telemetry) · [FAQ](https://open-codereview.ai/docs/faq)

## 命令

OCR 提供 `review`、`scan`、`delegate`、`config`、`llm`、`session`、`viewer` 等命令。完整的命令列表与所有参数（包括可恢复评审以及 `ocr scan` / `ocr delegate` 的全部选项），详见 **[CLI 参考](https://open-codereview.ai/docs/cli-reference)**。

## 示例

```bash
# 交互式供应商和模型设置
ocr config provider
ocr config model
ocr llm providers

# 删除自定义供应商
ocr config unset custom_providers.my-gateway

# 预览将被审查的文件（不调用 LLM）
ocr review --preview
ocr review -c abc123 -p

# 使用默认设置审查工作区变更
ocr review

# 以更高并发审查分支差异
ocr review --from main --to my-feature --concurrency 4

# 审查特定提交并以 JSON 格式输出详细信息
ocr review --commit abc123 --format json --audience agent

# 恢复中断的区间或单 commit 评审
ocr session list
ocr session show <session-id>
ocr review --from main --to my-feature --resume <session-id>
ocr review --commit abc123 --resume <session-id>

# 为本次审查选择或覆盖模型
ocr review --model claude-opus-4-6
ocr review --commit abc123 --model claude-sonnet-4-6

# 提供需求背景以获得更有针对性的审查
ocr review --background "为登录 API 添加限流"

# 从 Markdown 文件提供需求背景
ocr review --background-file ./docs/my_business_context.md

# 将内联背景与本地背景文件结合使用（两者都会生效）
ocr review --background "关注鉴权" --background-file ./docs/my_business_context.md

# 使用自定义审查规则
ocr review --rule /path/to/my-rules.json

# 预览某个文件路径生效的规则
ocr rules check src/main/java/com/example/Foo.java
ocr rules check --rule custom.json src/main/resources/mapper/UserMapper.xml

# 全量文件扫描：先预览文件列表（不调用 LLM）
ocr scan --preview

# 扫描整个仓库，限制消耗约 500k token
ocr scan --max-tokens-budget 500000

# 扫描子目录，跳过生成的/测试文件
ocr scan --path internal --exclude '**/*_test.go,**/generated/**'

# 扫描非 git 目录，使用 JSON 输出（包含 project_summary）
ocr scan --repo /path/to/plain/dir --format json

# 最快扫描：跳过规划、去重和项目总结
ocr scan --no-plan --no-dedup --no-summary

# 委托模式 — 让 AI agent 驱动评审（无需 LLM 配置）
ocr delegate preview
ocr delegate preview --from main --to feature-branch
ocr delegate preview --commit abc123
ocr delegate rule internal/handler.go internal/service.go cmd/main.go

# 在浏览器中查看审查会话历史
ocr viewer
ocr viewer --addr :3000
```

## 评审规则

OCR 通过四层优先级链解析评审规则（`--rule` 参数 > 项目配置 > 全局配置 > 内置默认），支持内联或文件形式的规则、`**` 通配匹配，以及 `include` / `exclude` 路径过滤。完整的规则文件格式与过滤语义，详见 **[评审规则](https://open-codereview.ai/docs/review-rules)**。

## 配置参考

配置位于 `~/.opencodereview/config.json`，可被环境变量覆盖，涵盖供应商、模型、MCP 服务器、语言与遥测。完整的配置项参考、环境变量与 MCP 服务器配置，详见 **[配置](https://open-codereview.ai/docs/configuration)** 与 **[MCP 服务器](https://open-codereview.ai/docs/mcp)**。

## 遥测

OpenTelemetry 集成，用于可观测性（spans、metrics）。默认关闭。

```bash
ocr config set telemetry.enabled true
ocr config set telemetry.exporter otlp
ocr config set telemetry.otlp_endpoint localhost:4317
```

设置 `telemetry.content_logging` 可在导出数据中包含 LLM 提示词和响应。

**协议选择：** 通过环境变量 `OTEL_EXPORTER_OTLP_PROTOCOL` 选择导出协议：

| 值 | 传输方式 | 说明 |
|---|---|---|
| `grpc`（默认） | gRPC | 默认端口 4317 |
| `http/protobuf` | HTTP | 默认端口 4318 |

**Endpoint 格式：** `telemetry.otlp_endpoint` 的值为 `host:port` 或 `http://host:port`，无需包含路径。SDK 会根据 [OTLP 规范](https://opentelemetry.io/docs/specs/otlp/#otlphttp-request)自动追加信号路径（如 `/v1/traces`）。

## 贡献

感谢所有为本项目做出贡献的人。参见 [CONTRIBUTING.zh-CN.md](CONTRIBUTING.zh-CN.md) 了解开发环境搭建、编码规范以及如何提交 Pull Request。

<a href="https://github.com/alibaba/open-code-review/graphs/contributors">
  <img src="https://contrib.rocks/image?repo=alibaba/open-code-review" />
</a>

## 许可证

[Apache-2.0](LICENSE) — Copyright 2026 Alibaba
