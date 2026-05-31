<p align="center">
  <a href="https://alibaba.github.io/open-code-review/">
    <img src="imgs/logo.svg" alt="OpenCodeReview logo" width="240" height="240">
  </a>
</p>
<p align="center">The open source AI code review agent.</p>
<p align="center">
  <a href="https://www.npmjs.com/package/@alibaba-group/open-code-review"><img alt="npm" src="https://img.shields.io/npm/v/@alibaba-group/open-code-review?style=flat-square" /></a>
  <a href="https://github.com/alibaba/open-code-review/actions/workflows/release.yml"><img alt="Build status" src="https://img.shields.io/github/actions/workflow/status/alibaba/open-code-review/release.yml?style=flat-square" /></a>
  <a href="https://github.com/alibaba/open-code-review/blob/main/LICENSE"><img alt="License" src="https://img.shields.io/github/license/alibaba/open-code-review?style=flat-square" /></a>
</p>
<p align="center">
  <a href="README.md">English</a> | 简体中文
</p>

---

## Open Code Review 是什么？

Open Code Review 是一款 AI 驱动的代码审查 CLI 工具。它的前身是阿里集团内部官方 AI 代码审查助手，过去两年在内部服务了数万开发者，识别了数百万个代码缺陷。经过大规模充分验证后，我们将其孵化为开源项目，对社区开放。只需配置一个模型端点即可使用。

它读取 Git diff，通过具备工具调用能力的 Agent 将变更文件发送至可配置的 LLM，生成具有行级精度的结构化审查意见。Agent 可以读取完整文件内容、搜索代码库、检查其他变更文件以获取上下文，从而进行深度审查——而非仅停留在表面的 diff 反馈。

![Highlights](imgs/highlights-zh.png)

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

### CLI

#### 安装

**通过 NPM 安装（推荐）**

```bash
npm install -g @alibaba-group/open-code-review
```

安装后，`ocr` 命令即可全局使用。

**从 GitHub Release 下载**

从 [GitHub Releases](https://github.com/alibaba/open-code-review/releases) 下载最新二进制文件：

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
```

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

```bash
# 方式 A：交互式配置
ocr config set llm.url https://api.anthropic.com/v1/messages
ocr config set llm.auth_token your-api-key-here
ocr config set llm.model claude-opus-4-6
ocr config set llm.use_anthropic true

# 方式 B：环境变量（优先级最高）
export OCR_LLM_URL=https://api.anthropic.com/v1/messages
export OCR_LLM_TOKEN=your-api-key-here
export OCR_LLM_MODEL=claude-opus-4-6
export OCR_USE_ANTHROPIC=true
```

配置存储于 `~/.opencodereview/config.json`。

同时兼容了 Claude Code 环境变量（`ANTHROPIC_BASE_URL`、`ANTHROPIC_AUTH_TOKEN`、`ANTHROPIC_MODEL`），并解析 `~/.zshrc` / `~/.bashrc` 中的相关导出。

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
```

### 集成到编程 Agent

OCR 可以无缝集成到 AI 编程 Agent 中，作为斜杠命令使用，在 Agent 工作流中直接进行代码审查。

#### 方式一：作为 Skill 安装

使用 `npx` 将 OCR skill 安装到项目中：

```bash
npx skills add alibaba/open-code-review --skill open-code-review
```

此命令从 [skills 注册表](skills/open-code-review/SKILL.md)安装 `open-code-review` skill，教会你的编程 Agent 如何调用 `ocr` 进行代码审查、按优先级分类问题，并可选择性地应用修复。

#### 方式二：作为 Claude Code Plugin 安装

对于 [Claude Code](https://docs.anthropic.com/en/docs/claude-code)，在 Claude Code 中通过以下命令安装命令插件：

```bash
/plugin marketplace add alibaba/open-code-review
/plugin install open-code-review@open-code-review
```

此命令注册 `/open-code-review:review` 斜杠命令，运行 OCR 并自动过滤和修复问题。

#### 方式三：直接复制命令文件

如果不想使用任何包管理器，可以直接复制命令文件，在 Claude Code 中使用 `/open-code-review` 斜杠命令。

**项目级**（通过 git 与团队共享）：

```bash
mkdir -p .claude/commands
curl -o .claude/commands/open-code-review.md \
  https://raw.githubusercontent.com/alibaba/open-code-review/main/plugins/open-code-review/commands/review.md
```

**用户级**（个人全局使用，适用于所有项目）：

```bash
mkdir -p ~/.claude/commands
curl -o ~/.claude/commands/open-code-review.md \
  https://raw.githubusercontent.com/alibaba/open-code-review/main/plugins/open-code-review/commands/review.md
```

> **前置条件**：所有集成方式都需要安装 `ocr` CLI 并配置 LLM。参见上方[安装](#安装)和[配置 LLM](#1-配置-llm)。

## 命令

| 命令 | 别名 | 描述 |
|------|------|------|
| `ocr review` | `ocr r` | 开始代码审查 |
| `ocr rules check <file>` | — | 预览某个文件路径生效的审查规则 |
| `ocr config set <key> <value>` | — | 设置配置项 |
| `ocr llm test` | — | 测试 LLM 连通性 |
| `ocr viewer` | `ocr v` | 启动 WebUI 会话查看器，地址 `localhost:5483` |
| `ocr version` | — | 显示版本信息 |

### `ocr review` 参数

| 参数 | 缩写 | 默认值 | 描述 |
|------|------|--------|------|
| `--repo` | — | 当前目录 | Git 仓库根目录 |
| `--from` | — | — | 源引用（如 `main`） |
| `--to` | — | — | 目标引用（如 `feature-branch`） |
| `--commit` | `-c` | — | 审查单个提交 |
| `--preview` | `-p` | `false` | 预览将被审查的文件列表，不调用 LLM |
| `--format` | `-f` | `text` | 输出格式：`text` 或 `json` |
| `--concurrency` | — | `8` | 最大并发文件审查数 |
| `--timeout` | — | `10` | 并发任务超时时间（分钟） |
| `--audience` | — | `human` | `human`（显示进度）或 `agent`（仅输出摘要） |
| `--rule` | — | — | 自定义 JSON 审查规则路径 |
| `--tools` | — | — | 自定义 JSON 工具配置路径 |
| `--platform` | — | — | 发布平台：`gitlab` |
| `--mr` | — | — | Merge Request IID（GitLab） |
| `--project-id` | — | — | GitLab 项目 ID 或命名空间路径（如 group/sub/project） |
| `--gitlab-base-url` | — | — | GitLab 基础 URL（默认：`https://gitlab.com`） |
| `--publish` | — | `false` | 将审查结果发布到 Merge Request |
| `--pr-summary` | — | `false` | 追加/更新 MR 描述中的摘要块 |
| `--clear-existing` | — | `false` | 发布前删除 OCR 评论 |
| `--clear-inline` | — | `false` | 删除 OCR 行内评论后退出 |
| `--clear-summary` | — | `false` | 删除 OCR 摘要/请求修改评论后退出 |
| `--no-inline` | — | `false` | 发布时跳过行内评论 |
| `--no-summary-comment` | — | `false` | 发布时跳过摘要评论 |

## 示例

```bash
# 预览将被审查的文件（不调用 LLM）
ocr review --preview
ocr review -c abc123 -p

# 使用默认设置审查工作区变更
ocr review

# 以更高并发审查分支差异
ocr review --from main --to my-feature --concurrency 4

# 审查特定提交并以 JSON 格式输出详细信息
ocr review --commit abc123 --format json --audience agent

# 使用自定义审查规则
ocr review --rule /path/to/my-rules.json

# 预览某个文件路径生效的规则
ocr rules check src/main/java/com/example/Foo.java
ocr rules check --rule custom.json src/main/resources/mapper/UserMapper.xml

# 在浏览器中查看审查会话历史
ocr viewer
ocr viewer --addr :3000
```

## GitLab MR 发布

OCR 可以将审查结果直接发布到 GitLab Merge Request。

### 用法

```bash
# 将审查结果发布到 GitLab MR
GITLAB_TOKEN=... ocr review --platform gitlab --project-id 456 --mr 123 --publish

# 发布并更新 MR 描述摘要
GITLAB_TOKEN=... ocr review --platform gitlab --project-id 456 --mr 123 --publish --pr-summary

# 删除 MR 上的 OCR 行内评论
GITLAB_TOKEN=... ocr review --platform gitlab --project-id 456 --mr 123 --clear-inline

# 删除 MR 上的 OCR 摘要/请求修改评论
GITLAB_TOKEN=... ocr review --platform gitlab --project-id 456 --mr 123 --clear-summary

# CI 环境变量推断
export GITLAB_TOKEN=...
export CI_PROJECT_ID=456
export CI_MERGE_REQUEST_IID=123
export CI_SERVER_URL=https://gitlab.example.com
ocr review --platform gitlab --publish
```

### GitLab 参数

| 参数 | 描述 |
|------|------|
| `--platform gitlab` | 启用 GitLab 平台模式 |
| `--mr <iid>` | Merge Request IID |
| `--project-id <id-or-path>` | GitLab 项目 ID 或命名空间路径（如 group/sub/project） |
| `--gitlab-base-url <url>` | 自托管 GitLab 基础 URL（默认：`https://gitlab.com`） |
| `--publish` | 将审查结果发布到 MR |
| `--pr-summary` | 追加/更新 MR 描述中的摘要块 |
| `--clear-existing` | 发布前删除 OCR 评论（需要 `--publish`） |
| `--clear-inline` | 删除 OCR 行内评论后退出 |
| `--clear-summary` | 删除 OCR 摘要/请求修改评论后退出 |
| `--no-inline` | 发布时跳过行内评论 |
| `--no-summary-comment` | 发布时跳过摘要评论 |

### 环境变量

| 变量 | 用途 |
|------|------|
| `GITLAB_TOKEN` | GitLab API 令牌（优先级最高） |
| `OCR_GITLAB_TOKEN` | GitLab API 令牌（备选） |
| `OCR_GITLAB_BASE_URL` | 自托管 GitLab 基础 URL |
| `CI_SERVER_URL` | CI 推断的 GitLab 基础 URL |
| `CI_PROJECT_ID` | CI 推断的项目 ID |
| `CI_MERGE_REQUEST_IID` | CI 推断的 MR IID |

### Marker 归属

OCR 只删除或更新包含 OCR marker 的评论。未标记的用户评论不会被触碰：

- 行内评论：`<!-- open-code-review:inline -->`
- 摘要评论：`<!-- open-code-review:summary -->`
- 请求修改评论：`<!-- open-code-review:request-changes -->`
- MR 描述摘要：由 `<!-- open-code-review:pr-summary:start -->` 和 `<!-- open-code-review:pr-summary:end -->` 包围

### GitLab 13.12 兼容性

GitLab 客户端支持 GitLab 13.12+ 自托管实例：

- `/merge_requests/:iid/diffs` 404 时回退到 `/merge_requests/:iid/changes?access_raw_diffs=true`
- 通过 `--gitlab-base-url`、`OCR_GITLAB_BASE_URL` 或 `CI_SERVER_URL` 支持自托管基础 URL

### E2E 测试

可选的 E2E 测试针对本地 GitLab 容器运行：

```bash
OCR_E2E_GITLAB=1 \
OCR_E2E_GITLAB_URL=http://localhost:8929 \
OCR_E2E_GITLAB_TOKEN=<token> \
OCR_E2E_GITLAB_PROJECT_ID=<project-id> \
OCR_E2E_GITLAB_MR_IID=<iid> \
  go test -tags=e2e ./internal/e2e/gitlab -count=1 -v
```

## 评审规则

OCR 通过四层优先级链解析评审规则。每层采用首次匹配原则：如果文件路径匹配到某个模式，则使用该规则；否则穿透到下一层。

| 优先级 | 来源 | 路径 | 描述 |
|--------|------|------|------|
| 1（最高） | `--rule` 参数 | 用户指定路径 | CLI 显式覆盖 |
| 2 | 项目配置 | `<repoDir>/.opencodereview/rule.json` | 项目级规则，可提交到 git |
| 3 | 全局配置 | `~/.opencodereview/rule.json` | 用户级个人偏好 |
| 4（最低） | 系统默认 | 内嵌 `system_rules.json` | 覆盖常见语言和文件类型的内置规则 |

### 规则文件格式

第 1–3 层使用相同的 JSON 格式：

```json
{
  "rules": [
    {
      "path": "force-api/**/*.java",
      "rule": "所有新方法必须对必填参数进行空值校验"
    },
    {
      "path": "**/*mapper*.xml",
      "rule": "检查 SQL 注入风险、参数错误和缺少闭合标签"
    }
  ]
}
```

- `path` 支持 `**` 递归匹配和 `{java,kt}` 大括号展开。
- 在每一层内，规则按声明顺序评估 —— 首次匹配生效。
- 如果规则文件不存在，将被静默跳过。

## 配置参考

配置文件：`~/.opencodereview/config.json`

| 键 | 类型 | 示例 |
|----|------|------|
| `llm.url` | string | `https://api.openai.com/v1/chat/completions` |
| `llm.auth_token` | string | `sk-xxxxxxx` |
| `llm.model` | string | `claude-opus-4-6` |
| `llm.use_anthropic` | boolean | `true` \| `false` |
| `language` | string | `English` \| `Chinese`（默认：Chinese） |
| `telemetry.enabled` | boolean | `true` \| `false` |
| `telemetry.exporter` | string | `console` \| `otlp` |
| `telemetry.otlp_endpoint` | string | OTLP 采集器地址 |
| `telemetry.content_logging` | boolean | 在遥测数据中包含提示词 |

环境变量优先级高于配置文件。

### 环境变量

| 变量 | 用途 |
|------|------|
| `OCR_LLM_URL` | LLM API 端点 URL |
| `OCR_LLM_TOKEN` | API 密钥 / 认证令牌 |
| `OCR_LLM_MODEL` | 模型名称 |
| `OCR_USE_ANTHROPIC` | `true` = Anthropic，`false` = OpenAI |


## 遥测

OpenTelemetry 集成，用于可观测性（spans、metrics）。默认关闭。

```bash
ocr config set telemetry.enabled true
ocr config set telemetry.exporter otlp
ocr config set telemetry.otlp_endpoint localhost:4317
```

设置 `telemetry.content_logging` 可在导出数据中包含 LLM 提示词和响应。

## 贡献

参见 [CONTRIBUTING.zh-CN.md](CONTRIBUTING.zh-CN.md) 了解开发环境搭建、编码规范以及如何提交 Pull Request。

## 许可证

[Apache-2.0](LICENSE) — Copyright 2026 Alibaba
