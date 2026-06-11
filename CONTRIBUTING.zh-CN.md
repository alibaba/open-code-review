# 贡献指南

感谢你对 OpenCodeReview 的关注！无论是修复拼写错误、报告 Bug，还是实现新功能，每一份贡献都很有价值。

[English version](CONTRIBUTING.md) | [日本語版](CONTRIBUTING.ja-JP.md) | [한국어](CONTRIBUTING.ko-KR.md) | [Русский](CONTRIBUTING.ru-RU.md)

## 行为准则

参与本项目即表示你同意维护一个尊重和包容的环境。请在所有交流中保持友善和建设性。

## 贡献方式

除了写代码，还有很多方式可以参与贡献：

- **报告 Bug** — 发现问题？请提交 issue 并附上复现步骤。
- **建议功能** — 有改进想法？可以在 [GitHub Discussions](https://github.com/alibaba/open-code-review/discussions/categories/ideas) 中发起讨论，或直接提交一个 [Feature Request](https://github.com/alibaba/open-code-review/issues/new?template=feature_request.yml) issue。
- **改进文档** — 修复错别字、完善说明或补充示例。也可以提交一个 [Documentation Issue](https://github.com/alibaba/open-code-review/issues/new?template=docs_report.yml) 来报告文档问题。
- **审查 PR** — 帮助我们审查其他贡献者的代码。
- **编写代码** — 修复 Bug、添加功能或提升性能。

## 快速开始

### 前置条件

- [Go 1.25+](https://go.dev/dl/)
- [Git](https://git-scm.com/)
- [Make](https://www.gnu.org/software/make/)

### 环境搭建

```bash
# 1. 在 GitHub 上 Fork 本仓库

# 2. 克隆你的 Fork
git clone https://github.com/<你的用户名>/open-code-review.git
cd open-code-review

# 3. 添加上游远端（用于同步主仓库的最新变更）
git remote add upstream https://github.com/alibaba/open-code-review.git

# 4. 构建项目
make build

# 5. 运行测试
make test
```

如果一切通过，就可以开始贡献了。

> **注意：** `upstream` 远端对贡献者是只读的，仅用于拉取主仓库的最新更新。你不能直接向 upstream 推送代码，所有贡献必须推送到你的 fork（`origin`），然后通过 Pull Request 提交。

## 开发工作流

### 分支管理

从 `main` 创建功能分支：

```bash
git checkout main
git pull upstream main
git checkout -b feat/your-feature-name
```

使用前缀标明变更类型：

| 前缀        | 用途                   |
| ----------- | ---------------------- |
| `feat/`     | 新功能                 |
| `fix/`      | Bug 修复               |
| `docs/`     | 仅文档变更             |
| `refactor/` | 代码重构（无行为变化） |
| `test/`     | 添加或更新测试         |
| `chore/`    | 构建、CI 或工具链变更  |

### 提交信息

遵循 [Conventional Commits](https://www.conventionalcommits.org/) 规范：

```
<type>(<scope>): <简短描述>

[可选的详细说明]
```

示例：

```
feat(agent): add support for custom tool definitions
fix(llm): handle timeout errors in Anthropic API calls
docs(README): update configuration examples
```

### 代码质量

提交前请确保通过以下检查：

```bash
# 格式化和静态检查
go fmt ./...
go vet ./...

# 带竞态检测运行测试
make test

# 构建成功
make build
```

### 项目结构

```
├── cmd/opencodereview/   # CLI 入口
├── internal/
│   ├── agent/            # 评审 Agent 逻辑
│   ├── config/           # 配置管理
│   ├── diff/             # Git diff 解析
│   ├── llm/              # LLM API 客户端（Anthropic & OpenAI）
│   ├── model/            # 数据模型
│   ├── session/          # 评审会话管理
│   ├── tool/             # 内置工具（file_read, code_search 等）
│   ├── telemetry/        # OpenTelemetry 集成
│   └── viewer/           # WebUI 会话查看器
├── pages/                # WebUI 前端
├── scripts/              # 构建和安装脚本
└── bin/                  # NPM 包装器
```

## 文档贡献

文档是 OpenCodeReview 的重要组成部分。我们欢迎对 README、代码注释、配置示例以及任何面向用户的文本进行改进。

### 哪些算文档贡献

- 修复错别字、语法错误或失效链接
- 完善表述不清的说明或补充缺失的上下文
- 为命令或配置项添加使用示例
- 更新过时的内容（例如功能变更后的文档同步）
- 翻译或改进中英文文档（`README.zh-CN.md`、`CONTRIBUTING.zh-CN.md`）

### 文档贡献流程

1. 如果你发现问题但暂时不打算自己修复，请提交一个 [Documentation Issue](https://github.com/alibaba/open-code-review/issues/new?template=docs_report.yml)。
2. 如果你想直接修复，fork 仓库后修改，提交 PR 时使用 `docs/` 分支前缀（例如 `docs/fix-config-example`）。
3. 纯文档 PR 不需要修改测试，但请确保文中涉及的命令和代码片段准确无误。

### 文档文件一览

| 文件                    | 用途                 |
| ----------------------- | -------------------- |
| `README.md`             | 项目主文档（英文）   |
| `README.zh-CN.md`       | 中文翻译             |
| `CONTRIBUTING.md`       | 贡献指南（英文）     |
| `CONTRIBUTING.zh-CN.md` | 贡献指南（中文）     |

## 提交变更

### 提交 Issue

在进行重大修改之前，请先开一个 issue 讨论方案。这可以避免重复劳动，并确保你的贡献与项目方向一致。

报告 Bug 时请包含：

1. OpenCodeReview 版本（`ocr version`）
2. 操作系统和架构
3. 复现步骤
4. 预期行为与实际行为
5. 相关日志或错误信息

### Pull Request 流程

1. **保持 PR 聚焦** — 每个 PR 只包含一个逻辑变更。多个独立改动请分别提交 PR。
2. **编写测试** — 为行为变更添加或更新测试。
3. **更新文档** — 如果变更影响用户侧行为，请同步更新相关文档。
4. **签署 CLA** — 所有贡献者需在 PR 合并前签署贡献者许可协议（详见下方）。
5. **填写 PR 模板** — 描述你的改动是什么以及为什么这样做。

### PR 标题格式

使用与 commit message 相同的 Conventional Commits 格式：

```
feat(agent): add support for custom tool definitions
```

### 审查流程

- 维护者通常会在几个工作日内审查你的 PR。
- 我们可能会要求修改——这很正常，是协作而非对立。
- 一旦批准，维护者会合并你的 PR。

## 贡献者许可协议（CLA）

我们要求所有贡献者在合并代码前签署阿里巴巴开源贡献者许可协议（CLA）。这确保项目可以在其许可条款下合法分发。

当你首次提交 PR 时，CLA bot 会自动发布评论并附上签署说明。只需点击链接进行电子签署即可，整个过程只需一分钟。

## 新手推荐

第一次参与？可以关注以下标签的 issue：

- [`good first issue`](https://github.com/alibaba/open-code-review/labels/good%20first%20issue) — 小型、范围明确的任务，适合快速上手。
- [`help wanted`](https://github.com/alibaba/open-code-review/labels/help%20wanted) — 我们希望得到社区帮助的问题。

适合入手的方向：

- 改善错误信息和 CLI 输出
- 为未覆盖的代码路径编写测试
- 文档改进

## 社区

- **Bug 报告** — [GitHub Issues](https://github.com/alibaba/open-code-review/issues)
- **功能建议** — [GitHub Discussions (Ideas)](https://github.com/alibaba/open-code-review/discussions/categories/ideas) 或 [Feature Request Issue](https://github.com/alibaba/open-code-review/issues/new?template=feature_request.yml)
- **使用疑问与帮助** — 对 OpenCodeReview 的使用有任何疑问，欢迎在 [GitHub Discussions](https://github.com/alibaba/open-code-review/discussions) 中提问交流

## 许可证

向 OpenCodeReview 贡献代码即表示你同意你的贡献将以 [Apache License 2.0](LICENSE) 进行许可。
