---
title: Command（Claude Code Plugin）
sidebar:
  order: 2
---

安装打包的命令，使 OCR 在 [Claude Code](https://docs.anthropic.com/en/docs/claude-code)
内端到端运行——评审 diff、分类发现，并自动应用值得采纳的修复。

## 仓库里有什么

仓库在
[`plugins/open-code-review/`](https://github.com/alibaba/open-code-review/tree/main/plugins/open-code-review)
下提供 Claude Code plugin。命令 prompt 本体位于
[`plugins/open-code-review/commands/review.md`](https://github.com/alibaba/open-code-review/blob/main/plugins/open-code-review/commands/review.md)，
是下述工作流的权威依据。

## 安装

### 方式 1：plugin marketplace（推荐）

在 **Claude Code 内**运行这两条命令：

```bash
/plugin marketplace add alibaba/open-code-review
/plugin install open-code-review@open-code-review
```

这会注册 `/open-code-review:review` slash 命令，并保持可通过 `/plugin` 更新。

### 方式 2：直接复制命令文件

若想跳过 plugin marketplace，把命令文件直接放进 `.claude/commands/`。这会注册为
`/open-code-review`（无 `:review` 后缀）。

**项目级**（随仓库提交，团队共享）：

```bash
mkdir -p .claude/commands
curl -o .claude/commands/open-code-review.md \
  https://raw.githubusercontent.com/alibaba/open-code-review/main/plugins/open-code-review/commands/review.md
```

**用户级**（机器上每个项目可用）：

```bash
mkdir -p ~/.claude/commands
curl -o ~/.claude/commands/open-code-review.md \
  https://raw.githubusercontent.com/alibaba/open-code-review/main/plugins/open-code-review/commands/review.md
```

### 其他支持命令的 agent

命令文件是带单个 frontmatter 字段的纯 markdown——没有任何 Claude Code 专有
内容。如果你的 agent 支持类似的 **command** 约定（从目录加载为可调用命令的
markdown prompt），上面的文件复制方法就是安装路径：把 `open-code-review.md`
放进你的 agent 读取命令的目录，按你的 agent 调用命令的方式调用它。prompt 正文
与 agent 无关——它只告诉模型选哪些 `ocr` 参数以及如何分级输出。

> **前置条件：** 首次运行时命令会自行安装 `ocr` CLI
> （通过 `npm install -g @alibaba-group/open-code-review`），前提是二进制不在
> `PATH` 上。你**确实**需要预先配置好 LLM——若 `ocr llm test` 连不上，命令会
> 失败。见[配置](../../configuration/)。

## 使用

在 Claude Code 中按名调用命令。通过 plugin marketplace 安装的用
`/open-code-review:review`，直接复制文件的用 `/open-code-review`：

```
/open-code-review:review
/open-code-review:review review this PR against main
/open-code-review:review focus on race conditions in commit abc123
```

prompt 解析你的请求并选择正确的 `ocr review` 参数：无参数 → 工作区模式
（staged + unstaged + untracked），提到 commit → `--commit`，提到分支区间 →
`--from` / `--to`。你也可以直接透传 OCR 参数
（如 `/open-code-review:review --commit abc123` 或 `--from main --to feature`）。

## 命令做什么

命令 prompt 很短——三步：

1. **运行评审。** 用从你请求推断的参数调用 `ocr review --audience agent`
   （描述了需求上下文时加可选 `--background`）。若 `ocr` 二进制不在 `PATH`，
   命令通过 `npm i -g @alibaba-group/open-code-review` 自动安装并继续。输出在 5
   分钟超时内捕获。
2. **过滤与评估。** 把每条评论分为 **High** / **Medium** / **Low**。低置信
   （疑似误报、吹毛求疵、缺上下文）评论被静默丢弃；其余展示。
3. **修复。** 对值得采纳的 High/Medium 项自动应用修复。与
   [Agent Skill](../agent-skill/) 不同，此命令**默认自动修复**——它是“评审并
   清理”工作流的合适选择，而非“给我看 diff”工作流。

若你想让命令在修改代码前先询问，或收紧分级标准，编辑你本地的 prompt 副本。Claude
Code 每次调用都重新读取命令，因此无需重启。

## 另见

- [Agent Skill](../agent-skill/)——SDK 级等价物；同一个底层 CLI，不同默认值
  （修复前先询问）。
- [Direct Subprocess](../subprocess/)——绕过 slash 命令，自行调用 CLI。
