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

AI 驱动的代码审查 CLI 工具，读取 Git diff，通过具备工具调用能力的 Agent 将变更文件发送至可配置的 LLM，生成具有行级精度的结构化审查意见。

Agent 可以读取完整文件内容、搜索代码库、检查其他变更文件以获取上下文，从而进行深度审查 —— 而非仅停留在表面的 diff 反馈。

![Open Benchmark](imgs/open-benchmark.png)

## 安装

### 通过 NPM 安装（推荐）

```bash
npm install -g @alibaba-group/open-code-review
```

安装后，`ocr` 命令即可全局使用。

### 从 GitHub Release 下载

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

### 从源码构建

```bash
git clone https://github.com/alibaba/open-code-review.git
cd open-code-review
make build
sudo cp dist/opencodereview /usr/local/bin/ocr
```

## 快速开始

### 1. 配置 LLM

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

工具也会回退使用 Claude Code 环境变量（`ANTHROPIC_BASE_URL`、`ANTHROPIC_AUTH_TOKEN`、`ANTHROPIC_MODEL`），并解析 `~/.zshrc` / `~/.bashrc` 中的相关导出。

### 2. 测试连通性

```bash
ocr llm test
```

### 3. 开始审查

```bash
cd your-project

# 工作区模式 —— 审查所有暂存、未暂存和未跟踪的变更
ocr review

# 分支范围 —— 比较两个引用
ocr review --from main --to feature-branch

# 单个提交
ocr review --commit abc123

# 多个提交合并审查（例如你的提交之间夹杂了其他人的提交）
ocr review --commit abc123,def456
```

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
| `--commit` | `-c` | — | 审查提交，支持单个或逗号分隔的多个提交 |
| `--preview` | `-p` | `false` | 预览将被审查的文件列表，不调用 LLM |
| `--format` | `-f` | `text` | 输出格式：`text` 或 `json` |
| `--concurrency` | — | `8` | 最大并发文件审查数 |
| `--timeout` | — | `10` | 并发任务超时时间（分钟） |
| `--audience` | — | `human` | `human`（显示进度）或 `agent`（仅输出摘要） |
| `--rule` | — | — | 自定义 JSON 审查规则路径 |
| `--tools` | — | — | 自定义 JSON 工具配置路径 |

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

# 审查多个提交合并（例如一个 bug 修复跨了多个提交）
ocr review --commit abc123,def456 --format json --audience agent

# 使用自定义审查规则
ocr review --rule /path/to/my-rules.json

# 预览某个文件路径生效的规则
ocr rules check src/main/java/com/example/Foo.java
ocr rules check --rule custom.json src/main/resources/mapper/UserMapper.xml

# 在浏览器中查看审查会话历史
ocr viewer
ocr viewer --addr :3000
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

## 架构

审查 Agent 遵循**三阶段工作流**：

1. **计划阶段** —— 对于超过 50 行的变更，Agent 会在审查前进行风险分析。较小的 diff 直接跳至主阶段。
2. **主任务循环** —— 每个变更文件分配独立的 goroutine。LLM 在对话循环中与内置工具交互（读取文件、搜索代码、读取 diff、提交评论），直到调用 `task_done`。
3. **记忆压缩** —— 当提示上下文超过 token 阈值（异步 60%，同步 80%）时，Agent 使用三区分区（冻结 / 压缩 / 活跃）管理上下文窗口大小。

### 关键设计决策

- **按文件并发处理** —— 文件并行审查（默认 8 个 worker）。超时机制防止单个文件阻塞其他文件。
- **双协议支持** —— 同时支持 Anthropic Messages API 和 OpenAI Chat Completions API，自动 URL 规范化。
- **工具调用 Agent** —— LLM 可以访问领域特定工具（`code_search`、`file_read`、`code_comment`、`file_find`、`file_read_diff`），实现跨引用的上下文感知审查，而非孤立的 diff 扫描。

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

### 模板参数

内部默认值定义于 `internal/config/template/task_template.json`：

| 参数 | 默认值 | 描述 |
|------|--------|------|
| `MAX_TOKENS` | 58888 | 每次 LLM 请求最大 token 数 |
| `MAX_TOOL_REQUEST_TIMES` | 20 | 每个文件最大工具调用迭代次数 |
| `PLAN_MODE_LINE_THRESHOLD` | 50 | 低于此行数跳过计划阶段 |
| `TOOL_REQUEST_WAIT_TIME_MS` | 10000 | 单次工具请求超时时间 |

## 内置工具

审查过程中 LLM Agent 可调用的工具：

| 工具 | 可用阶段 | 用途 |
|------|----------|------|
| `task_done` | main_task | 终止审查（DONE/FAILED） |
| `code_comment` | main_task | 提交行级审查意见 |
| `file_read` | main_task | 按行范围读取文件内容 |
| `code_search` | plan + main | 跨文件搜索文本/正则表达式 |
| `file_read_diff` | plan + main | 查看其他变更文件的 diff 内容 |
| `file_find` | plan + main | 按文件名关键词查找文件 |

## 系统审查规则

按文件类型通过 glob 模式匹配的内置审查清单，定义于 `internal/config/rules/system_rules.json`：

| 模式 | 关注领域 |
|------|----------|
| `*.java` | NPE 风险、死循环、switch 穿透、N+1 查询、线程安全 |
| `*.{ts,js,tsx,jsx}` | 代码质量、React 最佳实践、异步规范、XSS/安全 |
| `*.kt` | 空安全、协程使用、惯用模式 |
| `*{go,py,ets,lua,dart,swift,groovy}` | 逻辑缺陷、拼写错误 |
| `*{cpp,cc,hpp}` | 智能指针、RAII、STL、const 正确性 |
| `*.c` | malloc/free 配对、缓冲区溢出 |
| `pom.xml` / `build.gradle` | 禁止 SNAPSHOT 版本 |
| `package.json` | latest/通配符版本、依赖冲突 |
| `*mapper*.xml` / `*dao*.xml` | SQL 注入、性能、逻辑错误 |
| `*.properties` | 拼写检测、重复键、安全问题 |

可通过 `--rule path/to/rules.json` 覆盖。

## 遥测

OpenTelemetry 集成，用于可观测性（spans、metrics）。默认关闭。

```bash
ocr config set telemetry.enabled true
ocr config set telemetry.exporter otlp
ocr config set telemetry.otlp_endpoint localhost:4317
```

设置 `telemetry.content_logging` 可在导出数据中包含 LLM 提示词和响应。

## 开发

```bash
make build      # 为当前平台构建
make test       # 带竞态检测运行测试
make clean      # 清除 dist/
make build-all  # 交叉编译（linux/amd64, linux/arm64, darwin/amd64, darwin/arm64）
make dist       # 完整发布流水线
```

## 集成到编程 Agent

OCR 可以无缝集成到 AI 编程 Agent 中，作为斜杠命令使用，在 Agent 工作流中直接进行代码审查。

### 方式一：作为 Skill 安装

使用 `npx` 将 OCR skill 安装到项目中：

```bash
npx skills add alibaba/open-code-review --skill open-code-review
```

此命令从 [skills 注册表](skills/open-code-review/SKILL.md)安装 `open-code-review` skill，教会你的编程 Agent 如何调用 `ocr` 进行代码审查、按优先级分类问题，并可选择性地应用修复。

### 方式二：作为 Claude Code Plugin 安装

对于 [Claude Code](https://docs.anthropic.com/en/docs/claude-code)，在 Claude Code 中通过以下命令安装命令插件：

```bash
/plugin marketplace add alibaba/open-code-review
/plugin install open-code-review@open-code-review
```

此命令注册 `/open-code-review:review` 斜杠命令，运行 OCR 并自动过滤和修复问题。

### 方式三：直接复制命令文件

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

## 许可证

[Apache-2.0](LICENSE) — Copyright 2026 Alibaba
