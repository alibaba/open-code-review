---
title: 贡献
sidebar:
  order: 13
---

OCR 是 Apache-2.0 许可下的开源项目。欢迎 bug 报告、文档修复与代码贡献。本页为
快速参考；权威版本位于
[`CONTRIBUTING.md`](https://github.com/alibaba/open-code-review/blob/main/CONTRIBUTING.md)。

## 贡献方式

不写 Go 也能帮上忙：

- **Bug 报告**——开一个带复现步骤的
  [GitHub issue](https://github.com/alibaba/open-code-review/issues/new/choose)。
- **功能请求**——在
  [Discussions](https://github.com/alibaba/open-code-review/discussions/categories/ideas)
  开帖或开 feature-request issue。
- **文档**——错别字、缺失示例、失效链接——这些 PR 通常最快被合并。
- **评审其他 PR**——非维护者的评论有助于减轻评审者负担。
- **代码**——bug 修复、性能优化、新功能。

## 本地开发设置

### 前置条件

- [Go ≥ 1.25](https://go.dev/dl/)
- [Git](https://git-scm.com/)
- [Make](https://www.gnu.org/software/make/)

### 获取源码

```bash
# Fork on GitHub, then:
git clone https://github.com/<your-username>/open-code-review.git
cd open-code-review
git remote add upstream https://github.com/alibaba/open-code-review.git

make build       # writes dist/opencodereview
make test        # LC_ALL=C go test -v -race -count=1 ./...
```

> `upstream` remote 是只读的。推送到 `origin`（你的 fork）并从那里发起 PR。

### 运行本地构建

```bash
./dist/opencodereview review --preview
```

为方便起见，在 `~/bin/ocr-dev` 放一个指向 `dist/opencodereview` 的符号链接，即可在
任意仓库调用 `ocr-dev`。

### Make target

| Target | 作用 |
|---|---|
| `make build` | 为当前平台构建 → `dist/opencodereview`。 |
| `make build-darwin-amd64` | 交叉编译 macOS Intel。 |
| `make build-darwin-arm64` | 交叉编译 macOS Apple Silicon。 |
| `make build-linux-amd64` | 交叉编译 Linux x86_64。 |
| `make build-linux-arm64` | 交叉编译 Linux ARM64。 |
| `make build-windows-amd64` | 交叉编译 Windows x86_64。 |
| `make build-windows-arm64` | 交叉编译 Windows ARM64。 |
| `make build-all` | 全部六个交叉编译二进制（linux/darwin/windows × amd64/arm64）。 |
| `make sha256sum` | 为构建产物生成 `sha256sum.txt`。 |
| `make dist` | `clean → build-all → sha256sum`。CI 运行的内容。 |
| `make test` | 带 race 检测运行测试。 |
| `make clean` | 删除 `dist/`。 |

## 分支与提交约定

### 分支前缀

| 前缀 | 用途 |
|---|---|
| `feat/` | 新功能 |
| `fix/` | Bug 修复 |
| `docs/` | 仅文档 |
| `refactor/` | 无行为变更的重构 |
| `test/` | 仅测试变更 |
| `chore/` | 构建 / CI / 工具 |

```bash
git checkout main
git pull upstream main
git checkout -b feat/anthropic-streaming
```

### 提交信息

[Conventional Commits](https://www.conventionalcommits.org/) 格式：

```
<type>(<scope>): <short summary>

[optional body explaining the why]
```

示例：

```
feat(agent): add support for custom tool definitions
fix(llm): handle timeout errors in Anthropic API calls
docs(readme): clarify endpoint resolution priority
refactor(viewer): extract task-card rendering into helper
```

**PR 标题**也用相同格式，以便在生成的 changelog 中整洁显示。

## 项目结构

```
open-code-review/
├── cmd/opencodereview/        # CLI 入口——参数解析、分发
├── internal/
│   ├── agent/                 # 评审 agent 逻辑、子 agent 分发
│   ├── config/                # 模板、规则、白名单、内嵌 JSON
│   ├── diff/                  # Git diff 解析、三种模式
│   ├── gitcmd/                # Git 子进程运行器
│   ├── llm/                   # LLM client（Anthropic 与 OpenAI）、端点解析器
│   ├── model/                 # 数据结构（LlmComment、Diff……）
│   ├── pathutil/              # 路径工具
│   ├── release/               # Release notes 生成
│   ├── session/               # JSONL 会话写入器
│   ├── stdout/                # 可静音的 stdout writer
│   ├── suggestdiff/           # 建议 diff 渲染
│   ├── telemetry/             # OpenTelemetry 配置 + 辅助
│   ├── tool/                  # 工具注册表 + provider 实现
│   └── viewer/                # 内嵌 HTTP UI
├── pages/                     # WebUI 营销页（独立 React app）
├── plugins/                   # Claude Code slash 命令
├── extensions/                # 编辑器扩展（VS Code）
├── examples/                  # CI 配方（GitHub Actions、GitLab CI）
├── skills/                    # Agent SDK skill manifest
├── scripts/                   # NPM postinstall + 跨平台构建脚本
├── npm/                       # 各平台 optional dependency 包
└── bin/                       # NPM wrapper（Node）
```

多数贡献触及 `internal/agent/`、`internal/tool/` 或 `internal/llm/`。
`cmd/opencodereview/` 中的 CLI 层有意保持精简——参数解析后分发到 agent 包。

## 代码质量检查

开 PR 前：

```bash
go fmt ./...
go vet ./...
make test       # race-enabled, runs in CI on every push
make build      # smoke test the binary builds
```

CI 在每次推送时运行同一套，不会有意外。

## 添加新工具

一个工具有两部分：

1. [`internal/config/toolsconfig/tools.json`](https://github.com/alibaba/open-code-review/blob/main/internal/config/toolsconfig/tools.json)
   中的 **JSON 定义**：name、description 与 LLM 看到的 JSON-schema 参数。
2. 在 `internal/tool/definitions.go` 注册的 **Go provider**，含实际实现。

两者都存在，新工具名才能工作。现有六个见[工具](../tools/)，可当作模板。

## 添加新规则模式

编辑 `internal/config/rules/system_rules.json` 把新 glob 映射到规则文档，并在
`internal/config/rules/rule_docs/` 下添加对应 markdown。规则文档按模式一个文件存放
（英文）。`language` 配置只在 system prompt 追加一条指示模型以该语言响应的指令；
它不会切换 rule-doc 文件。

## PR 流程

1. **大改动先开 issue。** 提前对齐方向，好过在代码评审时才发现分歧。
2. **每个 PR 一个逻辑变更。** 若有两个无关修复，提两个 PR。
3. **更新测试。** 行为变更需测试覆盖——`make test` 必须通过。
4. **更新文档。** 若变更影响参数、config key 或评审流水线，同时更新本文档站
  （在 [`docs/`](https://github.com/alibaba/open-code-review)）与任何相关内联帮助。
5. **填写 PR 模板。** 维护者会评审，通常几个工作日内。

## 贡献者许可协议（CLA）

本项目要求 Alibaba Open Source CLA。首次开 PR 时会有 bot 贴链接——电子签署
（一分钟）。后续 PR 无需重签。

## 首次贡献？

找标了
[`good first issue`](https://github.com/alibaba/open-code-review/labels/good%20first%20issue)
或 [`help wanted`](https://github.com/alibaba/open-code-review/labels/help%20wanted)
的 issue。多数体量小且自包含，issue 描述里有足够上下文，便于上手。

## 另见

- [架构](../architecture/)——修改 `internal/agent/` 前你需要的心智模型。
- [工具](../tools/)——现有工具长什么样。
- 完整贡献指南：
  [CONTRIBUTING.md](https://github.com/alibaba/open-code-review/blob/main/CONTRIBUTING.md)
