---
title: 集成
sidebar:
  order: 12
---

OCR 是一个 CLI，能与任何能派生进程的环境组合。本节涵盖将其接入 agentic
工作流和 CI 的主要方式，每种集成方式一页。

## 为什么是这些集成？

OCR 的 `--audience agent` 模式专为被另一个 agent 驱动而设计：stdout 只携带
JSON / 最终摘要，无进度 UI。这让三种组合方式顺理成章：

1. **Agent skill**——把 OCR 注册为调用方 agent 可调用的 skill（如 Anthropic
   Agent SDK）。
2. **Command（Claude Code plugin）**——安装打包的命令，使
   `/open-code-review:review` 端到端运行 `ocr review`。在任何其他支持
   Claude-Code 风格命令约定的 agent 中也可用。
3. **Direct subprocess**——任何能调 `subprocess.run` 的框架（LangChain 工具、
   自定义 shell、CI 步骤）直接通过 shell 调用。

你可以混搭。skill 和 plugin 最终调用的都是同一个二进制。

## 选择模式

| 方式 | 最适合 | 页面 |
|---|---|---|
| Agent skill | 你基于 Anthropic Agent SDK 或其他消费 `SKILL.md` 的框架构建。 | [Agent Skill](agent-skill/) |
| Command（Claude Code plugin） | 你用 Claude Code（或任何有 Claude-Code 风格命令约定的 agent），希望 `/open-code-review:review` 做正确的事。 | [Command（Claude Code Plugin）](claude-code/) |
| Direct subprocess | 你需要从自定义脚本、LangChain 工具或非 Anthropic agent 调用 OCR。 | [Direct Subprocess](subprocess/) |
| CI/CD | 你希望 OCR 在每个 PR 或 pre-commit 时运行。 | [CI/CD](ci/) |

## MCP 怎么办？

OCR 目前不暴露 Model Context Protocol server。预期的集成方式是“agent 调用 CLI”，
更简单，且能避免 MCP server 引入的长期运行进程问题。如果你的 agent 平台特别
要求 MCP，用一个薄 shim 包裹 CLI——一个 30 行的 Node 脚本暴露单个 `review`
工具就够了。

反过来的方向**是**支持的：OCR 可以作为 MCP **客户端**，把外部 MCP server 的工具
拉进一次审查。见 [MCP 服务器](../mcp/)。

## 适用于所有模式的提示

- **始终传 `--audience agent`**，当调用方不是人时。否则进度行会污染待解析的输出。
- **有 PR / 需求上下文时始终传 `--background`**。质量提升显著，成本只是一个工具
  参数。
- **CI 中把 `--concurrency` 调低**（`--concurrency 4`）以免触发厂商速率限制。默认 8。
- **CI 中优先 `--from origin/main --to HEAD`** 而非 `--commit HEAD`——merge-base
  计算排除分支切出后落到 `main` 上的无关变更。
- **让 `OCR_LLM_TOKEN` 远离 stdout/logs。** OCR 不打印它，但配置不当的 shell
  可能泄露。使用 CI secret 掩码。

## 另见

- [CLI 参考](../cli-reference/)——review 命令的每个参数。
- [配置](../configuration/)——环境变量与 config key。
- [快速开始](../quickstart/)——首次评审的最小化设置。
